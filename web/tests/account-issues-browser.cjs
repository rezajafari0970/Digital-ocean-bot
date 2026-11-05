const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),{spawn}=require('node:child_process'),assert=require('node:assert/strict');
(async()=>{
 const base=process.env.DOB_UI_BASE,token='isolated-browser-fixture';
 if(!base?.startsWith('http://127.0.0.1:')||!token)throw Error('local ephemeral session required');
 const root=process.env.DOB_UI_ARTIFACT_DIR||os.tmpdir(),profile=fs.mkdtempSync(path.join(root,'ui-profile-'));
 const log=fs.openSync(path.join(root,'browser.log'),'w');
 const chrome=spawn(process.env.DOB_TEST_CHROMIUM||'/snap/bin/chromium',['--headless','--no-sandbox','--disable-dev-shm-usage','--disable-gpu','--remote-debugging-port=0','--remote-debugging-address=127.0.0.1','--user-data-dir='+profile,'about:blank'],{stdio:['ignore',log,log],env:{...process.env,TMPDIR:root}});
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




  const id=process.env.DOB_ISSUES_ID;
  await call('Page.navigate',{url:base+'/admin/#accounts'});
  async function waitFor(expression){const end=Date.now()+15000;while(Date.now()<end){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,100))}throw Error('UI assertion timeout: '+expression+' BODY: '+await evaluate('document.body.innerText.slice(0,5000)'))}
  await waitFor('document.querySelectorAll(".accountCard").length===7');
  for(const provider of ['digitalocean','vultr','linode','aws','kamatera','upcloud','ovh']){
   assert.equal(await evaluate(`(()=>{const row=cache.accounts.find(x=>x.provider===${JSON.stringify(provider)});const card=document.querySelector('[data-account-id="'+row.id+'"]');return ['Provider lock remains visible','Independent route error remains visible','CREATE OUTCOME UNKNOWN'].every(t=>card.innerText.includes(t))})()`),true);
  }
  assert.equal(await evaluate('document.documentElement.scrollWidth<=390+1'),true);
  await evaluate('window.issueID='+JSON.stringify(id)+';window.issueMessages=[];window.originalToast=toast;toast=(m,k)=>{issueMessages.push(m);originalToast(m,k)}');
  await evaluate('accountPreflight(issueID)');
  assert.equal(await evaluate('issueMessages.some(x=>x.includes("Preflight provider rejection")&&x.includes("provider identity"))'),true);
  assert.equal(await evaluate('document.querySelectorAll(".accountCard").length'),7);
  await evaluate('accountDash(issueID)');
  await waitFor('document.querySelector(".accountIssues")?.innerText.includes("Independent route error remains visible")');
  await evaluate('accountRefresh(issueID)');
  await waitFor('document.querySelector(".accountIssues")?.innerText.includes("Fresh credential rejection is visible")');
  assert.equal(await evaluate('issueMessages.some(x=>x.includes("Refresh failed:")&&x.includes("Fresh credential rejection is visible"))'),true);
  assert.equal(await evaluate('document.querySelectorAll(".dropletTable tbody tr").length'),10);
  assert.equal(await evaluate('[...document.querySelectorAll(".stat b")].some(e=>e.textContent==="PROVEN_LOWER_BOUND")'),true);
  for(const width of [320,390,800,801,1024,1365]){
   await call('Emulation.setDeviceMetricsOverride',{width,height:844,deviceScaleFactor:1,mobile:width<=800});
   await evaluate('scrollTo(0,0);document.querySelector("#toast").hidden=true');
   assert.equal(await evaluate('document.documentElement.scrollWidth<='+width+'+1 && innerWidth<='+width+'+1'),true,'dashboard must fit physical viewport '+width);
   assert.equal(await evaluate('[...document.querySelectorAll(".stat b")].every(e=>e.scrollWidth<=e.clientWidth+1)'),true,'capacity labels stay inside cards');
   if(width===801){
    assert.equal(await evaluate('(()=>{const e=document.querySelector(".dropletTable");return e.scrollWidth>e.clientWidth&&getComputedStyle(e).overflowX==="auto"})()'),true,'wide server table scrolls inside its wrapper');
   }
   const shot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,'account-issues-'+width+'.png'),Buffer.from(shot.data,'base64'));
  }
  await call('Page.reload');
  await waitFor('document.querySelector(".accountIssues")?.innerText.includes("Fresh credential rejection is visible")');
  assert.equal(await evaluate('document.querySelector(".accountIssues").innerText.includes("Independent route error remains visible")'),true);
  console.log('ACCOUNT_ISSUES_SEVEN_PROVIDERS_LIST_DETAIL_FAILED_REFRESH_PREFLIGHT_RELOAD_MOBILE_DESKTOP PASS');
 }finally{
  if(ws)ws.close();chrome.kill('SIGTERM');fs.closeSync(log);
  await new Promise(r=>setTimeout(r,500));fs.rmSync(profile,{recursive:true,force:true});
 }
})().catch(e=>{console.error(e.stack);process.exitCode=1});
