// Real mobile Chromium, real account UI assets; isolated API fixtures only.
// Deliberately lose provider scripts, then exercise visible validation and Save.
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),http=require('node:http');
const {spawn}=require('node:child_process'),assert=require('node:assert/strict');
(async()=>{
 const root=fs.mkdtempSync(path.join(os.tmpdir(),'dob-account-modules-'));
 const blocked=new Set(['aws-catalog.js','ovh-catalog.js']),scriptRequests=[],previews=[],saves=[];
 let delayedModule='',releaseModule=null,apiError='',realCredentials=false,delayPreview=false,releasePreview=null;
 const providers=['digitalocean','vultr','upcloud','linode','kamatera','aws','ovh'];
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
  if(u.pathname==='/api/v1/providers'){json(200,providers.map(name=>({name,display_name:name,status:'ready',...(realCredentials&&name==='aws'?{credential_fields:[{key:'access_key_id',label:'Access key ID',secret:true},{key:'secret_access_key',label:'Secret access key',secret:true},{key:'session_token',label:'Session token',secret:true,optional:true}]}:{})})));return}
  if(u.pathname==='/api/v1/proxies'){json(200,[{id:'fixture-proxy',name:'Fixture proxy',status:'healthy'}]);return}
  if(u.pathname==='/api/v1/proxy-drivers'){json(200,{drivers:[],country_options:[{code:'US',label:'United States'},{code:'DE',label:'Germany'}]});return}
  if(u.pathname==='/api/v1/accounts'&&req.method==='GET'){json(200,[]);return}
  if(u.pathname==='/readyz'){json(200,{status:'ready',checks:[{name:'database',ok:true}]});return}
  if(req.method==='POST'&&['/api/v1/accounts/preview','/api/v1/accounts'].includes(u.pathname)){
   let raw='';for await(const chunk of req)raw+=chunk;
   const body=JSON.parse(raw);
   if(!providers.includes(body.provider)){json(400,{error:'bad provider'});return}
   if(u.pathname.endsWith('/preview')){
    previews.push(body);
    if(delayPreview){delayPreview=false;await new Promise(resolve=>{releasePreview=resolve})}
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
  const wait=async (expression,limit=150)=>{for(let i=0;i<limit;i++){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,50))}throw Error('Timed out: '+expression+'; '+await evaluate('document.querySelector("#modalError")?.textContent'))};
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
  for(const provider of providers.slice(0,5)){
   const count=scriptRequests.filter(x=>['aws-catalog.js','ovh-catalog.js'].includes(x)).length;
   await open(provider);await validate();await ready();
   assert.equal(await evaluate('document.querySelector("#modalSubmit").disabled'),false);
   await save(provider);
   assert.equal(scriptRequests.filter(x=>['aws-catalog.js','ovh-catalog.js'].includes(x)).length,count,'unrelated provider adapter is never loaded');
  }
  console.log('PASS five providers validate/save with AWS and OVH scripts unavailable');

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

  // A timed-out caller must share the original still-loading script on retry.
  await evaluate('delete window.AWSCatalog');
  delayedModule='aws-catalog.js';await open('aws');const beforeTimeout=previews.length;
  const beforeScripts=scriptRequests.filter(x=>x==='aws-catalog.js').length;
  await validate();
  await wait('document.querySelector("#modalError").textContent.includes("AWSCatalog")&&!document.querySelector("#discoverAccount").disabled',260);
  assert(releaseModule);assert.equal(previews.length,beforeTimeout);
  await validate();await new Promise(r=>setTimeout(r,100));
  assert.equal(scriptRequests.filter(x=>x==='aws-catalog.js').length,beforeScripts+1,'timeout retry shares the original load');
  releaseModule();releaseModule=null;await ready();
  assert.equal(previews.length,beforeTimeout+1,'only the current preview can proceed');
  assert.equal(await evaluate('document.querySelector("#modalSubmit").disabled'),true);
  await evaluate('document.querySelector("[name=approve_managed_network]").click()');await save('aws');
  console.log('PASS timeout/retry/late response uses one script and preserves AWS consent');

  await open('vultr');apiError='Provider rejected these credentials';
  await validate();await wait('document.querySelector("#modalError").textContent.includes("Provider rejected")&&!document.querySelector("#discoverAccount").disabled');
  assert.equal(await evaluate('document.querySelector("#modalSubmit").disabled&&cache.accountPreview===null'),true);
  // A prior native close event can arrive after this wizard has opened.
  // Every shared-dialog listener must leave its validation and handlers intact.
  await evaluate('document.querySelector("#modal").dispatchEvent(new Event("close"))');
  assert.equal(await evaluate('document.querySelector("#modalSubmit").disabled&&cache.accountPreview===null'),true,'delayed close preserves failed validation');
  assert.equal(await evaluate('document.querySelector("[name=token]").value'),'fixture-token');
  await evaluate('document.querySelector("#modalCancel").click();delete window.AccountCatalog');
  blocked.add('account-catalog.js');
  await evaluate('document.querySelector("#primaryAction").click()');
  await wait('document.querySelector("#toast").textContent.includes("Unable to open form")');
  assert.equal(await evaluate('document.querySelector("#modal").open'),false);
  assert.match(await evaluate('document.querySelector("#toast").textContent'),/AccountCatalog/);
  blocked.delete('account-catalog.js');apiError='';
  await open('vultr');await validate();await ready();await save('vultr');
  console.log('PASS unavailable shared module has visible opening error and recovery');

  // Password managers may update .value with no input event, or emit only change.
  // The real provider metadata intentionally renders credential fields without names.
  realCredentials=true;
  const fillCredentials=async (secret='fixture-secret-current',event='')=>evaluate('(()=>{for(const [key,value] of Object.entries('+JSON.stringify({access_key_id:'ASIAFIXTUREONLY000001',secret_access_key:secret,session_token:'fixture-session-current'})+')){const el=document.querySelector("[data-provider-credential="+key+"]");el.value=value;'+(event?'el.dispatchEvent(new Event('+JSON.stringify(event)+',{bubbles:true}));':'')+'}})()');
  const consent=()=>evaluate('document.querySelector("[name=approve_managed_network]").click()');
  await open('aws');await fillCredentials();await validate();await ready();
  assert.deepEqual(JSON.parse(previews.at(-1).token),{access_key_id:'ASIAFIXTUREONLY000001',secret_access_key:'fixture-secret-current',session_token:'fixture-session-current'},'validation reads the visible fields even without input events');
  await consent();
  const beforeCredentialSave=saves.length;
  await fillCredentials('fixture-secret-replaced');
  await evaluate('document.querySelector("#modalSubmit").click()');
  assert.equal(saves.length,beforeCredentialSave,'changed silent credential must not save against an earlier validation');
  assert.equal(await evaluate('cache.accountPreview===null&&document.querySelector("#modalSubmit").disabled'),true);
  assert.match(await evaluate('document.querySelector("#modalError").textContent'),/changed.*validate again/i);
  await validate();await ready();await consent();
  await fillCredentials('fixture-secret-change-only','change');
  assert.equal(await evaluate('cache.accountPreview===null&&document.querySelector("#modalSubmit").disabled'),true,'change-only autofill invalidates validation');
  await validate();await ready();await consent();
  await evaluate('document.querySelector("#modalSubmit").click()');
  await wait('!document.querySelector("#modal").open');
  assert.equal(saves.length,beforeCredentialSave+1);
  assert.equal(JSON.parse(saves.at(-1).token).secret_access_key,'fixture-secret-change-only');
  assert.equal(JSON.parse(saves.at(-1).token).session_token,'fixture-session-current');
  console.log('PASS visible AWS credentials are captured at validation; silent and change-only edits require revalidation before Save');

  await open('aws');await fillCredentials();delayPreview=true;await validate();
  for(let i=0;i<100&&!releasePreview;i++)await new Promise(r=>setTimeout(r,20));
  assert(releasePreview,'preview fixture was delayed');
  await fillCredentials('fixture-secret-during-preview');releasePreview();releasePreview=null;
  await wait('!document.querySelector("#discoverAccount").disabled');
  assert.equal(await evaluate('cache.accountPreview===null&&document.querySelector("#modalSubmit").disabled'),true,'in-flight preview cannot validate silently changed inputs');
  await validate();await ready();await consent();
  await evaluate('document.querySelector("[name=proxy_id]").value="fixture-proxy";document.querySelector("#modalSubmit").click()');
  assert.equal(saves.length,beforeCredentialSave+1,'primary route is bound to validation');
  assert.equal(await evaluate('cache.accountPreview===null&&document.querySelector("#modalSubmit").disabled'),true);
  await evaluate('document.querySelector("#modalCancel").click()');
  console.log('PASS delayed AWS response and silent primary proxy edit cannot reuse stale validation');

  await open('vultr');
  assert.equal(await evaluate('document.querySelector("[name=proxy_country]").value'),'AUTO');
  await evaluate('document.querySelector("[name=proxy_id]").value="fixture-proxy";document.querySelector("[name=proxy_country]").value="US"');
  await validate();await ready();
  assert.equal(previews.at(-1).proxy_country,'US','country selected before provider validation');
  const beforeCountrySave=saves.length;
  await evaluate('document.querySelector("[name=proxy_country]").value="DE";document.querySelector("#modalSubmit").click()');
  assert.equal(saves.length,beforeCountrySave,'silent country change requires new proof');
  assert.equal(await evaluate('cache.accountPreview===null&&document.querySelector("#modalSubmit").disabled'),true);
  await validate();await ready();
  assert.equal(previews.at(-1).proxy_country,'DE');
  await save('vultr');
  assert.equal(saves.at(-1).proxy_country,'DE','country committed with account');
  console.log('PASS country defaults AUTO and preview/save cannot cross a changed country');

  await open('vultr');await validate();await ready();
  await evaluate('document.querySelector("[name=token]").value="fixture-replaced-token";document.querySelector("#modalSubmit").click()');
  assert.equal(saves.length,beforeCountrySave+1,'raw token provider is also bound to validation');
  assert.equal(await evaluate('cache.accountPreview===null&&document.querySelector("#modalSubmit").disabled'),true);
  await validate();await ready();
  await evaluate('document.querySelector("[name=token]").dispatchEvent(new Event("change"))');
  assert.equal(await evaluate('cache.accountPreview===null&&document.querySelector("#modalSubmit").disabled'),true,'change-only raw token invalidates validation');
  await evaluate('document.querySelector("#modalCancel").click()');
  console.log('PASS raw token credentials use the same validation boundary');

  assert.deepEqual(errors,[],'no uncaught browser exceptions');
  console.log('PASS genuine provider rejection remains blocked; no script exception or mutation replay');
 }finally{
  releaseModule?.();releasePreview?.();ws?.close();chrome.kill('SIGTERM');server.closeAllConnections();
  await new Promise(resolve=>server.close(resolve));fs.closeSync(log);
 }
})().catch(e=>{console.error(e);process.exitCode=1});
