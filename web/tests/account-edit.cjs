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
  await call('Page.addScriptToEvaluateOnNewDocument',{source:'if(location.origin==='+JSON.stringify(base)+'){window.__authWindows=[];window.__authOpenCalls=0;window.open=()=>{window.__authOpenCalls++;if('+JSON.stringify(process.env.DOB_TEST_AUTH_POPUP_BLOCKED==='1')+')return null;const w={opener:window,closed:false,document:document.implementation.createHTMLDocument(""),location:{href:"about:blank",replace(url){this.href=url}},close(){this.closed=true}};window.__authWindows.push(w);return w}}'});

  const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error('page evaluation failed: '+(r.exceptionDetails.exception?.description||r.exceptionDetails.text));return r.result.value};

  await call('Page.navigate',{url:base+'/admin/#accounts'});
  async function waitFor(expression){const end=Date.now()+12000;while(Date.now()<end){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,100))}throw Error('UI assertion timeout: '+expression+'; state='+JSON.stringify(await evaluate('({url:location.href,ready:document.readyState,viewType:typeof activePageView,current:typeof current==="undefined"?null:current,loading:typeof activePageView==="undefined"?null:activePageView?.loading,appHidden:document.querySelector("#app")?.hidden,handler:typeof document.querySelector("#primaryAction")?.onclick,rendered:document.querySelector("#content")?.dataset.renderedPage})')))}
  // Wait for the route and event handlers, not just the static HTML button.
  await waitFor('typeof activePageView!=="undefined"&&current==="accounts"&&activePageView&&!activePageView.loading&&!document.querySelector("#app").hidden&&typeof document.querySelector("#primaryAction").onclick==="function"');

  for(const provider of ['aws','ovh','vultr','digitalocean','linode','upcloud','kamatera']){
   await evaluate('window.__editAlerts=[];window.alert=m=>window.__editAlerts.push(m);document.querySelector("#modalSubmit").disabled=true;document.querySelector("#modalError").textContent="previous error";window.editAccount('+JSON.stringify(provider)+')');
   assert.deepEqual(await evaluate('window.__editAlerts'),[],provider+' opens without JS errors');
   await waitFor('document.querySelector("#modal").open');
   // Reproduce a queued close from a preceding dialog after the next one opens.
   await evaluate('document.querySelector("#modal").dispatchEvent(new Event("close"))');
   assert.equal(await evaluate('document.querySelector("#modalForm").classList.contains("accountForm")'),true,provider+' ignores stale close while open');

   assert.equal(await evaluate('document.querySelector("#modalSubmit").hidden||document.querySelector("#modalSubmit").disabled'),false,provider+' Save usable');
   assert.equal(await evaluate('document.querySelector("#modalError").textContent'),'');
   for(const name of ['size2','size3','image2','image3','proxy_id2','proxy_id3','region2','region3']){
    assert.equal(await evaluate('document.querySelector("[name='+name+']").value'),'',provider+' preserves optional '+name);
   }
   for(const size of [{width:320,height:480},{width:390,height:420},{width:844,height:390},{width:390,height:844}]){
    await call('Emulation.setDeviceMetricsOverride',{...size,deviceScaleFactor:1,mobile:true});
    for(const scroll of [0,99999]){
     const bounds=await evaluate('(()=>{document.querySelector("#modalFields").scrollTop='+scroll+';const e=document.querySelector("#modalSubmit"),r=e.getBoundingClientRect(),v=window.visualViewport;return {top:r.top,bottom:r.bottom,height:r.height,viewport:v?.height||innerHeight,hit:document.elementFromPoint(r.x+r.width/2,r.y+r.height/2)?.id}})()');
     assert.ok(bounds.top>=0&&bounds.bottom<=bounds.viewport+1&&bounds.height>=40,provider+' visible footer '+JSON.stringify(bounds));assert.equal(bounds.hit,'modalSubmit');
    }
   }
   await evaluate(provider==='aws'?'document.querySelector("#modalForm").requestSubmit()':'document.querySelector("#modalSubmit").click()');
   await waitFor('!document.querySelector("#modal").open');
   await waitFor('!document.querySelector("#modalForm").classList.contains("accountForm")');
   await waitFor('cache.accounts!==null&&activePageView&&!activePageView.loading');
  }
  await evaluate('load("settings")');
  await waitFor('current==="settings"&&cache.settings!==null&&!activePageView.loading');
  await evaluate('document.querySelector("[data-action=edit-settings]").click()');
  await waitFor('document.querySelector("#modal").open');
  assert.equal(await evaluate('document.querySelector("#modalFields [name=listen_port]").value'),'28080');
  assert.equal(await evaluate('document.querySelector("#modalFields input[disabled]").value'),'8443');
  await evaluate('document.querySelector("#modalSubmit").click()');
  await waitFor('!document.querySelector("#modal").open');
  await waitFor('current==="settings"&&!activePageView.loading');
  assert.equal(await evaluate('document.querySelector("#content").textContent.includes("Public panel port: 8443")'),true);
  console.log('ACCOUNT_EDIT_ALL_SIX_PROVIDERS_OPTIONAL_VALUES_AND_MOBILE_SAVE PASS');
 }finally{
  if(ws)ws.close();chrome.kill('SIGTERM');fs.closeSync(log);
  await new Promise(r=>setTimeout(r,500));fs.rmSync(profile,{recursive:true,force:true});
 }
})().catch(e=>{console.error(e.stack);process.exitCode=1});
