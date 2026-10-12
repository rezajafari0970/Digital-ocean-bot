const fs=require('node:fs'),path=require('node:path'),http=require('node:http'),os=require('node:os'),{spawn}=require('node:child_process'),assert=require('node:assert/strict');
(async()=>{
 const root=process.env.DOB_UI_ARTIFACT_DIR||fs.mkdtempSync(path.join(os.tmpdir(),'dob-view-test-'));
 fs.mkdirSync(root,{recursive:true});
 const staticRoot=path.resolve(__dirname,'../static');
 const profileData=[{route_class:'RESIDENTIAL',enabled:true,master_enabled:true,ports:[443],target_users_per_inbound:20,user_lifetime_seconds:600,users_per_second:1,revision:1}];
 const capacity={target:20,active:7,expired:0,quota_exhausted:0,deficit:13,created_last_cycle:1,inbounds:[{panel_id:'fixture',inbound_id:1,base_url:'http://fixture.invalid',port:443,target:20,active:7,deficit:13,created_last_cycle:1}],automation:{running:true,panels:[],cooldown_panels:0,quarantined_panels:0}};
 const activity={active_builds:107,waiting_builds:2,observed_at:new Date().toISOString(),records:Array.from({length:40},(_,i)=>({id:'build-'+i,account:'Account '+i,state:i%2?'PANEL_COMPLETE':'FAILED',active:false,step:'panel',attempt:1,error:i%2?'':'create operation failed',operation_error:i%2?'':'create_server: invalid_request (HTTP 400)',updated_at:new Date().toISOString()}))};
 const server=http.createServer((req,res)=>{
  const url=new URL(req.url,'http://127.0.0.1');
  if(url.pathname.startsWith('/static/')||url.pathname==='/admin/'){
   const name=url.pathname==='/admin/'?'index.html':path.basename(url.pathname);
   const file=path.join(staticRoot,name);if(!fs.existsSync(file)){res.writeHead(404);res.end();return}
   res.setHeader('Content-Type',name.endsWith('.js')?'text/javascript':name.endsWith('.css')?'text/css':'text/html');res.end(fs.readFileSync(file));return;
  }
  const data={'/api/v1/residential-proxies':[],'/api/v1/residential-routing':{enabled:true,verified_panels:1,scoped_panels:1,configured_proxies:1,healthy_proxies:1},'/api/v1/residential-performance':{panels:[],experiment:null,routing_enabled:true,defaults:{}},'/api/v1/configs':profileData,'/api/v1/config-capacity':capacity,'/api/v1/config-capacity/cleanup/current':null,'/api/v1/client-mutations':{counts:{SUCCEEDED:1},recent:[],execution_enabled:true},'/api/v1/server-protection':{control:{enabled:false,scope:'fleet',revision:1},panels:[]},'/api/v1/accounts':[],'/api/v1/build-activity':activity,'/api/v1/provision-errors':[],'/api/v1/config-capacity/resume':{detail:'Fixture resumed'}};
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
  const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error('Evaluation failed: '+JSON.stringify(r.exceptionDetails));return r.result.value};
  async function waitFor(expression,ms=12000){const end=Date.now()+ms;while(Date.now()<end){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,100))}throw Error('UI assertion timeout: '+expression+' body='+await evaluate('document.body.innerText.slice(-3000)'))}
  const mark=x=>{passed.push(x);console.log('PASS',x)};

  await call('Page.navigate',{url:base+'/admin/#build-activity'});await waitFor('!!document.querySelector("[data-build-rows]")&&!activePageView.loading');
  await evaluate('window.testActivity='+JSON.stringify(activity));
  assert.equal(await evaluate("document.querySelector('[data-build-summary] .stat b').textContent"),'107');
  assert.equal(await evaluate('document.documentElement.scrollWidth<=innerWidth+1'),true);mark('global summary and mobile layout');
  await evaluate("window.stableRow=document.querySelector('[data-build-id=build-20]');stableRow.scrollIntoView();window.oldTop=stableRow.getBoundingClientRect().top");
  await evaluate('refreshBuildActivity()');
  assert.equal(await evaluate("stableRow===document.querySelector('[data-build-id=build-20]')"),true);
  assert.equal(await evaluate('Math.abs(stableRow.getBoundingClientRect().top-oldTop)<2'),true);mark('poll keeps keyed row and vertical reading position');
  await evaluate("holdRead('slow','/api/v1/build-activity');window.slowPoll=refreshBuildActivity();void 0");await waitFor("fixtureHolds.some(x=>x.id==='slow')");
  assert.equal(await evaluate("document.querySelector('#buildActivityStatus').innerText.includes('Loading')"),false);
  const before=await evaluate("fixtureCalls.filter(x=>x.url==='/api/v1/build-activity').length");await evaluate('Promise.all(Array.from({length:8},()=>refreshBuildActivity()))');assert.equal(await evaluate("fixtureCalls.filter(x=>x.url==='/api/v1/build-activity').length"),before);
  await evaluate("finishRead('slow',testActivity);slowPoll");mark('no loading replacement and single-flight');
  for(const fault of [{kind:'network'},{kind:'malformed'},{kind:'json',body:null},{kind:'json',status:503,body:{error:'unavailable'}}]){
   await evaluate('fixtureQueue.push({url:"/api/v1/build-activity",...'+JSON.stringify(fault)+'});refreshBuildActivity()');
   assert.equal(await evaluate("document.querySelector('[data-build-summary] .stat b').textContent"),'107');
   assert.equal(await evaluate("document.querySelector('#buildActivityStatus').innerText.includes('last successful data')"),true);
   await evaluate("window.dispatchEvent(new Event('online'))");await waitFor("!document.querySelector('[data-read-status=build-activity]')");
  }mark('last-good data survives network, malformed, null and HTTP failures');
  await evaluate("fixtureQueue.push({url:'/api/v1/provision-errors',kind:'network'});refreshBuildActivity()");assert.equal(await evaluate("!!document.querySelector('[data-read-status=build-activity]')"),true);await evaluate("window.dispatchEvent(new Event('online'))");await waitFor("!document.querySelector('[data-read-status=build-activity]')");mark('failed history read is not empty success');
  await evaluate("holdRead('nav','/api/v1/build-activity');window.navPoll=refreshBuildActivity();void 0");await waitFor("fixtureHolds.some(x=>x.id==='nav')");await evaluate("window.newLoad=load('configs');finishRead('nav',{...testActivity,active_builds:999});Promise.all([newLoad,navPoll])");assert.equal(await evaluate("!!document.querySelector('#capacityStatusSlot')&&!document.querySelector('#buildActivityStatus')"),true);mark('navigation fences stale activity');
  await evaluate("load('build-activity');holdRead('modal','/api/v1/build-activity/build-0/provision');window.modalRead=openProvisionDetail('build-0');void 0");await waitFor("fixtureHolds.some(x=>x.id==='modal')");await evaluate("window.newLoad=load('configs');finishRead('modal',{});Promise.all([newLoad,modalRead])");assert.equal(await evaluate("document.querySelector('#modal').open||fixtureAlerts.length>0"),false);mark('late diagnostics cannot open on another page');
  await evaluate("(async()=>{await load('build-activity');fixtureQueue.push({url:'/api/v1/build-activity/build-0/provision',kind:'json',body:{state:'COMPLETED',steps:[],events:[]}});await openProvisionDetail('build-0')})()");assert.equal(await evaluate("document.querySelector('#modal').open"),true);const count=await evaluate('fixtureCalls.length');await evaluate('refreshBuildActivity()');assert.equal(await evaluate('fixtureCalls.length'),count);await evaluate("document.querySelector('#modal').close()");mark('open diagnostics preserved');
  await call('Emulation.setDeviceMetricsOverride',{width:1280,height:844,deviceScaleFactor:1,mobile:false});await evaluate("const wrap=document.querySelector('#buildActivityStatus .tableWrap');wrap.style.maxWidth='400px';wrap.scrollLeft=140;window.oldLeft=wrap.scrollLeft;refreshBuildActivity()");assert.equal(await evaluate("document.querySelector('#buildActivityStatus .tableWrap').scrollLeft"),await evaluate('oldLeft'));mark('horizontal table scroll preserved');
  await evaluate("load('configs');fixtureQueue.push({url:'/api/v1/build-activity',kind:'network'});load('build-activity')");await waitFor('activePageView.loadFailed');await waitFor("document.querySelector('[data-build-rows]')&&!activePageView.loadFailed",9000);mark('initial load failure recovers');
  assert.equal(await evaluate("fixtureCalls.filter(x=>x.method!=='GET').length"),0);assert.deepEqual(errors,[]);
  await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});await evaluate("scrollTo(0,0);document.querySelector('#toast').hidden=true");const shot=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:false});fs.writeFileSync(path.join(root,'build-activity-mobile.png'),Buffer.from(shot.data,'base64'));
  fs.writeFileSync(path.join(root,'result.json'),JSON.stringify({status:'PASS',passed,page_errors:errors,mutation_count:0},null,2));console.log('BUILD_ACTIVITY_BROWSER_PASS');
 }finally{ws?.close();chrome.kill('SIGTERM');server.close();server.closeAllConnections();fs.closeSync(log)}
})().catch(e=>{console.error(e);process.exitCode=1});
