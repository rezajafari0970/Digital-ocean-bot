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




  const id=process.env.DOB_RULE_ID;
  await call('Page.navigate',{url:base+'/admin/#accounts'});
  async function waitFor(expression){const end=Date.now()+15000;while(Date.now()<end){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,100))}throw Error('UI assertion timeout: '+expression+' BODY: '+await evaluate('document.body.innerText.slice(0,5000)'))}
  await waitFor('document.querySelectorAll(".accountCard").length===1');
  await evaluate('window.ruleID='+JSON.stringify(id)+';window.alert=()=>{};window.confirm=()=>true');
  await evaluate('editAccount(ruleID)');
  const check='document.querySelector("[name=apply_to_existing]")';
  assert.equal(await evaluate(check+'.checked'),false);
  assert.equal(await evaluate('document.documentElement.scrollWidth<=window.innerWidth+1'),true);
  await evaluate('document.querySelector("[name=lifetime_min_minutes]").value="150";document.querySelector("[name=lifetime_max_minutes]").value="160";document.querySelector("#modalSubmit").click()');
  await waitFor('!document.querySelector("#modal").open&&cache.accounts?.[0].rule_application?.revision===1');
  assert.equal((await evaluate('api("/__fixture/tick",{method:"POST"})')).retiring,0);
  await evaluate('editAccount(ruleID)');
  assert.equal(await evaluate(check+'.checked'),false);
  assert.equal(await evaluate('document.querySelector("[name=lifetime_min_minutes]").value'),'150');
  await evaluate(check+'.click();document.querySelector("[name=desired_server_count]").value="4";document.querySelector("#modalSubmit").click()');
  await waitFor('!document.querySelector("#modal").open&&cache.accounts?.[0].desired_server_count===4');
  assert.equal((await evaluate('api("/__fixture/tick",{method:"POST"})')).retiring,0);
  await evaluate('editAccount(ruleID)');
  assert.equal(await evaluate(check+'.checked'),true);
  const mobile=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,'account-rule-mobile.png'),Buffer.from(mobile.data,'base64'));
  await evaluate('document.querySelector("[name=desired_server_count]").value="2";document.querySelector("#modalSubmit").click()');
  await waitFor('!document.querySelector("#modal").open&&cache.accounts?.[0].desired_server_count===2');
  assert.equal((await evaluate('api("/__fixture/tick",{method:"POST"})')).retiring,1);
  assert.equal((await evaluate('api("/__fixture/tick",{method:"POST"})')).retiring,1);
  await call('Emulation.setDeviceMetricsOverride',{width:1365,height:950,deviceScaleFactor:1,mobile:false});
  await evaluate('refreshAccountsInPlace({force:true})');
  const desktop=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,'account-rule-desktop.png'),Buffer.from(desktop.data,'base64'));
  await evaluate('editAccount(ruleID)');
  await evaluate(check+'.click();document.querySelector("#modalSubmit").click()');
  await waitFor('!document.querySelector("#modal").open&&cache.accounts?.[0].rule_application?.apply_to_existing===false');
  await call('Page.reload');
  await waitFor('typeof cache!=="undefined"&&cache.accounts?.[0]?.rule_application?.apply_to_existing===false');
  console.log('ACCOUNT_RULE_DEFAULT_OFF_FUTURE_ONLY_GROWTH_SHRINK_PERSIST_RELOAD_MOBILE_DESKTOP PASS');
 }finally{
  if(ws)ws.close();chrome.kill('SIGTERM');fs.closeSync(log);
  await new Promise(r=>setTimeout(r,500));fs.rmSync(profile,{recursive:true,force:true});
 }
})().catch(e=>{console.error(e.stack);process.exitCode=1});
