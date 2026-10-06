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



  const id=process.env.DOB_DELETE_ID,other=process.env.DOB_OTHER_ID;
  await call('Page.navigate',{url:base+'/admin/#accounts'});
  async function waitFor(expression){const end=Date.now()+15000;while(Date.now()<end){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,100))}throw Error('UI assertion timeout: '+expression+' BODY: '+await evaluate('document.body.innerText.slice(0,5000)'))}
  await waitFor('document.querySelectorAll(".accountCard").length===2');
  await evaluate('window.deleteID='+JSON.stringify(id)+';window.otherID='+JSON.stringify(other)+';window.confirm=()=>true;window.originalFetch=window.fetch;window.deleteCalls=0;window.dropOnce=true;window.fetch=async function(url,opt){const r=await window.originalFetch(url,opt);if(opt?.method==="DELETE"){window.deleteCalls++;if(window.dropOnce){window.dropOnce=false;throw Error("Fixture lost response after commit")}}return r}');
  await evaluate('Promise.all([window.deleteAccount(deleteID),window.deleteAccount(deleteID)])');
  await waitFor('cache.accounts.find(r=>r.id===deleteID)?.deletion?.phase==="SETTLING"&&!accountDeletionBusy.has(deleteID)');
  assert.equal(await evaluate('window.deleteCalls'),1);
  assert.equal(await evaluate('[...document.querySelectorAll("[data-action=delete-account]")].find(b=>b.dataset.id===deleteID).disabled'),true);
  assert.ok(await evaluate('document.body.innerText.includes("Safety wait: about")'));
  assert.ok(await evaluate('document.body.innerText.includes("Next check:")'));
  await evaluate('[...document.querySelectorAll(".accountCard")].find(b=>b.dataset.accountId===deleteID).scrollIntoView()');
  assert.equal(await evaluate('document.documentElement.scrollWidth<=window.innerWidth+1'),true);
  const mobile=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,'account-deletion-mobile.png'),Buffer.from(mobile.data,'base64'));
  await call('Page.reload');await waitFor('typeof cache!=="undefined"&&cache.accounts?.some(r=>r.id==='+JSON.stringify(id)+'&&r.deletion?.phase==="SETTLING")');
  await evaluate('window.deleteID='+JSON.stringify(id)+';window.otherID='+JSON.stringify(other)+';window.originalFetch=window.fetch;window.holdPoll=true;window.pollCaptured=false;window.fetch=async function(url,opt){const r=await window.originalFetch(url,opt);if(url==="/api/v1/accounts"&&window.holdPoll){window.holdPoll=false;window.pollCaptured=true;await new Promise(resolve=>window.releasePoll=resolve)}return r};window.oldPoll=refreshAccountsInPlace({force:true});undefined');
  await waitFor('window.pollCaptured');
  await evaluate('api("/__fixture/finish",{method:"POST"})');
  await evaluate('refreshAccountsInPlace({force:true})');
  assert.equal(await evaluate('cache.accounts.some(r=>r.id===deleteID)'),false);
  assert.ok(await evaluate('document.querySelector("#toast").textContent.includes("Removed from panel")'));
  await evaluate('window.releasePoll();window.oldPoll');
  assert.equal(await evaluate('cache.accounts.some(r=>r.id===deleteID)'),false);
  assert.equal(await evaluate('document.querySelectorAll(".accountCard").length'),1);
  await call('Emulation.setDeviceMetricsOverride',{width:1365,height:950,deviceScaleFactor:1,mobile:false});
  await evaluate('window.confirm=()=>true;document.querySelector("[data-action=delete-account]").click()');
  await waitFor('cache.accounts[0]?.deletion&&!accountDeletionBusy.size');
  const desktop=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,'account-deletion-desktop.png'),Buffer.from(desktop.data,'base64'));
  await evaluate('api("/__fixture/finish",{method:"POST"})');
  await evaluate('refreshAccountsInPlace({force:true})');
  assert.equal(await evaluate('cache.accounts.length'),0);
  assert.equal(await evaluate('document.querySelectorAll(".accountCard").length'),0);
  assert.ok(await evaluate('document.querySelector("#toast").textContent.includes("Other fixture")'));
  console.log('ACCOUNT_DELETE_LOST_RESPONSE_DUPLICATE_SETTLE_RELOAD_STALE_POLL_COMPLETION_LAST_ACCOUNT_MOBILE_DESKTOP PASS');
 }finally{
  if(ws)ws.close();chrome.kill('SIGTERM');fs.closeSync(log);
  await new Promise(r=>setTimeout(r,500));fs.rmSync(profile,{recursive:true,force:true});
 }
})().catch(e=>{console.error(e.stack);process.exitCode=1});
