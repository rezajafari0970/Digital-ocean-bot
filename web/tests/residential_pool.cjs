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

  await call('Browser.grantPermissions',{origin:base,permissions:['clipboardReadWrite','clipboardSanitizedWrite']}).catch(()=>{});
  await call('Page.navigate',{url:base+'/admin/#residential'});
  async function waitFor(expression){const end=Date.now()+12000;while(Date.now()<end){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,100))}throw Error('UI assertion timeout: '+expression)}
  await waitFor('document.querySelector("[data-residential=import]")!==null');
  await evaluate('document.querySelector("[data-residential=import]").click()');
  const lines='127.0.0.1:1201:user:pass:colon\n\n127.0.0.1:1202:user:second';
  await evaluate('document.querySelector("#residentialDialog [name=proxies]").value='+JSON.stringify(lines));
  await evaluate('document.querySelector("#residentialDialog [data-submit]").click()');
  await waitFor('document.querySelectorAll("#residentialDialog [data-row]").length===2');
  assert.deepEqual(await evaluate('[...document.querySelectorAll("[data-row]")].map(x=>x.value)'),['ads1','ads2']);
  await evaluate('document.querySelectorAll("[data-row]")[1].value="Ads2";document.querySelector("#residentialDialog [data-submit]").click()');
  await waitFor('cache.residential?.length===2&&!document.querySelector("#residentialDialog")');
  assert.deepEqual(await evaluate('cache.residential.map(x=>x.name).sort()'),['Ads2','ads1']);
  assert.equal(await evaluate('document.querySelector("#content").textContent.includes("pass:colon")'),false);
  await evaluate('Object.defineProperty(navigator,"clipboard",{configurable:true,value:{writeText:async text=>{window.fixtureClipboard=text}}})');
  await evaluate('document.querySelector("[data-residential=copy-all]").click()');
  await waitFor('typeof window.fixtureClipboard==="string"');
  assert.equal((await evaluate('window.fixtureClipboard')).split('\n').length,2);
  assert.ok((await evaluate('window.fixtureClipboard')).includes('pass:colon'));
  const snap=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,'residential-mobile.png'),Buffer.from(snap.data,'base64'));
  assert.equal(await evaluate('document.documentElement.scrollWidth<=window.innerWidth+1'),true);
  await call('Emulation.setDeviceMetricsOverride',{width:1365,height:950,deviceScaleFactor:1,mobile:false});
  const desktop=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,'residential-desktop.png'),Buffer.from(desktop.data,'base64'));
  await evaluate('document.querySelector("[data-residential-id]").click();window.fixtureClipboard=null;document.querySelector("[data-residential=copy-selected]").click()');
  await waitFor('typeof window.fixtureClipboard==="string"');assert.equal((await evaluate('window.fixtureClipboard')).split('\n').length,1);
  await evaluate('window.confirm=()=>false;document.querySelector("[data-residential=delete-selected]").click()');
  assert.equal(await evaluate('cache.residential.length'),2);
  await evaluate('window.confirm=()=>true;document.querySelector("[data-residential=delete-selected]").click()');
  await waitFor('cache.residential.length===1');
  await evaluate('document.querySelector("[data-residential=delete-all]").click()');
  await waitFor('cache.residential.length===0');
  assert.equal(await evaluate('localStorage.getItem("fixtureClipboard")'),null);
  console.log('UI_BULK_EXACT_NAMES_COPY_ALL_SELECTED_DELETE_CONFIRMATION_AND_MOBILE PASS');
 }finally{
  if(ws)ws.close();chrome.kill('SIGTERM');fs.closeSync(log);
  await new Promise(r=>setTimeout(r,500));fs.rmSync(profile,{recursive:true,force:true});
 }
})().catch(e=>{console.error(e.message);process.exitCode=1});
