// Real mobile Chromium regression for the single configuration lifecycle.
const fs=require('node:fs'),path=require('node:path'),http=require('node:http'),os=require('node:os'),{spawn}=require('node:child_process'),assert=require('node:assert/strict');
(async()=>{
 const root=process.env.DOB_UI_ARTIFACT_DIR||fs.mkdtempSync(path.join(os.tmpdir(),'dob-unified-ui-'));
 fs.mkdirSync(root,{recursive:true});const staticRoot=path.resolve(__dirname,'../static');
 const saves=[],deletions=[],requests=[];
 let catalog={unified:true,automatic:true,observed_at:new Date().toISOString(),drivers:[],profiles:[],servers:[{id:'11111111-1111-4111-a111-111111111111',host:'192.0.2.10',region:'de-fra'},{id:'22222222-2222-4222-a222-222222222222',host:'192.0.2.20',region:'nl-ams'}]};
 for(const protocol of ['vless','vmess','trojan','shadowsocks'])for(const network of ['tcp','ws','grpc','xhttp'])for(const security of ['none','tls','reality']){
  const supported=protocol==='vless'&&(security==='reality'&&network!=='ws'||security==='tls'&&network!=='xhttp'||security==='none'&&network==='tcp')||protocol==='vmess'&&network!=='xhttp'&&security!=='reality'||protocol==='trojan'&&(network!=='xhttp'&&security==='tls'||network==='tcp'&&security==='none')||protocol==='shadowsocks'&&network==='tcp'&&security==='none';
  if(supported)catalog.drivers.push({id:[protocol,network,security].join('/'),protocol,network,security,public:true,client_requirement:['vless','trojan'].includes(protocol)&&security==='none'?'plain_tcp_compatible_client':''});
 }
 const server=http.createServer((req,res)=>{
  const url=new URL(req.url,'http://127.0.0.1');requests.push([req.method,url.pathname]);
  const json=(body,status=200)=>{res.writeHead(status,{'Content-Type':'application/json'});res.end(JSON.stringify(body))};
  if(url.pathname.startsWith('/api/v1/configs/profiles')){
   if(url.pathname.endsWith('/servers'))return json({servers:[{host:'192.0.2.10',state:'VERIFIED',applied_revision:1,desired_revision:1,verified_at:new Date().toISOString()}]});
   if(req.method==='GET')return json(catalog);
   let raw='';req.on('data',x=>raw+=x);req.on('end',()=>{
    const body=JSON.parse(raw),id=url.pathname.split('/')[5],current=catalog.profiles.find(x=>x.profile.id===id);
    if(url.pathname.endsWith('/delete-batch')){deletions.push(body);const ids=new Set(body.profiles.map(x=>x.id));catalog.profiles=catalog.profiles.filter(x=>!ids.has(x.profile.id));return json({accepted:ids.size,conflicts:0},202)}
    if(id&&current&&body.revision!==current.profile.revision)return json({error:'profile_revision_changed'},409);
    if(req.method==='DELETE'){deletions.push(body);catalog.profiles=catalog.profiles.filter(x=>x!==current);return json({state:'deleting'},202)}
    saves.push(body);const profile={...body,id:body.id||'profile-'+saves.length,revision:(body.revision||0)+1,panel_ids:body.panel_ids||[],spec:{...body.spec,port:body.spec.port||(body.spec.cloudflare_account?2083:10000+saves.length)}};
    const item={profile,state:profile.enabled?'syncing':'paused',eligible:2,verified:2,ready:0,published:0,target:2*profile.classes.reduce((n,c)=>n+(profile.class_settings?.[c]?.count??profile.clients_per_class),0),pending:2,blocked:0,deleting:0};
    catalog.profiles=[...catalog.profiles.filter(x=>x.profile.id!==profile.id),item];json({profile,state:'queued'});
   });return;
  }
  if(url.pathname.startsWith('/static/')||url.pathname==='/admin/'){
   const name=url.pathname==='/admin/'?'index.html':path.basename(url.pathname),file=path.join(staticRoot,name);
   if(!fs.existsSync(file)){res.writeHead(404);res.end();return}
   res.setHeader('Content-Type',name.endsWith('.js')?'text/javascript':name.endsWith('.css')?'text/css':'text/html');res.end(fs.readFileSync(file));return;
  }
  const data={'/api/v1/configs/cloudflare':{"accounts": [{"id": "11111111-1111-4111-8111-111111111111", "name": "Cloudflare fixture", "enabled": true, "revision": 1, "manage_ssl": true, "error_code": "", "record_count": 0, "records": [], "zones": [{"id": "zone-a", "name": "example.com", "status": "active", "enabled": true, "present": true, "ssl_strict": true}]}]},'/api/v1/project-update':{available:true,status:'succeeded',current_revision:'fixture'},'/api/v1/accounts':[],'/api/v1/build-activity':[],'/api/v1/provision-errors':[]};json(data[url.pathname]||{status:'ready',checks:[],active_servers:2});
 });
 await new Promise(r=>server.listen(0,'127.0.0.1',r));const base='http://127.0.0.1:'+server.address().port;
 const profile=fs.mkdtempSync(path.join(root,'ui-profile-')),log=fs.openSync(path.join(root,'browser.log'),'w');
 const chrome=spawn('/snap/bin/chromium',['--headless','--no-sandbox','--disable-dev-shm-usage','--disable-gpu','--remote-debugging-port=0','--remote-debugging-address=127.0.0.1','--user-data-dir='+profile,'about:blank'],{stdio:['ignore',log,log],env:{...process.env,TMPDIR:root}});
 let ws;const errors=[],passed=[];
 try{
  let port;const until=Date.now()+20000;
  while(Date.now()<until){try{port=Number(fs.readFileSync(path.join(profile,'DevToolsActivePort'),'utf8').split('\n')[0]);break}catch{}await new Promise(r=>setTimeout(r,100))}
  if(!port)throw Error('Browser did not start');
  const version=await(await fetch('http://127.0.0.1:'+port+'/json/version')).json();ws=new WebSocket(version.webSocketDebuggerUrl);await new Promise((r,j)=>{ws.onopen=r;ws.onerror=j});
  let seq=0;const pending=new Map();ws.onmessage=({data})=>{const m=JSON.parse(data);if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails);if(m.id){const h=pending.get(m.id);if(h){pending.delete(m.id);m.error?h[1](Error(m.error.message)):h[0](m.result)}}};
  const send=(method,params={},sessionId)=>new Promise((resolve,reject)=>{const id=++seq;pending.set(id,[resolve,reject]);ws.send(JSON.stringify({id,method,params,sessionId}))});
  const {targetId}=await send('Target.createTarget',{url:'about:blank'}),{sessionId}=await send('Target.attachToTarget',{targetId,flatten:true});const call=(m,p)=>send(m,p,sessionId);
  await call('Page.enable');await call('Runtime.enable');await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
  const init=`if(location.origin===${JSON.stringify(base)}){
   localStorage.setItem('token','local-read-fixture');window.confirm=()=>true;window.fixtureAlerts=[];window.alert=msg=>fixtureAlerts.push(msg);
   window.fixtureCalls=[];window.fixtureHolds=[];window.fixtureQueue=[];window.fixtureIntervals=[];
   window.setInterval=(fn,ms)=>{fixtureIntervals.push({fn,ms});return fixtureIntervals.length};
   const realFetch=window.fetch.bind(window);
   window.fetch=(input,opt={})=>{const url=new URL(input,location.href).pathname;fixtureCalls.push({url,method:opt.method||'GET'});const i=fixtureQueue.findIndex(x=>x.url===url);if(i<0)return realFetch(input,opt);const item=fixtureQueue.splice(i,1)[0];
    if(item.kind==='hold')return new Promise((resolve,reject)=>fixtureHolds.push({id:item.id,resolve,reject,signal:opt.signal}));
    if(item.kind==='network')return Promise.reject(TypeError('Network offline'));
    return Promise.resolve(new Response(item.kind==='malformed'?'broken-json':JSON.stringify(item.body),{status:item.status||200,headers:{'Content-Type':'application/json'}}));
   };
   window.holdRead=(id,url='/api/v1/config-capacity')=>fixtureQueue.push({id,url,kind:'hold'});
   window.finishRead=(id,body,status=200)=>{const item=fixtureHolds.find(x=>x.id===id);if(!item)throw Error('missing held request '+id);item.resolve(new Response(JSON.stringify(body),{status,headers:{'Content-Type':'application/json'}}))};
  }`;
  await call('Page.addScriptToEvaluateOnNewDocument',{source:init});
  const evaluate=async (expression,userGesture=false)=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true,userGesture});if(r.exceptionDetails)throw Error('Evaluation failed: '+JSON.stringify(r.exceptionDetails));return r.result.value};
  async function waitFor(expression,ms=12000){const end=Date.now()+ms;while(Date.now()<end){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,100))}throw Error('UI assertion timeout: '+expression+' body='+await evaluate('document.body.innerText.slice(-3000)'))}
  const mark=x=>{passed.push(x);console.log('PASS',x)};


  await call('Page.navigate',{url:base+'/admin/#configs'});
  await waitFor("document.querySelector('#configProfilesSlot')?.innerText.includes('No configurations')");
  assert.equal(await evaluate("/Manual client maintenance|Reality transport change history|Apply saved settings/.test(document.body.innerText)"),false);
  assert.equal(requests.some(x=>['/api/v1/configs','/api/v1/config-capacity','/api/v1/client-mutations','/api/v1/configs/transport','/api/v1/configs/trojan'].includes(x[1])),false);
  // HTTP deployment must not depend on crypto.randomUUID's secure-context API.
  await evaluate("Object.defineProperty(crypto,'randomUUID',{value:undefined,configurable:true});document.querySelector('#primaryAction').click()");
  await waitFor("document.querySelector('#modal').open");
  assert.equal(await evaluate("document.querySelector('[name=profile_protocol]').value"),'');
  assert.deepEqual(await evaluate("[...document.querySelector('[name=profile_protocol]').options].map(x=>x.value).sort()"),['','shadowsocks','trojan','vless','vmess']);
  await evaluate("var f=n=>document.querySelector('[name=profile_'+n+']');var change=(n,v)=>{f(n).value=v;f(n).dispatchEvent(new Event('change'))};change('protocol','trojan')");
  assert.deepEqual(await evaluate("[...f('security').options].map(x=>x.value).sort()"),['none','tls']);
  await evaluate("change('security','none')");assert.equal(await evaluate("document.querySelector('#profileCompatibility').innerText.includes('recent official Xray clients reject')"),true);
  await evaluate("change('protocol','vless');change('security','reality');f('name').value='My Reality';f('port').value='443';f('direct_count').value='3';f('residential').checked=false;document.querySelector('#modalSubmit').click()");
  await waitFor("!document.querySelector('#modal').open&&document.querySelector('[data-profile-id]')");
  assert.equal(saves.length,1);assert.equal(saves[0].spec.port,443);assert.equal(saves[0].clients_per_class,3);assert.equal(saves[0].spec.security,'reality');assert.match(saves[0].request_id,/^[0-9a-f-]{36}$/);
  assert.equal(await evaluate("document.querySelector('[data-profile-id]').innerText.includes('0 / 6')"),true);
  mark('one New flow, no default protocol, supported public capabilities, exact published counts');

  await evaluate("document.querySelector('[data-edit-profile]').click()");await waitFor("document.querySelector('#modal').open");
  await evaluate("change('network','grpc');f('path').value='my_service';change('network','ws')");assert.match(await evaluate("f('path').value"),/^\//);
  await evaluate("change('network','grpc')");assert.equal(await evaluate("f('path').value"),'my_service');
  await evaluate("change('network','xhttp')");assert.match(await evaluate("f('path').value"),/^\//);
  await evaluate("change('network','tcp');change('protocol','vmess');change('security','none');f('port').value='500';f('direct_count').value='2';f('fleet').checked=false;f('fleet').dispatchEvent(new Event('change'));document.querySelector('[name=profile_selected]').checked=true;document.querySelector('#modalSubmit').click()");
  await waitFor("!document.querySelector('#modal').open&&document.querySelector('[data-profile-revision=\"2\"]')");assert.equal(saves[1].spec.protocol,'vmess');assert.equal(saves[1].spec.port,500);assert.equal(saves[1].clients_per_class,2);assert.equal(saves[1].panel_ids.length,1);assert.equal('path' in saves[1].spec,false);
  mark('editable protocol/count/port/server scope; transport paths remain independent');
  await evaluate("document.querySelector('[data-edit-profile]').click()");await waitFor("document.querySelector('#modal').open");
  await evaluate("f('direct_count').value='2';f('direct_lifetime').value='5';f('direct_lifetime_unit').value='minutes';f('direct_devices').value='2';f('residential').checked=true;f('residential').dispatchEvent(new Event('change'));f('residential_count').value='5';f('residential_lifetime').value='2';f('residential_lifetime_unit').value='hours';f('residential_devices').value='7';f('residential_publish').checked=false;document.querySelector('#modalSubmit').click()");
  await waitFor("!document.querySelector('#modal').open&&document.querySelector('[data-profile-revision=\"3\"]')");
  assert.equal(saves.at(-1).class_settings.DIRECT.lifetime_seconds,300);assert.equal(saves.at(-1).class_settings.RESIDENTIAL.lifetime_seconds,7200);
  assert.equal(saves.at(-1).class_settings.DIRECT.count,2);assert.equal(saves.at(-1).class_settings.RESIDENTIAL.count,5);assert.equal(saves.at(-1).class_settings.RESIDENTIAL.device_limit,7);assert.equal(saves.at(-1).class_settings.RESIDENTIAL.publish,false);
  mark('Direct and Residential save independent counts, units, device limits and publication');


  await evaluate("document.querySelector('[data-toggle-profile]').click()");await waitFor("document.querySelector('[data-toggle-profile]')?.textContent==='Resume'");assert.equal(saves.at(-1).enabled,false);
  await evaluate("document.querySelector('[data-toggle-profile]').click()");await waitFor("document.querySelector('[data-toggle-profile]')?.textContent==='Pause'");assert.equal(saves.at(-1).enabled,true);
  await evaluate("document.querySelector('[data-edit-profile]').click()");await waitFor("document.querySelector('#modal').open");
  await evaluate("fixtureQueue.push({url:'/api/v1/configs/profiles/profile-1',status:409,body:{error:'profile_revision_changed'}});document.querySelector('#modalSubmit').click()");
  await waitFor("document.querySelector('#modalError').innerText.includes('Reopen')");assert.equal(await evaluate("document.querySelector('#modalSubmit').disabled"),false);
  await evaluate("document.querySelector('#modalCancel').click()");
  mark('pause/resume and stale edit recovery');

  await evaluate("fixtureQueue.push({url:'/api/v1/configs/profiles',kind:'network'});refreshConfigProfiles()");
  await waitFor("document.querySelector('[data-profile-read-error]')");assert.equal(await evaluate("!!document.querySelector('[data-profile-id]')"),true);
  await evaluate("document.querySelector('[data-retry-profiles]').click()");await waitFor("!document.querySelector('[data-profile-read-error]')");
  await evaluate("holdRead('hung-profiles','/api/v1/configs/profiles');window.hungProfiles=refreshConfigProfiles();void 0");
  await waitFor("fixtureHolds.some(x=>x.id==='hung-profiles')");
  await waitFor("document.querySelector('[data-profile-read-error]')",18000);
  assert.equal(await evaluate("fixtureHolds.find(x=>x.id==='hung-profiles').signal.aborted"),true);
  await evaluate("document.querySelector('[data-retry-profiles]').click()");await waitFor("!document.querySelector('[data-profile-read-error]')");
  await evaluate("finishRead('hung-profiles',{drivers:[],profiles:[],servers:[]});hungProfiles");
  assert.equal(await evaluate("!!document.querySelector('[data-profile-id]')"),true);
  mark('hung reads have a hard deadline and recover even if transport ignores abort');
  await evaluate("holdRead('old-profiles','/api/v1/configs/profiles');void refreshConfigProfiles()");await waitFor("fixtureHolds.some(x=>x.id==='old-profiles')");
  await evaluate("location.hash='#dashboard'");await waitFor("!document.querySelector('#configProfilesSlot')");
  await evaluate("location.hash='#configs'");await waitFor("document.querySelector('[data-profile-id]')");
  await evaluate("finishRead('old-profiles',{drivers:[],profiles:[],servers:[],unified:true,observed_at:'2000-01-01T00:00:00Z'})");
  assert.equal(await evaluate("document.querySelector('#configProfilesSlot').innerText.includes('My Reality')"),true);
  mark('failed reads preserve counts with a stale notice; late responses cannot overwrite a new page');

  await evaluate("document.querySelector('#primaryAction').click()");await waitFor("document.querySelector('#modal').open");
  await evaluate("change('protocol','shadowsocks');f('name').value='Independent SS';document.querySelector('#modalSubmit').click()");await waitFor("!document.querySelector('#modal').open&&document.querySelectorAll('[data-profile-id]').length===2");
  assert.equal(saves.at(-1).spec.port,0);assert.equal(saves.at(-1).spec.protocol,'shadowsocks');
  await evaluate("document.querySelector('[data-delete-profile]').click()");await waitFor("document.querySelector('#modal').open");
  await evaluate("document.querySelector('#modalSubmit').click()");await waitFor("!document.querySelector('#modal').open&&document.querySelectorAll('[data-profile-id]').length===1");
  assert.equal(deletions.length,1);assert.equal(await evaluate("document.querySelector('[data-profile-id]').innerText.includes('Independent SS')"),true);
  assert.equal(await evaluate('document.documentElement.scrollWidth<=innerWidth+1'),true);
  const shot=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:false});fs.writeFileSync(path.join(root,'unified-configs-mobile.png'),Buffer.from(shot.data,'base64'));
  mark('automatic port, confirmed delete UI, sibling preservation, mobile layout');
  await evaluate("window.keptDetails=document.querySelector('[data-profile-servers]');keptDetails.open=true");await waitFor("keptDetails.querySelector('table')");
  await evaluate("holdRead('details-once','/api/v1/configs/profiles/'+keptDetails.dataset.profileServers+'/servers');window.detailsRead=refreshProfileServers(keptDetails);void 0");await waitFor("fixtureHolds.some(x=>x.id==='details-once')");
  const detailCalls=await evaluate("fixtureCalls.filter(x=>x.url.endsWith('/servers')).length");
  await evaluate("void refreshProfileServers(keptDetails)");assert.equal(await evaluate("fixtureCalls.filter(x=>x.url.endsWith('/servers')).length"),detailCalls);
  await evaluate("finishRead('details-once',{servers:[]});detailsRead");
  catalog.profiles[0].published++;await evaluate('refreshConfigProfiles()');assert.equal(await evaluate("document.querySelector('[data-profile-servers]')===keptDetails"),true);
  mark('server details coalesce overlapping reads and retain open DOM during aggregate changes');
  assert.equal(await evaluate("profileServerStatus({state:'VERIFIED',removing:true,applied_revision:1,desired_revision:2,code:'profile_class_traffic_retrying'})"),'Queued for removal');
  assert.equal(await evaluate("profileServerStatus({state:'APPLYING',stage:'deleting',removing:true,applied_revision:1,desired_revision:2})"),'Removing clients and listener');
  assert.equal(await evaluate("profileServerStatus({state:'RETIRED',removing:true,code:'profile_delete_unverified'})"),'Removed');
  mark('deletion status overrides revision drift and obsolete connection errors');
  for(const [code,label] of [['profile_routes_refresh_pending','Refreshing verified routes; output resumes automatically'],['profile_output_refresh_pending','Refreshing the native output snapshot'],['profile_output_disabled','Publication is disabled in this profile’s settings']]){
   const value={state:'VERIFIED',applied_revision:1,desired_revision:1,verified_at:new Date().toISOString(),publication_code:code};
   assert.equal(await evaluate('profileServerStatus('+JSON.stringify(value)+')'),label);
   assert.equal(await evaluate('profileServerStatus('+JSON.stringify({...value,removing:true})+')'),'Queued for removal');
  }
  mark('verified runtime exposes publication hold and deletion keeps precedence');

  const reviewed=catalog.profiles.map(x=>({id:x.profile.id,revision:x.profile.revision}));
  await evaluate("document.querySelector('[data-delete-all-profiles]').click()");await waitFor("document.querySelector('#modal').open");assert.equal(deletions.length,1);
  const late={...catalog.profiles[0],profile:{...catalog.profiles[0].profile,id:'late-profile',name:'Created later'}};catalog.profiles.push(late);
  await evaluate("document.querySelector('#modalSubmit').click()");await waitFor("!document.querySelector('#modal').open&&document.querySelectorAll('[data-profile-id]').length===1");assert.deepEqual(deletions.at(-1).profiles,reviewed);assert.equal(catalog.profiles[0].profile.id,'late-profile');
  mark('delete all confirms exact snapshot and excludes later creations');
  await evaluate("document.querySelector('#primaryAction').click()");await waitFor("document.querySelector('#modal').open");
  await evaluate("change('protocol','vless');change('network','ws');change('security','tls');f('name').value='Cloudflare TLS';f('cf_account').value='11111111-1111-4111-8111-111111111111';f('cf_enabled').checked=true;f('cf_enabled').dispatchEvent(new Event('change'))");
  assert.equal(await evaluate("f('direct_insecure').disabled&&f('residential_insecure').disabled&&f('host').disabled"),true);
  await evaluate("change('cf_count','2')");
  assert.equal(await evaluate("f('cf_active').value"),'6');
  assert.equal(await evaluate("document.querySelector('#profileCFScheduleHint').innerText.includes('at least 6')"),true);
  const beforeInvalid=saves.length;
  await evaluate("change('cf_active','4');document.querySelector('#modalSubmit').click()");
  assert.equal(await evaluate("document.querySelector('#modal').open&&!f('cf_active').checkValidity()"),true);
  assert.equal(saves.length,beforeInvalid);
  await evaluate("document.querySelector('#modalForm').dispatchEvent(new Event('submit',{cancelable:true}))");
  await waitFor("document.querySelector('#modalError').innerText.includes('at least 6')");
  assert.equal(saves.length,beforeInvalid);
  await evaluate("change('cf_active','6');change('cf_interval','5')");
  assert.equal(await evaluate("document.querySelector('#profileCFScheduleHint').innerText.includes('needs 26')&&!f('cf_interval').checkValidity()"),true);
  await evaluate("change('cf_interval','30')");
  assert.equal(await evaluate("f('cf_active').checkValidity()&&f('cf_interval').checkValidity()"),true);
  const cfShot=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:false});fs.writeFileSync(path.join(root,'cloudflare-schedule-mobile.png'),Buffer.from(cfShot.data,'base64'));
  mark('Cloudflare screenshot schedule raises capacity to 6; invalid overlap is explained and blocked before POST');

  await evaluate("document.querySelector('#modalSubmit').click()");await waitFor("!document.querySelector('#modal').open&&[...document.querySelectorAll('[data-profile-id]')].some(x=>x.innerText.includes('Cloudflare TLS'))");
  assert.equal(saves.at(-1).spec.cloudflare_min_lifetime_seconds,2700);assert.equal(saves.at(-1).spec.cloudflare_max_lifetime_seconds,3600);assert.equal(saves.at(-1).spec.cloudflare_records,2);assert.equal(saves.at(-1).spec.cloudflare_max_active,6);assert.equal(saves.at(-1).spec.host,'');
  await evaluate("[...document.querySelectorAll('[data-profile-id]')].find(x=>x.innerText.includes('Cloudflare TLS')).querySelector('[data-edit-profile]').click()");await waitFor("document.querySelector('#modal').open");
  assert.equal(await evaluate("f('cf_enabled').checked"),true);
  assert.equal(await evaluate("f('cf_count').value==='2'&&f('cf_active').value==='6'"),true);
  await evaluate("change('cf_active','1');f('cf_enabled').checked=false;f('cf_enabled').dispatchEvent(new Event('change'))");
  assert.equal(await evaluate("f('cf_active').disabled&&f('cf_active').checkValidity()&&f('cf_account').disabled"),true);
  await evaluate("document.querySelector('#modalSubmit').click()");await waitFor("!document.querySelector('#modal').open&&[...document.querySelectorAll('[data-profile-id]')].some(x=>x.innerText.includes('Cloudflare TLS')&&x.dataset.profileRevision==='2'&&!x.querySelector('[data-edit-profile]').disabled)");
  assert.equal('cloudflare_account' in saves.at(-1).spec,false);
  await evaluate("[...document.querySelectorAll('[data-profile-id]')].find(x=>x.innerText.includes('Cloudflare TLS')).querySelector('[data-edit-profile]').click()");await waitFor("document.querySelector('#modal').open");

  await evaluate("change('network','tcp')");assert.equal(await evaluate("f('cf_enabled').disabled&&!f('cf_enabled').checked"),true);
  await evaluate("document.querySelector('#modalCancel').click();document.querySelector('#cloudflareSettings').click()");await waitFor("document.querySelector('#modal').open&&document.querySelector('[data-cf-edit]')");
  await evaluate("document.querySelector('[data-cf-edit]').click()");assert.equal(await evaluate("document.querySelector('[name=cf_secret]').type==='password'&&document.querySelector('[name=cf_secret]').value===''"),true);
  await evaluate("document.querySelector('#modalCancel').click()");
  mark('Cloudflare WS TLS toggle, random lifetime defaults, retained edit state, unsupported transport and write-only credentials');
  assert.deepEqual(errors,[]);
  fs.writeFileSync(path.join(root,'transport-result.json'),JSON.stringify({status:'PASS',cases:passed,page_errors:errors},null,2));console.log('UNIFIED_CONFIG_BROWSER_PASS');
 }finally{ws?.close();chrome.kill('SIGTERM');server.close();server.closeAllConnections();fs.closeSync(log)}
})().catch(e=>{console.error(e);process.exitCode=1});
