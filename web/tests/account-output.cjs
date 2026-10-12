const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),{spawn}=require('node:child_process'),assert=require('node:assert/strict');
(async()=>{
 const base=process.env.DOB_UI_BASE,token='isolated-browser-fixture';
 if(!base?.startsWith('http://127.0.0.1:')||!token)throw Error('local ephemeral session required');
 const root=process.env.DOB_UI_ARTIFACT_DIR||os.tmpdir(),profile=fs.mkdtempSync(path.join(root,'ui-profile-'));
 const log=fs.openSync(path.join(root,'browser.log'),'w');
 const chrome=spawn('/snap/bin/chromium',['--headless','--disable-popup-blocking','--no-sandbox','--disable-dev-shm-usage','--disable-gpu','--remote-debugging-port=0','--remote-debugging-address=127.0.0.1','--user-data-dir='+profile,'about:blank'],{stdio:['ignore',log,log],env:{...process.env,TMPDIR:root}});
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
  const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true,userGesture:true});if(r.exceptionDetails)throw Error('page evaluation failed: '+(r.exceptionDetails.exception?.description||r.exceptionDetails.text));return r.result.value};

  await call('Page.navigate',{url:base+'/admin/#accounts'});
  async function waitFor(expression){const end=Date.now()+12000;while(Date.now()<end){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,100))}throw Error('UI assertion timeout: '+expression)}

  const fixtures=JSON.parse(process.env.DOB_ACCOUNT_FIXTURES);
  await waitFor('document.querySelectorAll("[data-action=open-account-output]").length===8');
  assert.equal(await evaluate('document.documentElement.scrollWidth<=window.innerWidth+1'),true);
  const before=(await send('Target.getTargets')).targetInfos.map(x=>x.targetId);
  // Rapid independent clicks must retain their originating account and class.
  await evaluate('[...document.querySelectorAll("[data-action=open-account-output]")].forEach(b=>b.click())');
  let targets=[];
  for(let i=0;i<100;i++){
   targets=(await send('Target.getTargets')).targetInfos.filter(x=>!before.includes(x.targetId)&&x.url.includes('/share/account-output/'));
   if(targets.length===8)break;
   await new Promise(r=>setTimeout(r,100));
  }
  assert.equal(targets.length,8);
  const seen=new Set(),popups=[];
  for(const target of targets){
   const {sessionId:sid}=await send('Target.attachToTarget',{targetId:target.targetId,flatten:true});
   const ev=async expression=>{const v=await send('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true,userGesture:true},sid);return v.result?.value};
   await send('Page.enable',{},sid);await send('Page.bringToFront',{},sid);
   let text='';
   for(let i=0;i<50;i++){text=await ev('document.getElementById("configs")?.textContent');if(text)break;await new Promise(r=>setTimeout(r,100))}
   const matches=fixtures.flatMap(f=>Object.entries(f.URIs).filter(([,uri])=>text===uri+'\n').map(([cls])=>f.Account+'/'+cls));
   assert.equal(matches.length,1,'popup must show exactly one owned class');
   assert.ok(!seen.has(matches[0]));seen.add(matches[0]);
   const raw=new URL(target.url);raw.searchParams.delete('view');
   const response=await fetch(raw);assert.equal(response.status,200);assert.equal(await response.text(),text);
   popups.push({target,sid,ev,text});
  }
  assert.equal(seen.size,8);
  let snap=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,'account-output-mobile.png'),Buffer.from(snap.data,'base64'));
  await call('Emulation.setDeviceMetricsOverride',{width:1365,height:950,deviceScaleFactor:1,mobile:false});
  snap=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,'account-output-desktop.png'),Buffer.from(snap.data,'base64'));
  await fetch(base+'/__fixture/expire',{method:'POST'});
  // Real live viewer clears expired credentials using the same capability.
  for(const p of popups){
   await send('Page.bringToFront',{},p.sid);
   let cleared=false;
   for(let i=0;i<60;i++){if(await p.ev('document.getElementById("configs")?.textContent===""')){cleared=true;break}await new Promise(r=>setTimeout(r,100))}
   assert.ok(cleared,'expired output remained visible');
   assert.equal(await p.ev('location.href'),p.target.url);
  }
  console.log('ACCOUNT_OUTPUT_8_BUTTONS_4_PROVIDERS_POPUPS_RAW_URI_FIXED_TOKEN_EXPIRY_MOBILE_DESKTOP PASS');
 }finally{
  if(ws)ws.close();chrome.kill('SIGTERM');fs.closeSync(log);
  await new Promise(r=>setTimeout(r,500));fs.rmSync(profile,{recursive:true,force:true});
 }
})().catch(e=>{console.error(e.stack);process.exitCode=1});
