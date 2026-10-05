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
  const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error('page evaluation failed: '+expression+' '+JSON.stringify(r.exceptionDetails));return r.result.value};

  await call('Browser.grantPermissions',{origin:base,permissions:['clipboardReadWrite','clipboardSanitizedWrite']}).catch(()=>{});
  await call('Page.navigate',{url:base+'/admin/#residential'});
  async function waitFor(expression){const end=Date.now()+12000;while(Date.now()<end){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,100))}throw Error('UI assertion timeout: '+expression)}
  await waitFor('document.querySelector("[data-performance=start]")!==null');
  await evaluate('document.querySelector("[data-performance=start]").click()');
  await waitFor('document.querySelectorAll("#residentialDialog [name=panel]").length===1');
  await evaluate('document.querySelector("#residentialDialog [name=mode]").value="timed";document.querySelector("#residentialDialog [name=mode]").dispatchEvent(new Event("change",{bubbles:true}))');
  await evaluate('document.querySelector("#residentialDialog [data-submit]").click()');
  await waitFor('document.querySelector("#residentialDialog [data-preview]")?.textContent.includes("canary.test")');
  assert.ok((await evaluate('document.querySelector("#residentialDialog [data-preview]").textContent')).includes('canary.test'));
  assert.equal(await evaluate('document.documentElement.scrollWidth<=window.innerWidth+1'),true);
  const snap=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,'performance-preview-mobile.png'),Buffer.from(snap.data,'base64'));
  await evaluate(`window.originalFetch=window.fetch;window.dropOnce=true;window.fetch=async function(url,opt){const r=await window.originalFetch(url,opt);if(window.dropOnce&&opt?.body&&url==='/api/v1/residential-performance'){window.dropOnce=false;throw Error('Fixture lost response after commit')}return r};document.querySelector("#residentialDialog [data-submit]").click()`);
  await waitFor('document.querySelector("#residentialDialog [data-error]")?.textContent.includes("lost response")');
  await evaluate('document.querySelector("#residentialDialog [data-submit]").click()');
  await waitFor('!document.querySelector("#residentialDialog")&&performanceStatus?.experiment?.state==="RUNNING"');
  assert.equal(await evaluate('document.querySelector("[data-performance=keep]").disabled'),true);
  assert.equal(await evaluate('document.querySelector("[data-performance=rollback]").disabled'),false);
  await call('Page.reload');
  await waitFor('typeof performanceStatus!=="undefined"&&performanceStatus?.experiment?.state==="RUNNING"');
  await evaluate('document.querySelector("[data-performance=rollback]").click()');
  await waitFor('typeof performanceStatus!=="undefined"&&performanceStatus?.experiment?.state==="ROLLING_BACK"');
  assert.ok(await evaluate('document.querySelector("#residentialPerformance").textContent.includes("ROLLBACK_PENDING")'));
  await call('Emulation.setDeviceMetricsOverride',{width:1365,height:950,deviceScaleFactor:1,mobile:false});
  const desktop=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,'performance-rollback-desktop.png'),Buffer.from(desktop.data,'base64'));
  console.log('UI_PROFILE_PREVIEW_LOST_RESPONSE_RETRY_RELOAD_ROLLBACK_PENDING_MOBILE PASS');
 }finally{
  if(ws)ws.close();chrome.kill('SIGTERM');fs.closeSync(log);
  await new Promise(r=>setTimeout(r,500));fs.rmSync(profile,{recursive:true,force:true});
 }
})().catch(e=>{console.error(e.message);process.exitCode=1});
