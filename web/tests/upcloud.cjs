const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),{spawn}=require('node:child_process'),assert=require('node:assert/strict');
(async()=>{
 const base=process.env.DOB_UI_BASE,token='isolated-browser-fixture';
 if(!base?.startsWith('http://127.0.0.1:')||!token)throw Error('local ephemeral session required');
 const root=process.env.DOB_UI_ARTIFACT_DIR||os.tmpdir(),profile=fs.mkdtempSync(path.join(root,'ui-profile-'));
 const log=fs.openSync(path.join(root,'browser.log'),'w');
 const chrome=spawn('/snap/bin/chromium',['--headless','--no-sandbox','--disable-dev-shm-usage','--disable-gpu','--remote-debugging-port=0','--remote-debugging-address=127.0.0.1','--user-data-dir='+profile,'about:blank'],{stdio:['ignore',log,log],env:{...process.env,TMPDIR:root}});
 let ws;try{
  const until=Date.now()+20000;let port;
  while(Date.now()<until){try{port=Number(fs.readFileSync(path.join(profile,'DevToolsActivePort'),'utf8').split('\n')[0]);break}catch{};await new Promise(r=>setTimeout(r,200))}
  if(!port)throw Error('browser did not start');
  const version=await(await fetch('http://127.0.0.1:'+port+'/json/version')).json();
  ws=new WebSocket(version.webSocketDebuggerUrl);await new Promise((r,j)=>{ws.onopen=r;ws.onerror=j});
  let seq=0;const pending=new Map();
  ws.onmessage=({data})=>{const m=JSON.parse(data);if(m.id){const h=pending.get(m.id);if(h){pending.delete(m.id);m.error?h[1](Error(m.error.message)):h[0](m.result)}}};
  const send=(method,params={},sessionId)=>new Promise((resolve,reject)=>{const id=++seq;pending.set(id,[resolve,reject]);ws.send(JSON.stringify({id,method,params,sessionId}))});
  const {targetId}=await send('Target.createTarget',{url:'about:blank'});
  const {sessionId}=await send('Target.attachToTarget',{targetId,flatten:true});
  const call=(m,p)=>send(m,p,sessionId);
  await call('Page.enable');await call('Runtime.enable');
  await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
  await call('Page.addScriptToEvaluateOnNewDocument',{source:'if(location.origin==='+JSON.stringify(base)+'){localStorage.setItem("token",'+JSON.stringify(token)+')}'});
  const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error('page evaluation failed: '+(r.exceptionDetails.exception?.description||r.exceptionDetails.text));return r.result.value};

  await call('Page.navigate',{url:base+'/admin/#accounts'});
  async function waitFor(expression){const end=Date.now()+12000;while(Date.now()<end){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,100))}throw Error('UI assertion timeout: '+expression)}
  await waitFor('document.querySelector("#primaryAction")!==null');
  await evaluate('document.querySelector("#primaryAction").click()');
  await waitFor('document.querySelector("#modalFields [name=provider]")!==null');
  assert.deepEqual((await evaluate('[...document.querySelector("#modalFields [name=provider]").options].map(x=>x.value)')).sort(),['digitalocean','upcloud','vultr']);
  await evaluate('document.querySelector("#modalFields [name=provider]").value="upcloud";document.querySelector("#modalFields [name=provider]").dispatchEvent(new Event("change"))');
  assert.ok((await evaluate('document.querySelector("#accountCredentialLabel").textContent')).includes('ucat_'));
  assert.equal(await evaluate('document.querySelector("#modalFields [name=token]").type'),'password');
  // Provider responses are synthetic; account creation and cloud mutations are forbidden.
  await evaluate(`window.previewRequests=[];window.realFetch=window.fetch;window.fetch=async(url,opt)=>{if(url==='/api/v1/accounts/preview'){window.previewRequests.push(JSON.parse(opt.body));return new Response(JSON.stringify({provider:'upcloud',account:{},defaults:{region:'fi-hel1'},regions:[{ID:'fi-hel1',Name:'Helsinki',Available:true},{ID:'es-mad1',Name:'Madrid',Available:true}],plans:[
 {ID:'1xCPU-2GB',CPU:1,MemoryMB:2048,DiskGB:50,Available:true,PricesByRegion:{'fi-hel1':{Currency:'EUR',Hourly:.0075,MonthlyEstimate:5.04},'es-mad1':{Currency:'EUR',Hourly:.01,MonthlyEstimate:6.72}}},
 {ID:'DEV-1xCPU-1GB',CPU:1,MemoryMB:1024,DiskGB:20,Available:true,PricesByRegion:{'fi-hel1':{Currency:'EUR',Hourly:.005,MonthlyEstimate:3.36}}},
],images:[
 {ID:'ubuntu26',Name:'Ubuntu Server 26.04 LTS (cloud-init)',Family:'ubuntu',Version:'26.04',Architecture:'x86_64',Available:true},
 {ID:'ubuntu24-a',Name:'Ubuntu Server 24.04 LTS (cloud-init)',Family:'ubuntu',Version:'24.04',Architecture:'x86_64',Available:true},
 {ID:'ubuntu24-b',Name:'Ubuntu Server 24.04 LTS (cloud-init)',Family:'ubuntu',Version:'24.04',Architecture:'x86_64',Available:true},
 {ID:'ubuntu22',Name:'Ubuntu Server 22.04 LTS (cloud-init)',Family:'ubuntu',Version:'22.04',Architecture:'x86_64',Available:true}
],policy:{defaults:{images:{family:'ubuntu',versions:['26.04','24.04','22.04']},desired_servers:5,lifetime_min_minutes:90,lifetime_max_minutes:120,build_spacing_min_minutes:1,build_spacing_max_minutes:3,max_concurrent:1}}}),{status:200,headers:{'Content-Type':'application/json'}})};if(opt?.method&&opt.method!=='GET')throw Error('fixture blocks mutations');return window.realFetch(url,opt)};document.querySelector("#modalFields [name=name]").value="UpCloud fixture";document.querySelector("#modalFields [name=token]").value="ucat_fixture";document.querySelector("#discoverAccount").click()`);
  await waitFor('document.querySelector("#accountDiscovery [name=size]")!==null');
  assert.equal(await evaluate('window.previewRequests[0].provider'),'upcloud');
  assert.equal(await evaluate('document.querySelector("#accountDiscovery [name=desired_server_count]").value'),'5');
  assert.equal(await evaluate('document.querySelector("#accountDiscovery [name=region]").value'),'fi-hel1');
  assert.equal(await evaluate('document.querySelector("#modalSubmit").hidden'),false);
  assert.deepEqual(await evaluate("['image','image2','image3'].map(n=>document.querySelector('#accountDiscovery [name='+n+']').value)"),['ubuntu26','ubuntu24-a','ubuntu22']);
  const imageLabels=await evaluate("[...document.querySelector('#accountDiscovery [name=image]').options].map(o=>o.textContent)");
  assert.equal(new Set(imageLabels).size,4);assert.ok(imageLabels.every(x=>x.includes('x86_64')&&x.includes('cloud-init')));
  assert.ok(imageLabels[1].includes('ubuntu24-a'));assert.ok(imageLabels[2].includes('ubuntu24-b'));
  assert.ok((await evaluate("document.querySelector('#accountSize').options[0].textContent")).includes('EUR 0.0075/h | ≈5.04/mo'));
  await evaluate("document.querySelector('#accountDiscovery [name=size2]').value='DEV-1xCPU-1GB';document.querySelector('#accountDiscovery [name=image2]').value='ubuntu24-b';document.querySelector('#accountDiscovery [name=region]').value='es-mad1';document.querySelector('#accountDiscovery [name=region]').dispatchEvent(new Event('change'))");
  const labels=await evaluate("['size','size2','size3'].map(n=>document.querySelector('#accountDiscovery [name='+n+']').options[0].textContent)");
  assert.ok(labels.every(x=>x.includes('EUR 0.01/h | ≈6.72/mo')));
  assert.equal(await evaluate("document.querySelector('#accountDiscovery [name=size2]').value"),'DEV-1xCPU-1GB');
  assert.equal(await evaluate("document.querySelector('#accountDiscovery [name=image2]').value"),'ubuntu24-b');
  assert.ok((await evaluate("document.querySelector('#accountDiscovery [name=size2]').selectedOptions[0].textContent")).includes('Price unavailable'));
  assert.equal(await evaluate("catalogPriceLabel({PriceMonthly:6},'fra1','digitalocean')"),'$6/mo');
  assert.equal(await evaluate("catalogPriceLabel({PriceMonthly:5},'fra1','vultr')"),'$5/mo');
  assert.equal(await evaluate("catalogPriceLabel({PriceMonthly:6},'fi-hel1','upcloud')"),'Price unavailable');
  await evaluate("window.savedFixture=cache.accountPreview");

  assert.equal(await evaluate('document.documentElement.scrollWidth<=window.innerWidth+1'),true);
  let snap=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,'upcloud-account-mobile.png'),Buffer.from(snap.data,'base64'));
  await call('Emulation.setDeviceMetricsOverride',{width:1365,height:950,deviceScaleFactor:1,mobile:false});
  snap=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,'upcloud-account-desktop.png'),Buffer.from(snap.data,'base64'));
  await evaluate('document.querySelector("#modalFields [name=provider]").value="vultr";document.querySelector("#modalFields [name=provider]").dispatchEvent(new Event("change"))');
  assert.equal(await evaluate('cache.accountPreview===null&&document.querySelector("#modalSubmit").hidden&&document.querySelector("#accountDiscovery").textContent===""'),true);

  await evaluate("document.querySelector('#modal').close();cache.accounts=[{id:'upcloud-fixture',provider:'upcloud',name:'Fixture',region:'fi-hel1',regions:['fi-hel1','es-mad1'],sizes:['1xCPU-2GB','DEV-1xCPU-1GB','1xCPU-2GB'],image:'ubuntu24-b',images:['ubuntu24-b','ubuntu26','ubuntu22']}];window.oldFixtureFetch=window.fetch;window.fetch=async(url,opt)=>url==='/api/v1/accounts/upcloud-fixture/options'?new Response(JSON.stringify(window.savedFixture),{status:200}):window.oldFixtureFetch(url,opt);editAccount('upcloud-fixture')");
  await waitFor("document.querySelector('#modalTitle').textContent==='Edit Fixture'");
  assert.deepEqual(await evaluate("['image','image2','image3'].map(n=>document.querySelector('#modalFields [name='+n+']').value)"),['ubuntu24-b','ubuntu26','ubuntu22']);
  await evaluate("document.querySelector('#modalFields [name=region]').value='es-mad1';document.querySelector('#modalFields [name=region]').dispatchEvent(new Event('change'))");
  assert.ok((await evaluate("['size','size2','size3'].map(n=>document.querySelector('#modalFields [name='+n+']').options[0].textContent)")).every(x=>x.includes('EUR 0.01/h')));
  assert.equal(await evaluate("document.querySelector('#modalFields [name=size2]').value"),'DEV-1xCPU-1GB');

  await evaluate("document.querySelector('#modal').close()");
  await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
  await evaluate("window.blockFixture={version:4,code:'TRIAL_FIREWALL',blocked_at:new Date().toISOString()};window.countFixture={id:'capacity-fixture',name:'UpCloud trial',provider:'upcloud',provider_state:'ACTIVE',enabled:true,provider_can_create:false,scheduler_can_build:false,provider_reason:'UpCloud trial firewall restriction (TRIAL_FIREWALL). Resolve with UpCloud.',scheduler_reason:'Provider restriction',create_block:blockFixture,capacity_state:'RESOURCE_BUDGET',capacity_limit_known:true,capacity_freshness:'fresh',provider_freshness:'fresh',droplet_limit:2,droplet_available:2,provider_droplets:0,desired_server_count:5,desired_remaining:5,buildable_now:0,pending_builds:0,plan_available:{'1xCPU-1GB':2,'DEV-1xCPU-1GB-10GB':2}};document.querySelector('#content').innerHTML=accountCard(countFixture)");
  assert.equal(await evaluate("document.querySelector('#content').innerText.includes('TRIAL_FIREWALL')"),true);
  assert.equal(await evaluate("document.querySelector('[data-action=account-create-retry]').dataset.version"),'4');
  assert.equal(await evaluate("document.querySelector('#content').innerText.includes('Can build now\\n0')"),true);
  await evaluate("document.querySelector('details.section').open=true");
  assert.equal(await evaluate("document.querySelector('#content').innerText.includes('2 additional')"),true);
  assert.equal(await evaluate('document.documentElement.scrollWidth<=window.innerWidth+1'),true);
  snap=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:true});fs.writeFileSync(path.join(root,'upcloud-capacity-list-mobile.png'),Buffer.from(snap.data,'base64'));
  await evaluate("window.fetch=async(url,opt)=>url==='/api/v1/accounts/capacity-fixture/dashboard'?new Response(JSON.stringify({account:countFixture,capacity:{capacity_state:'RESOURCE_BUDGET',server_limit:2,provider_servers:0,managed_servers:0,desired_servers:5,desired_remaining:5,available:2,buildable_now:0,pending_builds:0,plan_available:countFixture.plan_available,data_available:true,data_status:'fresh',last_refresh:new Date().toISOString()}}),{status:200}):window.oldFixtureFetch(url,opt);accountDash('capacity-fixture')");
  assert.equal(await evaluate("document.querySelector('#content').innerText.includes('Can build now\\n0')"),true);
  assert.equal(await evaluate("document.querySelector('#content').innerText.includes('Resource-derived total capacity')"),true);
  assert.equal(await evaluate("document.querySelector('#content').innerText.includes('TRIAL_FIREWALL')"),true);
  assert.equal(await evaluate('document.documentElement.scrollWidth<=window.innerWidth+1'),true);
  snap=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:true});fs.writeFileSync(path.join(root,'upcloud-capacity-detail-mobile.png'),Buffer.from(snap.data,'base64'));
  assert.ok(await evaluate("accountCard({...countFixture,create_block:null,buildable_now:null,capacity_state:'UNKNOWN',capacity_limit_known:false}).includes('Unknown')"));
  console.log('UPCLOUD_PROVIDER_FORM_CATALOG_MOBILE_DESKTOP_STALE_PREVIEW_RESET PASS');
 }finally{
  if(ws)ws.close();chrome.kill('SIGTERM');fs.closeSync(log);
  await new Promise(r=>setTimeout(r,500));fs.rmSync(profile,{recursive:true,force:true});
 }
})().catch(e=>{console.error(e.message);process.exitCode=1});
