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
  await call('Page.navigate',{url:base+'/admin/#configs'});await waitFor('!!document.querySelector("#configProfilesSlot")&&!activePageView.loading');
  assert.equal(await evaluate('document.documentElement.scrollWidth<=innerWidth+1'),true);
  await evaluate('window.testCap='+JSON.stringify(capacity));mark('initial mobile render');
  await evaluate("holdRead('account','/api/v1/accounts/123/dashboard');window.oldAccount=accountDash(123);void 0");await waitFor("fixtureHolds.some(x=>x.id==='account')");
  await evaluate("load('configs')");await evaluate("finishRead('account',{account:{name:'Old account'}});oldAccount");assert.equal(await evaluate('fixtureAlerts.length'),0);mark('superseded account read does not alert or replace current page');
  await evaluate("fixtureQueue.push({url:'/api/v1/accounts/123/dashboard',kind:'network'});accountDash(123)");await waitFor('activePageView.loadFailed');
  await evaluate("fixtureQueue.push({url:'/api/v1/accounts/123/dashboard',kind:'json',body:{account:{name:'Recovered account'},capacity:{},resources:{},network:{},deployments:{},runtime:{}}});window.dispatchEvent(new Event('online'))");
  await waitFor("document.querySelector('#title').textContent==='Account — Recovered account'");assert.equal(await evaluate('fixtureAlerts.length'),0);mark('account read recovers while preserving its route');
  for(const freshness of ['scheduled','stale','expired']){
   await evaluate('fixtureQueue.push({url:"/api/v1/accounts/123/dashboard",kind:"json",body:{account:{name:"Inventory fixture"},capacity:{data_status:'+JSON.stringify(freshness)+'},resources:{total:3,managed:2,unmanaged:1,active:2},network:{},deployments:{},runtime:{}}});accountDash(123)');
   assert.equal(await evaluate("fixtureCalls.filter(x=>x.url==='/api/v1/accounts/123/refresh').length"),0);
   assert.equal(await evaluate("document.body.innerText.includes('Provider servers observed')"),true);
  }mark('account observation ages never trigger implicit provider mutations');
  for(const available of [false,true]){
   await evaluate('fixtureQueue.push({url:"/api/v1/accounts/123/dashboard",kind:"json",body:{account:{name:"Inventory fixture"},capacity:{},resources:{data_available:'+available+'},droplets:[],network:{},deployments:{},runtime:{}}});accountDash(123)');
   assert.equal(await evaluate("document.body.innerText.includes('Provider inventory unavailable; no provider observation')"),!available);
   assert.equal(await evaluate("document.body.innerText.includes('No provider servers in the latest inventory.')"),available);
  }mark('missing inventory and observed empty inventory remain distinct');

  await evaluate("load('configs')");
  await evaluate("window.previousView=activePageView;accountDash(123,{preserveHash:true})");assert.equal(await evaluate("current==='configs'&&activePageView===previousView"),true);mark('late account action cannot invalidate another route');
  const accountHealth=[{id:'locked-fixture',name:'Locked <img src=x onerror=alert(1)>',provider:'digitalocean',health:'broken',reason:'LOCKED',provider_state:'LOCKED',provider_error_state:'PROXY_ERROR',observed_provider_status:'locked'},{id:'full-fixture',name:'Full account',provider:'digitalocean',health:'capacity',reason:'CAPACITY_FULL',provider_state:'ACTIVE'},{id:'transient-fixture',name:'Temporary network issue',provider:'vultr',health:'degraded',reason:'TRANSPORT_ERROR',provider_state:'ACTIVE'}];
  const health={account_health:accountHealth,account_health_total:3,account_health_truncated:false,broken_accounts:1,degraded_accounts:1,status:'ready',active_servers:45,inactive_servers:1,observed_at:new Date().toISOString(),server_health:[{account_name:'Fixture',provider:'upcloud',provider_resource_id:'fixture-resource',lifecycle_state:'READY',reason:'ROUTING_UNVERIFIED',provider_state:'TOKEN_INVALID'}]};
  await evaluate('fixtureQueue.push({url:"/api/v1/system",kind:"json",body:'+JSON.stringify(health)+'});load("dashboard")');
  assert.equal(await evaluate("document.querySelector('#dashboardStatus').innerText.includes('Routing not verified')&&document.querySelector('#dashboardStatus').innerText.includes('TOKEN_INVALID')"),true);
  assert.equal(await evaluate("document.querySelector('[data-read-key=account-health-locked-fixture]').innerText.includes('Provider account locked')&&document.querySelector('[data-read-key=account-health-locked-fixture]').innerText.includes('PROXY_ERROR')"),true);
  assert.equal(await evaluate("document.querySelector('[data-read-key=account-health-full-fixture]').innerText.includes('Capacity full')"),true);
  assert.equal(await evaluate("document.querySelectorAll('[data-read-key^=account-health-] img').length"),0);
  for(const width of [390,1280]){await call('Emulation.setDeviceMetricsOverride',{width,height:844,deviceScaleFactor:1,mobile:width<800});assert.equal(await evaluate('document.documentElement.scrollWidth<=innerWidth+1'),true)}
  mark('account restrictions, independent proxy failures and capacity displayed safely on mobile and desktop');
  assert.equal(await evaluate("document.querySelector('#dashboardStatus .tableWrap').scrollWidth<=document.querySelector('#dashboardStatus .tableWrap').clientWidth+1"),true);
  await evaluate('fixtureQueue.push({url:"/api/v1/system",kind:"json",body:'+JSON.stringify({...health,active_servers:46,server_health:[]})+'});refreshDashboard()');
  assert.equal(await evaluate("document.querySelector('#dashboardStatus .stat b').textContent"),'46');mark('dashboard refreshes without navigation and mobile reasons fit');
  await evaluate("holdRead('dashboard-old','/api/v1/system');window.dashboardOld=refreshDashboard();void 0");await waitFor("fixtureHolds.some(x=>x.id==='dashboard-old')");
  await evaluate('load("configs")');await evaluate('finishRead("dashboard-old",'+JSON.stringify(health)+');dashboardOld');
  assert.equal(await evaluate("!!document.querySelector('#configProfilesSlot')&&!document.querySelector('#dashboardStatus')"),true);mark('dashboard refresh cannot overwrite a different page');
  await evaluate("load('residential')");await waitFor("document.querySelector('#residentialPerformance')?.innerText.includes('No performance profile')");
  await evaluate("holdRead('performance-old','/api/v1/residential-performance');window.oldPerformance=refreshResidentialPerformance();void 0");await waitFor("fixtureHolds.some(x=>x.id==='performance-old')");
  const perfReads=await evaluate("fixtureCalls.filter(x=>x.url==='/api/v1/residential-performance').length");await evaluate("Promise.all(Array.from({length:8},()=>refreshResidentialPerformance()))");assert.equal(await evaluate("fixtureCalls.filter(x=>x.url==='/api/v1/residential-performance').length"),perfReads);
  await evaluate("load('configs')");await evaluate("finishRead('performance-old',{panels:[],experiment:{id:'late'}});oldPerformance");
  assert.equal(await evaluate("performanceStatus.experiment===null&&!!document.querySelector('#configProfilesSlot')"),true);mark('residential status is single-flight and ignores obsolete responses');
  // The visible popup exists before any asynchronous sharing request begins.
  assert.deepEqual(await evaluate("(()=>{const w=openOutputWindow();const result=[!!w,w?.document.body.innerText.includes('Preparing your output'),w?.opener===null];w?.close();return result})()",true),[true,true,true]);mark('output popup immediately explains pending loading');
  // The popup can steal focus; restore the tested tab before visibility-gated reads.
  await send('Target.activateTarget',{targetId});
  await waitFor('!document.hidden');
  await evaluate('fixtureQueue.push({url:"/api/v1/panel-settings",kind:"json",body:{listen_port:18080,web_path:"/admin",username:"fixture"}},{url:"/api/v1/project-update",kind:"json",body:{available:true,status:"succeeded",current_revision:"runtime-new-revision",installed_revision:"historical-old-revision",ended_at:"2026-10-12T10:00:00Z"}});load("settings")');
  await waitFor("document.querySelector('#projectUpdateStatus')?.innerText.includes('runtime-new-')");
  assert.equal(await evaluate("document.querySelector('#projectUpdateStatus').innerText.includes('Last update installed revision: historical-o')"),true);mark('running version and old update receipt are distinct');
  await evaluate("load('configs')");
  // An expired admin session stops automatic reads, without automatically signing in.
  await evaluate("fixtureQueue.push({url:'/api/v1/configs/profiles',kind:'json',status:401,body:{error:'unauthorized'}});refreshConfigProfiles()");assert.equal(await evaluate("token===''&&!document.querySelector('#login').hidden&&activePageView===null"),true);
  const afterLogout=await evaluate('fixtureCalls.length');await evaluate("wakePageReads();refreshConfigProfiles()");assert.equal(await evaluate('fixtureCalls.length'),afterLogout);mark('expired session invalidates reads and respects authentication');
  // Only the local fixture establishes a replacement session; production login is never bypassed.
  await evaluate("token='local-new-fixture';showApp()");await waitFor("!!document.querySelector('#configProfilesSlot')&&!activePageView.loading");
  assert.deepEqual(errors,[]);await evaluate("document.querySelector('#configProfilesSlot').scrollIntoView();document.querySelector('#toast').hidden=true");const shot=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:false});fs.writeFileSync(path.join(root,'configs-mobile.png'),Buffer.from(shot.data,'base64'));
  await call('Page.navigate',{url:base+'/share/output/view-fixture?view=1'});await waitFor("document.querySelector('#configs')?.textContent.startsWith('vless://')");
  assert.equal(await evaluate("document.querySelector('#outputStatus').hidden"),true);
  assert.equal(await evaluate('document.documentElement.scrollWidth<=innerWidth+1'),true);mark('human output wraps long URI lines on mobile without altering their text');
  await evaluate("fixtureQueue.push({url:'/share/output/view-fixture',kind:'json',status:503,body:{error:'unavailable'}})");
  await waitFor("document.querySelector('#outputStatus').textContent.includes('temporarily unavailable')");
  assert.equal(await evaluate("document.querySelector('#configs').textContent"),'');
  await waitFor("document.querySelector('#configs').textContent.startsWith('vless://')&&document.querySelector('#outputStatus').hidden");mark('human output explains failure, clears stale URIs and recovers');
  fs.writeFileSync(path.join(root,'result.json'),JSON.stringify({status:'PASS',passed,page_errors:errors,mutation_count:0},null,2));console.log('UI_REFRESH_RECOVERY_BROWSER_PASS');
 }finally{ws?.close();chrome.kill('SIGTERM');server.close();server.closeAllConnections();fs.closeSync(log)}
})().catch(e=>{console.error(e);process.exitCode=1});
