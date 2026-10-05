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
  const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error('page evaluation failed');return r.result.value};

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
  await evaluate(`window.previewRequests=[];window.realFetch=window.fetch;window.fetch=async(url,opt)=>{if(url==='/api/v1/accounts/preview'){window.previewRequests.push(JSON.parse(opt.body));return new Response(JSON.stringify({provider:'upcloud',account:{},defaults:{region:'fi-hel1'},regions:[{ID:'fi-hel1',Name:'Helsinki',Available:true}],plans:[{ID:'1xCPU-2GB',CPU:1,MemoryMB:2048,DiskGB:50,Available:true}],images:[{ID:'01000000-0000-4000-8000-000030240200',Family:'ubuntu',Version:'24.04',Available:true}],policy:{defaults:{desired_servers:5,lifetime_min_minutes:90,lifetime_max_minutes:120,build_spacing_min_minutes:1,build_spacing_max_minutes:3,max_concurrent:1}}}),{status:200,headers:{'Content-Type':'application/json'}})};if(opt?.method&&opt.method!=='GET')throw Error('fixture blocks mutations');return window.realFetch(url,opt)};document.querySelector("#modalFields [name=name]").value="UpCloud fixture";document.querySelector("#modalFields [name=token]").value="ucat_fixture";document.querySelector("#discoverAccount").click()`);
  await waitFor('document.querySelector("#accountDiscovery [name=size]")!==null');
  assert.equal(await evaluate('window.previewRequests[0].provider'),'upcloud');
  assert.equal(await evaluate('document.querySelector("#accountDiscovery [name=desired_server_count]").value'),'5');
  assert.equal(await evaluate('document.querySelector("#accountDiscovery [name=region]").value'),'fi-hel1');
  assert.equal(await evaluate('document.querySelector("#modalSubmit").hidden'),false);
  assert.equal(await evaluate('document.documentElement.scrollWidth<=window.innerWidth+1'),true);
  let snap=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,'upcloud-account-mobile.png'),Buffer.from(snap.data,'base64'));
  await call('Emulation.setDeviceMetricsOverride',{width:1365,height:950,deviceScaleFactor:1,mobile:false});
  snap=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,'upcloud-account-desktop.png'),Buffer.from(snap.data,'base64'));
  await evaluate('document.querySelector("#modalFields [name=provider]").value="vultr";document.querySelector("#modalFields [name=provider]").dispatchEvent(new Event("change"))');
  assert.equal(await evaluate('cache.accountPreview===null&&document.querySelector("#modalSubmit").hidden&&document.querySelector("#accountDiscovery").textContent===""'),true);
  console.log('UPCLOUD_PROVIDER_FORM_CATALOG_MOBILE_DESKTOP_STALE_PREVIEW_RESET PASS');
 }finally{
  if(ws)ws.close();chrome.kill('SIGTERM');fs.closeSync(log);
  await new Promise(r=>setTimeout(r,500));fs.rmSync(profile,{recursive:true,force:true});
 }
})().catch(e=>{console.error(e.message);process.exitCode=1});
