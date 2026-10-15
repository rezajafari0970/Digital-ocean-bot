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

  await call('Page.navigate',{url:base+'/admin/#ip-inventory'});
  async function waitFor(expression){const end=Date.now()+15000;while(Date.now()<end){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,100))}throw Error('UI assertion timeout: '+expression+' BODY: '+await evaluate('document.body.innerText.slice(0,6000)'))}
  await waitFor('document.querySelectorAll("[data-ip-account]").length===2');
  const text=await evaluate('document.querySelector("#content").innerText');
  assert.ok(text.includes('Allocation and rotation disabled'));
  assert.ok(text.includes('fixture <account>'));
  assert.ok(text.includes('Quota unknown'));
  assert.ok(text.includes('approved canary plan'));
  assert.equal(await evaluate('document.querySelectorAll("account").length'),0);
  assert.equal(await evaluate('document.querySelector("[data-ip-account] tbody td:nth-child(2)").textContent'),'0');
  assert.equal(await evaluate('document.documentElement.scrollWidth<=window.innerWidth+1'),true);
  await evaluate('window.ipCalls=[];window.ipOrigFetch=window.fetch;window.fetch=(url,opt)=>{if(opt?.method==="POST")window.ipCalls.push(url);return window.ipOrigFetch(url,opt)};document.querySelector("[data-ip-refresh]").click()');
  await waitFor('window.ipCalls.length===1&&document.querySelectorAll("[data-ip-refresh]:not(:disabled)").length===2');
  assert.equal((await evaluate('window.ipCalls'))[0],'/api/v1/accounts/11111111-1111-4111-8111-111111111111/ip-inventory/refresh');
  await call('Page.reload');await waitFor('document.querySelectorAll("[data-ip-account]").length===2');
  const v6='document.querySelectorAll("[data-ip-policy]")[1]',v4='document.querySelectorAll("[data-ip-policy]")[0]';
  await evaluate(v6+'.closest("details").open=true');
  assert.equal(await evaluate(v6+'.querySelector("[name=diversity_bits]").value'),'');
  assert.equal(await evaluate(v6+'.querySelector("[name=desired]").value'),'');
  await evaluate('const f='+v6+';f.querySelector("[name=desired]").value="2";f.querySelector("[name=diversity_bits]").value="56";f.querySelector("button").click()');
  await waitFor(v6+'?.dataset.revision==="1"');
  assert.equal(await evaluate(v6+'.querySelector("[name=desired]").value'),'2');
  assert.equal(await evaluate(v4+'.querySelector("[name=desired]").value'),'');
  await call('Page.reload');await waitFor(v6+'?.dataset.revision==="1"');
  assert.equal(await evaluate(v6+'.querySelector("[name=diversity_bits]").value'),'56');
  await evaluate(v6+'.closest("details").open=true');
  assert.equal(await evaluate('document.documentElement.scrollWidth<=window.innerWidth+1'),true);
  const shot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,'ip-inventory-mobile.png'),Buffer.from(shot.data,'base64'));
  console.log('IP_INVENTORY_SETTINGS_NULL_CAS_FAMILY_ISOLATION_RELOAD_MOBILE PASS');
 }finally{
  if(ws)ws.close();chrome.kill('SIGTERM');fs.closeSync(log);
  await new Promise(r=>setTimeout(r,500));fs.rmSync(profile,{recursive:true,force:true});
 }
})().catch(e=>{console.error(e.stack);process.exitCode=1});
