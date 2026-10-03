const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),{spawn}=require('node:child_process'),assert=require('node:assert/strict');
(async()=>{
 const base=process.env.DOB_UI_BASE,token=process.env.DOB_UI_TOKEN;
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
  async function waitFor(expression){const end=Date.now()+12000;while(Date.now()<end){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,150))}throw Error('UI assertion timeout')}
  await waitFor('document.querySelectorAll(".accountCard").length>0');
  const cards=await evaluate('document.querySelectorAll(".accountCard").length');
  assert.equal(await evaluate('document.querySelectorAll(".accountCard .billingCompact").length'),cards);
  assert.equal(await evaluate('[...document.querySelectorAll(".billingCompact")].every(x=>x.closest(".accountCard")&&!x.open)'),true);
  const snap=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,'accounts-mobile.png'),Buffer.from(snap.data,'base64'));
  console.log('UI_ACCOUNT_BILLING_NESTED_COMPACT PASS accounts='+cards);
  await evaluate('load("residential")');await waitFor('Array.isArray(cache.residential)');
  const resIDs=await evaluate('(cache.residential||[]).map(x=>x.id)');
  await evaluate('load("proxies")');await waitFor('Array.isArray(cache.proxies)');
  const proxyIDs=await evaluate('(cache.proxies||[]).map(x=>x.id)');
  assert.equal(resIDs.some(x=>proxyIDs.includes(x)),false);
  console.log('UI_RESIDENTIAL_PROXY_ISOLATION PASS');
  await evaluate('load("configs")');await waitFor('document.querySelector("[data-action=resume-capacity]")!==null');
  const activeCleanup=await evaluate('cache.cleanup&&!["done","cancelled"].includes(cache.cleanup.status)');
  if(activeCleanup)assert.equal(await evaluate('document.querySelectorAll(".cleanupStatus [data-action=cancel-cleanup]").length'),1);
  console.log('UI_CLEANUP_STATUS_SURVIVES_NAVIGATION PASS');
  await evaluate('load("dashboard")');await waitFor('document.querySelectorAll(".stat").length>=10');
  assert.equal(await evaluate('document.querySelector("#content").textContent.includes("Panels needing attention")'),true);
  console.log('UI_DASHBOARD PASS');
 }finally{
  if(ws)ws.close();chrome.kill('SIGTERM');fs.closeSync(log);
  await new Promise(r=>setTimeout(r,500));fs.rmSync(profile,{recursive:true,force:true});
 }
})().catch(e=>{console.error(e.message);process.exitCode=1});
