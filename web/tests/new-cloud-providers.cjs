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
  const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error('page evaluation failed: '+(r.exceptionDetails.exception?.description||r.exceptionDetails.text));return r.result.value};

  await call('Page.navigate',{url:base+'/admin/#accounts'});
  async function waitFor(expression){const end=Date.now()+12000;while(Date.now()<end){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,100))}throw Error('UI assertion timeout: '+expression+'; state='+JSON.stringify(await evaluate('({url:location.href,ready:document.readyState,viewType:typeof activePageView,current:typeof current==="undefined"?null:current,loading:typeof activePageView==="undefined"?null:activePageView?.loading,appHidden:document.querySelector("#app")?.hidden,handler:typeof document.querySelector("#primaryAction")?.onclick,rendered:document.querySelector("#content")?.dataset.renderedPage})')))}
  // Wait for the route and event handlers, not just the static HTML button.
  await waitFor('typeof activePageView!=="undefined"&&current==="accounts"&&activePageView&&!activePageView.loading&&!document.querySelector("#app").hidden&&typeof document.querySelector("#primaryAction").onclick==="function"');
  await evaluate('document.querySelector("#primaryAction").click()');
  await waitFor('document.querySelector("#modalFields [name=provider]")!==null');
  assert.deepEqual((await evaluate('[...document.querySelector("#modalFields [name=provider]").options].map(x=>x.value)')).sort(),['aws','digitalocean','linode','ovh','upcloud','vultr']);
  for(const provider of ['aws','ovh']){
   await evaluate('document.querySelector("#modalFields [name=provider]").value='+JSON.stringify(provider)+';document.querySelector("#modalFields [name=provider]").dispatchEvent(new Event("change"))');
   const fields=await evaluate('[...document.querySelectorAll("[data-provider-credential]")].map(x=>({key:x.dataset.providerCredential,type:x.type}))');
   assert.equal(fields.length,provider==='aws'?6:5);
   for(const f of fields.filter(x=>/secret|key|token/.test(x.key)&&x.key!=='project_id'))assert.equal(f.type,'password');
   const credentialSelector='[data-provider-credential="'+(provider==='aws'?'access_key_id':'application_key')+'"]';
   await evaluate('(()=>{const el=document.querySelector('+JSON.stringify(credentialSelector)+');el.value="fixture-credential";el.dispatchEvent(new Event("input"))})()');
   const serialized=JSON.parse(await evaluate('document.querySelector("#modalFields [name=token]").value'));
   assert.equal(serialized[provider==='aws'?'access_key_id':'application_key'],'fixture-credential');
   if(provider==='aws'){assert.equal(serialized.region,'eu-west-1');assert.deepEqual(serialized.security_group_ids,[])}else assert.equal(serialized.endpoint,'ovh-eu');
   assert.equal(await evaluate('cache.accountPreview===null&&document.querySelector("#modalSubmit").hidden'),true);
   assert.equal(await evaluate('document.documentElement.scrollWidth<=window.innerWidth+1'),true);
   const snap=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,provider+'-account-mobile.png'),Buffer.from(snap.data,'base64'));
  }
  await evaluate('document.querySelector("#modalFields [name=provider]").value="linode";document.querySelector("#modalFields [name=provider]").dispatchEvent(new Event("change"))');
  assert.equal(await evaluate('document.querySelector("#modalFields [name=token]").value'),'');
  assert.equal(await evaluate('document.querySelectorAll("[data-provider-credential]").length'),0);
  assert.equal(await evaluate('document.querySelector("#modalFields [name=token]").type'),'password');
  console.log('NEW_CLOUD_DRIVER_CREDENTIAL_FIELDS_ISOLATION_MOBILE PASS');
 }finally{
  if(ws)ws.close();chrome.kill('SIGTERM');fs.closeSync(log);
  await new Promise(r=>setTimeout(r,500));fs.rmSync(profile,{recursive:true,force:true});
 }
})().catch(e=>{console.error(e.stack);process.exitCode=1});
