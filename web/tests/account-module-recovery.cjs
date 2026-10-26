// Real mobile Chromium, real account UI assets; isolated API fixtures only.
// Deliberately lose provider scripts, then exercise visible validation and Save.
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),http=require('node:http');
const {spawn}=require('node:child_process'),assert=require('node:assert/strict');
(async()=>{
 const root=fs.mkdtempSync(path.join(os.tmpdir(),'dob-account-modules-'));
 const blocked=new Set(['aws-catalog.js','ovh-catalog.js']),scriptRequests=[],previews=[],saves=[];
 let delayedModule='',releaseModule=null,apiError='';
 const providers=['digitalocean','vultr','upcloud','linode','aws','ovh'];
 const server=http.createServer(async(req,res)=>{
  const u=new URL(req.url,'http://localhost');
  res.setHeader('Cache-Control','no-store');
  res.setHeader('Content-Security-Policy',"default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:");
  const json=(status,data)=>{res.writeHead(status,{'Content-Type':'application/json'});res.end(JSON.stringify(data))};
  if(u.pathname.startsWith('/static/')){
   const name=path.basename(u.pathname);scriptRequests.push(name);
   if(blocked.has(name)){res.writeHead(503);res.end();return}
   if(name===delayedModule){delayedModule='';await new Promise(resolve=>{releaseModule=resolve})}
   const file=path.join(process.cwd(),'web/static',name);
   if(!fs.existsSync(file)){res.writeHead(404);res.end();return}
   res.setHeader('Content-Type',name.endsWith('.js')?'text/javascript':name.endsWith('.css')?'text/css':'text/html');
   res.end(fs.readFileSync(file));return;
  }
  if(u.pathname==='/admin/'){res.setHeader('Content-Type','text/html');res.end(fs.readFileSync('web/static/index.html'));return}
  if(u.pathname==='/api/v1/auth/me'){json(200,{id:'fixture',role:'admin'});return}
  if(u.pathname==='/api/v1/providers'){json(200,providers.map(name=>({name,display_name:name,status:'ready'})));return}
  if(u.pathname==='/api/v1/proxies'||u.pathname==='/api/v1/accounts'&&req.method==='GET'){json(200,[]);return}
  if(u.pathname==='/readyz'){json(200,{status:'ready',checks:[{name:'database',ok:true}]});return}
  if(req.method==='POST'&&['/api/v1/accounts/preview','/api/v1/accounts'].includes(u.pathname)){
   let raw='';for await(const chunk of req)raw+=chunk;
   const body=JSON.parse(raw);
   if(!providers.includes(body.provider)){json(400,{error:'bad provider'});return}
   if(u.pathname.endsWith('/preview')){
    previews.push(body);
    if(apiError){json(401,{error:'provider_credential_invalid',detail:apiError});return}
    const d={provider:body.provider,account:{id:'fixture-'+body.provider},regions:[{ID:'region-1',Name:'Region one',Available:true}],
     plans:[{ID:'plan-1',Available:true,CPU:1,MemoryMB:1024,DiskGB:20,PriceMonthly:6}],images:[{ID:'ubuntu',Available:true,Family:'ubuntu',Version:'24.04'}],defaults:{region:'region-1'},policy:{},server_limit:10};
    if(body.provider==='aws')d.onboarding={complete:true,network_approval:true,resolved:{managed_network:true,region:'region-1'}};
    json(200,d);return;
   }
   saves.push(body);json(201,{id:'fixture-'+body.provider});return;
  }
  json(404,{error:'fixture route missing'});
 });
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 const base='http://127.0.0.1:'+server.address().port;
 const profile=path.join(root,'chrome'),log=fs.openSync(path.join(root,'browser.log'),'w');
 const chrome=spawn(process.env.DOB_TEST_CHROMIUM||'/snap/bin/chromium',
  ['--headless','--no-sandbox','--disable-dev-shm-usage','--disable-gpu','--remote-debugging-port=0','--remote-debugging-address=127.0.0.1','--user-data-dir='+profile,'about:blank'],
  {stdio:['ignore',log,log]});
 let ws;
 try{
  let port;for(let i=0;i<100;i++){try{port=Number(fs.readFileSync(path.join(profile,'DevToolsActivePort'),'utf8').split('\n')[0]);break}catch{}await new Promise(r=>setTimeout(r,100))}
  assert(port,'browser startup');
  const version=await(await fetch('http://127.0.0.1:'+port+'/json/version')).json();
  ws=new WebSocket(version.webSocketDebuggerUrl);await new Promise((resolve,reject)=>{ws.onopen=resolve;ws.onerror=reject});
  let seq=0;const pending=new Map(),errors=[];
  ws.onmessage=({data})=>{const m=JSON.parse(data);if(m.id){const p=pending.get(m.id);if(p){pending.delete(m.id);m.error?p[1](Error(m.error.message)):p[0](m.result)}}else if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails.exception?.description||m.params.exceptionDetails.text)};
  const send=(method,params={},sessionId)=>new Promise((resolve,reject)=>{const id=++seq;pending.set(id,[resolve,reject]);ws.send(JSON.stringify({id,method,params,sessionId}))});
  const {targetId}=await send('Target.createTarget',{url:'about:blank'});
  const {sessionId}=await send('Target.attachToTarget',{targetId,flatten:true}),call=(m,p)=>send(m,p,sessionId);
  await call('Page.enable');await call('Runtime.enable');
  await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
  await call('Page.addScriptToEvaluateOnNewDocument',{source:'if(location.origin==='+JSON.stringify(base)+')localStorage.setItem("token","fixture")'});
  const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(r.exceptionDetails.exception?.description||r.exceptionDetails.text);return r.result.value};
  const wait=async expression=>{for(let i=0;i<150;i++){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,50))}throw Error('Timed out: '+expression+'; '+await evaluate('document.querySelector("#modalError")?.textContent'))};
  await call('Page.navigate',{url:base+'/admin/#accounts'});
  await wait('typeof activePageView!=="undefined"&&current==="accounts"&&!activePageView.loading');
  assert.equal(await evaluate('typeof AWSCatalog'),'undefined');
  assert.equal(await evaluate('typeof OVHCatalog'),'undefined');
  const open=async provider=>{
   await evaluate('openAccountWizard()');
   await evaluate('(()=>{const p=document.querySelector("[name=provider]");p.value='+JSON.stringify(provider)+';p.dispatchEvent(new Event("change"));document.querySelector("[name=name]").value="Fixture";document.querySelector("[name=token]").value='+JSON.stringify(provider==='aws'?'{"secret_access_key":"fixture-secret"}':'fixture-token')+'})()');
  };
  const validate=()=>evaluate('document.querySelector("#discoverAccount").click()');
  const ready=()=>wait('cache.accountPreview!==null&&!document.querySelector("#discoverAccount").disabled&&document.querySelector("#modalError").textContent===""');
  const save=async provider=>{
   const before=saves.length;
   await evaluate('document.querySelector("#modalSubmit").click()');
   await wait('!document.querySelector("#modal").open');
   assert.equal(saves.length,before+1,'one account save');
   const b=saves.at(-1);assert.equal(b.provider,provider);assert.equal(b.region,'region-1');assert.equal(b.size,'plan-1');assert.equal(b.image,'ubuntu');
   assert.equal(b.token,provider==='aws'?'{"secret_access_key":"fixture-secret","managed_network":true,"region":"region-1"}':'fixture-token');
  };
  for(const provider of providers.slice(0,4)){
   const count=scriptRequests.filter(x=>['aws-catalog.js','ovh-catalog.js'].includes(x)).length;
   await open(provider);await validate();await ready();
   assert.equal(await evaluate('document.querySelector("#modalSubmit").disabled'),false);
   await save(provider);
   assert.equal(scriptRequests.filter(x=>['aws-catalog.js','ovh-catalog.js'].includes(x)).length,count,'unrelated provider adapter is never loaded');
  }
  console.log('PASS four providers validate/save with AWS and OVH scripts unavailable');

  await open('aws');const before=previews.length;
  await validate();await wait('document.querySelector("#modalError").textContent.includes("AWSCatalog")&&!document.querySelector("#discoverAccount").disabled');
  assert.equal(previews.length,before,'module failure happens before API preview');
  assert.equal(await evaluate('document.querySelector("#modalSubmit").disabled&&cache.accountPreview===null'),true);
  assert.equal(await evaluate('document.querySelector("[name=token]").value'),'{"secret_access_key":"fixture-secret"}');
  blocked.delete('aws-catalog.js');
  await validate();await ready();
  assert.equal(await evaluate('document.querySelector("#modalSubmit").disabled'),true,'AWS consent cannot be bypassed');
  await evaluate('document.querySelector("[name=approve_managed_network]").click()');
  assert.equal(await evaluate('document.querySelector("#modalSubmit").disabled'),false);
  await save('aws');
  console.log('PASS AWS recoverable module error preserves credentials, then enforces consent and saves');

  blocked.delete('ovh-catalog.js');await open('ovh');await validate();await ready();await save('ovh');
  console.log('PASS OVH missing catalog recovered without AWS dependency');

  // Missing shared modules recover before mounting the form.
  await evaluate('delete window.AccountCatalog;delete window.PreviewStream;delete window.ProviderAuthorization');
  await open('vultr');await validate();await ready();await save('vultr');
  console.log('PASS shared account modules recover and account Save completes');

  // A delayed asset must not apply the closed wizard's AWS preview to a new Vultr form.
  await evaluate('delete window.AWSCatalog');
  delayedModule='aws-catalog.js';await open('aws');const beforeStale=previews.length;await validate();
  for(let i=0;i<100&&!releaseModule;i++)await new Promise(r=>setTimeout(r,20));
  assert(releaseModule,'asset read was delayed');
  await evaluate('document.querySelector("#modalCancel").click()');
  await open('vultr');releaseModule();releaseModule=null;
  await validate();await ready();assert.equal(previews.length,beforeStale+1);assert.equal(previews.at(-1).provider,'vultr');
  await save('vultr');
  console.log('PASS delayed provider module cannot submit or overwrite a newer wizard');

  await open('vultr');apiError='Provider rejected these credentials';
  await validate();await wait('document.querySelector("#modalError").textContent.includes("Provider rejected")&&!document.querySelector("#discoverAccount").disabled');
  assert.equal(await evaluate('document.querySelector("#modalSubmit").disabled&&cache.accountPreview===null'),true);
  assert.equal(await evaluate('document.querySelector("[name=token]").value'),'fixture-token');
  assert.deepEqual(errors,[],'no uncaught browser exceptions');
  console.log('PASS genuine provider rejection remains blocked; no script exception or mutation replay');
 }finally{
  releaseModule?.();ws?.close();chrome.kill('SIGTERM');server.closeAllConnections();
  await new Promise(resolve=>server.close(resolve));fs.closeSync(log);
 }
})().catch(e=>{console.error(e);process.exitCode=1});
