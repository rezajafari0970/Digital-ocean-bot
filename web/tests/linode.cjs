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
  async function waitFor(expression){const end=Date.now()+12000;while(Date.now()<end){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,100))}throw Error('UI assertion timeout: '+expression+'; state='+JSON.stringify(await evaluate('({url:location.href,ready:document.readyState,viewType:typeof activePageView,current:typeof current==="undefined"?null:current,loading:typeof activePageView==="undefined"?null:activePageView?.loading,appHidden:document.querySelector("#app")?.hidden,handler:typeof document.querySelector("#primaryAction")?.onclick,rendered:document.querySelector("#content")?.dataset.renderedPage})')))}
  // Wait for the route and event handlers, not just the static HTML button.
  await waitFor('typeof activePageView!=="undefined"&&current==="accounts"&&activePageView&&!activePageView.loading&&!document.querySelector("#app").hidden&&typeof document.querySelector("#primaryAction").onclick==="function"');
  await evaluate('document.querySelector("#primaryAction").click()');
  await waitFor('document.querySelector("#modalFields [name=provider]")!==null');
  assert.deepEqual((await evaluate('[...document.querySelector("#modalFields [name=provider]").options].map(x=>x.value)')).sort(),['digitalocean','linode','upcloud','vultr']);
  await evaluate('document.querySelector("#modalFields [name=provider]").value="linode";document.querySelector("#modalFields [name=provider]").dispatchEvent(new Event("change"))');
  assert.ok((await evaluate('document.querySelector("#accountCredentialLabel").textContent')).includes('Personal Access Token'));
  assert.equal(await evaluate('document.querySelector("#modalFields [name=token]").type'),'password');
  const fixture={provider:'linode',account:{},defaults:{region:'us-east'},regions:[{ID:'us-east',Name:'Newark',Available:true},{ID:'id-cgk',Name:'Jakarta',Available:true}],plans:[
   {ID:'g6-nanode-1',CPU:1,MemoryMB:1024,DiskGB:25,Available:true,AvailableRegions:['us-east','id-cgk'],PricesByRegion:{'us-east':{Currency:'USD',Hourly:.0075,MonthlyEstimate:5},'id-cgk':{Currency:'USD',Hourly:.009,MonthlyEstimate:6}}},
   {ID:'hourly',CPU:2,MemoryMB:2048,DiskGB:50,Available:true,AvailableRegions:['us-east','id-cgk'],PricesByRegion:{'us-east':{Currency:'USD',Hourly:.02,MonthlyEstimate:0},'id-cgk':{Currency:'USD',Hourly:.03,MonthlyEstimate:0}}}
  ],images:['26.04','24.04','22.04'].map(v=>({ID:'linode/ubuntu'+v,Name:'Ubuntu '+v,Family:'ubuntu',Version:v,Architecture:'x86_64',Available:true})),policy:{defaults:{images:{family:'ubuntu',versions:['26.04','24.04','22.04']},desired_servers:5,lifetime_min_minutes:90,lifetime_max_minutes:120,build_spacing_min_minutes:1,build_spacing_max_minutes:3,max_concurrent:1}}};
  await evaluate('window.fixture='+JSON.stringify(fixture)+';window.previewRequests=[];window.realFetch=window.fetch;window.fetch=async(url,opt)=>{if(url==="/api/v1/accounts/preview"){window.previewRequests.push(JSON.parse(opt.body));return new Response(JSON.stringify(window.fixture),{status:200})}if(opt?.method&&opt.method!=="GET")throw Error("fixture blocks mutations");return window.realFetch(url,opt)};document.querySelector("#modalFields [name=name]").value="Linode fixture";document.querySelector("#modalFields [name=token]").value="fixture-token";document.querySelector("#discoverAccount").click()');
  await waitFor('document.querySelector("#accountDiscovery [name=size]")!==null');
  assert.equal(await evaluate('window.previewRequests[0].provider'),'linode');
  assert.equal(await evaluate('document.querySelector("#accountDiscovery [name=desired_server_count]").value'),'5');
  assert.deepEqual(await evaluate("['image','image2','image3'].map(n=>document.querySelector('#accountDiscovery [name='+n+']').value)"),['linode/ubuntu26.04','linode/ubuntu24.04','linode/ubuntu22.04']);
  assert.ok((await evaluate("document.querySelector('#accountSize').options[0].textContent")).includes('USD 0.0075/h | 5.00/mo'));
  assert.ok((await evaluate("document.querySelector('[data-catalog-price-note]').textContent")).includes('provider-listed caps'));
  await evaluate("document.querySelector('#accountDiscovery [name=size2]').value='hourly';document.querySelector('#accountDiscovery [name=region]').value='id-cgk';document.querySelector('#accountDiscovery [name=region]').dispatchEvent(new Event('change'))");
  assert.ok((await evaluate("document.querySelector('#accountSize').options[0].textContent")).includes('USD 0.009/h | 6.00/mo'));
  assert.equal(await evaluate("document.querySelector('#accountDiscovery [name=size2]').value"),'hourly');
  assert.ok((await evaluate("document.querySelector('#accountDiscovery [name=size2]').selectedOptions[0].textContent")).includes('USD 0.03/h | hourly billing'));
  assert.equal(await evaluate("catalogPriceLabel(fixture.plans[1],'us-east','linode')"),'USD 0.02/h | hourly billing');
  assert.equal(await evaluate("catalogPriceLabel(fixture.plans[1],'missing','linode')"),'Price unavailable');
  assert.equal(await evaluate("catalogPriceLabel({PriceMonthly:6},'us-east','digitalocean')"),'$6/mo');
  assert.equal(await evaluate("catalogPriceLabel({PriceMonthly:5},'us-east','vultr')"),'$5/mo');
  assert.equal(await evaluate("catalogPriceLabel({PricesByRegion:{x:{Currency:'EUR',Hourly:.01,MonthlyEstimate:6.72}}},'x','upcloud')"),'EUR 0.01/h | ≈6.72/mo');
  assert.equal(await evaluate('document.documentElement.scrollWidth<=window.innerWidth+1'),true);
  let snap=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,'linode-account-mobile.png'),Buffer.from(snap.data,'base64'));
  await call('Emulation.setDeviceMetricsOverride',{width:1365,height:950,deviceScaleFactor:1,mobile:false});
  snap=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,'linode-account-desktop.png'),Buffer.from(snap.data,'base64'));
  await evaluate('document.querySelector("#modalFields [name=provider]").value="vultr";document.querySelector("#modalFields [name=provider]").dispatchEvent(new Event("change"))');
  assert.equal(await evaluate('cache.accountPreview===null&&document.querySelector("#modalSubmit").hidden&&document.querySelector("#accountDiscovery").textContent===""'),true);
  await evaluate("document.querySelector('#modal').close();cache.accounts=[{id:'linode-fixture',provider:'linode',name:'Fixture',region:'us-east',regions:['us-east','id-cgk'],sizes:['g6-nanode-1','hourly'],image:'linode/ubuntu24.04',images:['linode/ubuntu24.04','linode/ubuntu26.04','linode/ubuntu22.04']}];window.fixtureFetch=window.fetch;window.fetch=async(url,opt)=>url==='/api/v1/accounts/linode-fixture/options'?new Response(JSON.stringify(window.fixture),{status:200}):window.fixtureFetch(url,opt);editAccount('linode-fixture')");
  await waitFor("document.querySelector('#modalTitle').textContent==='Edit Fixture'");
  await evaluate("document.querySelector('#modalFields [name=region]').value='id-cgk';document.querySelector('#modalFields [name=region]').dispatchEvent(new Event('change'))");
  assert.ok((await evaluate("document.querySelector('#modalFields [name=size]').options[0].textContent")).includes('USD 0.009/h | 6.00/mo'));
  assert.equal(await evaluate("document.querySelector('#modalFields [name=size2]').value"),'hourly');
  assert.ok(await evaluate("accountCard({id:'linode-fixture',name:'Linode fixture',provider:'linode',enabled:true,provider_state:'ACTIVE',capacity_state:'UNKNOWN',capacity_limit_known:false,capacity_freshness:'fresh',provider_freshness:'fresh',droplet_limit:0,provider_droplets:0,desired_server_count:5,buildable_now:null}).includes('Unknown')"));
  console.log('LINODE_FORM_PREVIEW_PRICE_EDIT_UNKNOWN_CAPACITY_MOBILE_DESKTOP_REGRESSION PASS');
 }finally{
  if(ws)ws.close();chrome.kill('SIGTERM');fs.closeSync(log);
  await new Promise(r=>setTimeout(r,500));fs.rmSync(profile,{recursive:true,force:true});
 }
})().catch(e=>{console.error(e.stack);process.exitCode=1});
