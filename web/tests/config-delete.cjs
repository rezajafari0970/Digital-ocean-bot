const fs=require('node:fs'),path=require('node:path'),http=require('node:http'),os=require('node:os'),{spawn}=require('node:child_process'),assert=require('node:assert/strict');
(async()=>{
 const root=process.env.DOB_UI_ARTIFACT_DIR||fs.mkdtempSync(path.join(os.tmpdir(),'dob-view-test-'));
 fs.mkdirSync(root,{recursive:true});
 const staticRoot=path.resolve(__dirname,'../static');
 const profileData=[{route_class:'RESIDENTIAL',enabled:true,master_enabled:true,ports:[443],target_users_per_inbound:20,user_lifetime_seconds:600,users_per_second:1,revision:1}];
 const capacity={target:20,active:7,expired:0,quota_exhausted:0,deficit:13,created_last_cycle:1,inbounds:[{panel_id:'fixture',inbound_id:1,base_url:'http://fixture.invalid',port:443,target:20,active:7,deficit:13,created_last_cycle:1}],automation:{running:true,panels:[],cooldown_panels:0,quarantined_panels:0}};
 const outputHTML=fs.readFileSync(path.resolve(__dirname,'../../internal/adminapi/output_view.go'),'utf8').match(/io.WriteString\(w, `([\s\S]*?)`\)/)[1];
 const server=http.createServer((req,res)=>{
  const url=new URL(req.url,'http://127.0.0.1');
  if(url.pathname.startsWith('/static/')||url.pathname==='/admin/'){
   const name=url.pathname==='/admin/'?'index.html':path.basename(url.pathname);
   const file=path.join(staticRoot,name);if(!fs.existsSync(file)){res.writeHead(404);res.end();return}
   res.setHeader('Content-Type',name.endsWith('.js')?'text/javascript':name.endsWith('.css')?'text/css':'text/html');res.end(fs.readFileSync(file));return;
  }
  if(url.pathname==='/share/output/view-fixture'){res.setHeader('Content-Type',url.searchParams.get('view')==='1'?'text/html':'text/plain');res.end(url.searchParams.get('view')==='1'?outputHTML:'vless://fixture@host.test:443?note='+('long'.repeat(200))+'\n');return}
  const data={'/api/v1/configs/profiles':{unified:true,drivers:[],profiles:[],servers:[],observed_at:new Date().toISOString()},'/api/v1/project-update':{available:true,status:'succeeded',current_revision:'runtime-new-revision',installed_revision:'historical-old-revision',ended_at:'2026-10-12T10:00:00Z'},'/api/v1/residential-proxies':[],'/api/v1/residential-routing':{enabled:true,verified_panels:1,scoped_panels:1,configured_proxies:1,healthy_proxies:1},'/api/v1/residential-performance':{panels:[],experiment:null,routing_enabled:true,defaults:{}},'/api/v1/configs':profileData,'/api/v1/config-capacity':capacity,'/api/v1/config-capacity/cleanup/current':null,'/api/v1/client-mutations':{counts:{SUCCEEDED:1},recent:[],execution_enabled:true},'/api/v1/server-protection':{control:{enabled:false,scope:'fleet',revision:1},panels:[]},'/api/v1/accounts':[],'/api/v1/build-activity':[],'/api/v1/provision-errors':[],'/api/v1/config-capacity/resume':{detail:'Fixture resumed'}};
  res.setHeader('Content-Type','application/json');res.end(JSON.stringify(url.pathname in data?data[url.pathname]:{status:'ready',checks:[],active_servers:1}));
 });
 await new Promise(r=>server.listen(0,'127.0.0.1',r));const base='http://127.0.0.1:'+server.address().port;
 const profile=fs.mkdtempSync(path.join(root,'ui-profile-')),log=fs.openSync(path.join(root,'browser.log'),'w');
 const chrome=spawn(process.env.DOB_TEST_CHROMIUM||'/snap/bin/chromium',['--headless','--no-sandbox','--disable-dev-shm-usage','--disable-gpu','--remote-debugging-port=0','--remote-debugging-address=127.0.0.1','--user-data-dir='+profile,'about:blank'],{stdio:['ignore',log,log],env:{...process.env,TMPDIR:root}});
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
  await waitFor('!!document.querySelector("#configProfilesSlot")&&!activePageView.loading');
  await evaluate('window.fixtures='+JSON.stringify([0,1,2,3].map(i=>({profile:{id:'00000000-0000-4000-8000-00000000000'+i,name:'Profile '+i,revision:1,enabled:true,fleet:true,classes:['DIRECT'],clients_per_class:1,spec:{protocol:'vless',network:'tcp',security:'reality',port:18440+i}},state:'ready',eligible:1,verified:1,ready:1,published:1,target:1,pending:0,blocked:0,deleting:0})))+';window.listBody={unified:true,profiles:fixtures.slice(0,3),drivers:[],servers:[],observed_at:new Date().toISOString()};fixtureQueue.push({url:"/api/v1/configs/profiles",kind:"json",body:listBody});refreshConfigProfiles()');
  assert.equal(await evaluate('document.querySelectorAll("[data-profile-id]").length'),3);
  await evaluate('holdRead("old-profiles","/api/v1/configs/profiles");window.oldProfiles=refreshConfigProfiles();void 0');
  await waitFor('fixtureHolds.some(x=>x.id==="old-profiles")');
  await evaluate('document.querySelector("[data-delete-profile]").click()');
  await evaluate('fixtureQueue.push({url:"/api/v1/configs/profiles/"+fixtures[0].profile.id,kind:"json",status:202,body:{id:fixtures[0].profile.id,state:"removed",cleanup_state:"pending"}},{url:"/api/v1/configs/profiles",kind:"network"});document.querySelector("#modalForm").requestSubmit()',true);
  await waitFor('!document.querySelector("#modal").open&&!document.querySelector("[data-profile-id=\\""+fixtures[0].profile.id+"\\"]")');
  assert.equal(await evaluate('document.querySelectorAll("[data-profile-id]").length'),2);
  await evaluate('finishRead("old-profiles",listBody);oldProfiles');
  assert.equal(await evaluate('document.querySelectorAll("[data-profile-id]").length'),2);
  assert.equal(await evaluate('fixtureCalls.filter(x=>x.method==="DELETE").length'),1);
  mark('committed Delete removes immediately despite failed refresh and stale held response');

  await evaluate('document.querySelector("[data-delete-profile]").click();fixtureQueue.push({url:"/api/v1/configs/profiles/"+fixtures[1].profile.id,kind:"json",status:500,body:{error:"fixture_write_failed"}});document.querySelector("#modalForm").requestSubmit()',true);
  await waitFor('document.querySelector("#modalError").textContent.length>0');
  assert.equal(await evaluate('document.querySelectorAll("[data-profile-id]").length'),2);
  await evaluate('document.querySelector("#modal").close()');
  mark('failed write keeps the profile visible and never claims removal');

  await evaluate('document.querySelector("[data-delete-all-profiles]").click();fixtureQueue.push({url:"/api/v1/configs/profiles/delete-batch",kind:"json",status:202,body:{accepted:1,conflicts:1,results:[{id:fixtures[1].profile.id,state:"revision_conflict"},{id:fixtures[2].profile.id,state:"removed",cleanup_state:"pending"}]}},{url:"/api/v1/configs/profiles",kind:"network"});document.querySelector("#modalForm").requestSubmit()',true);
  await waitFor('!document.querySelector("#modal").open&&document.querySelectorAll("[data-profile-id]").length===1');
  assert.equal(await evaluate('document.querySelector("[data-profile-id]").dataset.profileId'),await evaluate('fixtures[1].profile.id'));
  mark('partial batch removes accepted profiles while preserving revision conflicts');

  await evaluate('fixtureQueue.push({url:"/api/v1/configs/profiles",kind:"json",body:{...listBody,profiles:[...fixtures,{...fixtures[3],profile:{...fixtures[3].profile,id:"00000000-0000-4000-8000-000000000009",delete_requested_at:new Date().toISOString()}}]}});refreshConfigProfiles()');
  assert.deepEqual(await evaluate('[...document.querySelectorAll("[data-profile-id]")].map(x=>x.dataset.profileId)'),await evaluate('[fixtures[1].profile.id,fixtures[3].profile.id]'));
  mark('late snapshots cannot resurrect removals; older pending deletions stay outside Configs');

  await evaluate('document.querySelector("[data-delete-profile]").click();fixtureQueue.push({url:"/api/v1/configs/profiles/"+fixtures[1].profile.id,kind:"json",status:202,body:{id:fixtures[3].profile.id,state:"removed",cleanup_state:"pending"}});document.querySelector("#modalForm").requestSubmit()',true);
  await waitFor('document.querySelector("#modalError").textContent.includes("could not be confirmed")');
  assert.equal(await evaluate('document.querySelectorAll("[data-profile-id]").length'),2);
  await evaluate('document.querySelector("#modal").close()');
  for(const width of [390,1280]){await call('Emulation.setDeviceMetricsOverride',{width,height:844,deviceScaleFactor:1,mobile:width<800});assert.equal(await evaluate('document.documentElement.scrollWidth<=innerWidth+1'),true)}
  assert.deepEqual(errors,[]);
  assert.equal(await evaluate('fixtureCalls.filter(x=>x.url==="/api/v1/configs/profiles/delete-batch").length'),1);
  fs.writeFileSync(path.join(root,'result.json'),JSON.stringify({status:'PASS',passed,page_errors:errors},null,2));
  console.log('IMMEDIATE_PROFILE_DELETE_BROWSER_PASS');
 }finally{ws?.close();chrome.kill('SIGTERM');server.close();server.closeAllConnections();fs.closeSync(log)}
})().catch(e=>{console.error(e);process.exitCode=1});
