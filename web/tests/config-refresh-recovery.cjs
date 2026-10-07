const fs=require('node:fs'),path=require('node:path'),http=require('node:http'),os=require('node:os'),{spawn}=require('node:child_process'),assert=require('node:assert/strict');
(async()=>{
 const root=process.env.DOB_UI_ARTIFACT_DIR||fs.mkdtempSync(path.join(os.tmpdir(),'dob-view-test-'));
 fs.mkdirSync(root,{recursive:true});
 const staticRoot=path.resolve(__dirname,'../static');
 const profileData=[{route_class:'RESIDENTIAL',enabled:true,master_enabled:true,ports:[443],target_users_per_inbound:20,user_lifetime_seconds:600,users_per_second:1,revision:1}];
 const capacity={target:20,active:7,expired:0,quota_exhausted:0,deficit:13,created_last_cycle:1,inbounds:[{panel_id:'fixture',inbound_id:1,base_url:'http://fixture.invalid',port:443,target:20,active:7,deficit:13,created_last_cycle:1}],automation:{running:true,panels:[],cooldown_panels:0,quarantined_panels:0}};
 const server=http.createServer((req,res)=>{
  const url=new URL(req.url,'http://127.0.0.1');
  if(url.pathname.startsWith('/static/')||url.pathname==='/admin/'){
   const name=url.pathname==='/admin/'?'index.html':path.basename(url.pathname);
   const file=path.join(staticRoot,name);if(!fs.existsSync(file)){res.writeHead(404);res.end();return}
   res.setHeader('Content-Type',name.endsWith('.js')?'text/javascript':name.endsWith('.css')?'text/css':'text/html');res.end(fs.readFileSync(file));return;
  }
  const data={'/api/v1/configs':profileData,'/api/v1/config-capacity':capacity,'/api/v1/config-capacity/cleanup/current':null,'/api/v1/client-mutations':{counts:{SUCCEEDED:1},recent:[],execution_enabled:true},'/api/v1/server-protection':{control:{enabled:false,scope:'fleet',revision:1},panels:[]},'/api/v1/accounts':[],'/api/v1/build-activity':[],'/api/v1/provision-errors':[],'/api/v1/config-capacity/resume':{detail:'Fixture resumed'}};
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
  await call('Page.navigate',{url:base+'/admin/#configs'});await waitFor('!!document.querySelector("#capacityStatusSlot")&&!activePageView.loading');
  assert.equal(await evaluate('document.documentElement.scrollWidth<=innerWidth+1'),true);
  await evaluate('window.testCap='+JSON.stringify(capacity));mark('initial mobile render');
  // Older same-page response cannot overwrite the new DOM, even when transport ignores abort.
  await evaluate("holdRead('old');window.oldPoll=refreshConfigStatus();void 0");await waitFor("fixtureHolds.some(x=>x.id==='old')");
  await evaluate("load('configs')");await evaluate("finishRead('old',{...testCap,active:999});oldPoll");
  assert.equal(await evaluate("document.querySelector('#capacityStatusSlot').innerText.includes('999')"),false);mark('same-page reload drops stale refresh');
  // Navigation invalidates in-flight rendering and each freshly mounted slot.
  await evaluate("holdRead('nav');window.navLoad=load('configs');void 0");await waitFor("fixtureHolds.some(x=>x.id==='nav')");
  await evaluate("load('output')");await evaluate("finishRead('nav',testCap);navLoad");assert.equal(await evaluate("!!document.querySelector('#outputAutomationStatus')&&!document.querySelector('#capacityStatusSlot')"),true);mark('late full render cannot overwrite navigation');
  await evaluate("holdRead('account','/api/v1/accounts/123/dashboard');window.oldAccount=accountDash(123);void 0");await waitFor("fixtureHolds.some(x=>x.id==='account')");
  await evaluate("load('configs')");await evaluate("finishRead('account',{account:{name:'Old account'}});oldAccount");assert.equal(await evaluate('fixtureAlerts.length'),0);mark('superseded account read does not alert or replace current page');
  await evaluate("fixtureQueue.push({url:'/api/v1/accounts/123/dashboard',kind:'network'});accountDash(123)");await waitFor('activePageView.loadFailed');
  await evaluate("fixtureQueue.push({url:'/api/v1/accounts/123/dashboard',kind:'json',body:{account:{name:'Recovered account'},capacity:{},resources:{},network:{},deployments:{},runtime:{}}});window.dispatchEvent(new Event('online'))");
  await waitFor("document.querySelector('#title').textContent==='Account — Recovered account'");assert.equal(await evaluate('fixtureAlerts.length'),0);mark('account read recovers while preserving its route');
  await evaluate("load('configs')");
  await evaluate("window.previousView=activePageView;accountDash(123,{preserveHash:true})");assert.equal(await evaluate("current==='configs'&&activePageView===previousView"),true);mark('late account action cannot invalidate another route');
  // Removing only one target while a request is in-flight is harmless and does not partially update.
  await evaluate("holdRead('target');window.targetPoll=refreshConfigStatus();void 0");await waitFor("fixtureHolds.some(x=>x.id==='target')");
  await evaluate("document.querySelector('#cleanupStatusSlot').remove();finishRead('target',{...testCap,active:888});targetPoll");
  assert.equal(await evaluate("document.querySelector('#capacityStatusSlot').innerText.includes('888')"),false);await evaluate("load('configs')");mark('immutable target fences prevent null DOM and partial writes');
  // Background failures retain last successful counters and recover without mutation.
  for(const fault of [{kind:'network'},{kind:'json',body:null},{kind:'malformed'},{kind:'json',status:503,body:{error:'unavailable'}}]){
   await evaluate('fixtureQueue.push({url:"/api/v1/config-capacity",...'+JSON.stringify(fault)+'});refreshConfigStatus()');
   assert.equal(await evaluate("document.querySelector('#capacityStatusSlot').innerText.includes('last successful data')"),true);
   assert.equal(await evaluate("document.querySelector('#capacityStatusSlot .stat b').textContent"),'20');
   await evaluate("window.dispatchEvent(new Event('online'))");await waitFor("!document.querySelector('[data-read-status=configs]')");
  }mark('network, null, malformed and HTTP errors preserve status and auto-recover');
  await evaluate("holdRead('single');window.singlePoll=refreshConfigStatus();void 0");await waitFor("fixtureHolds.some(x=>x.id==='single')");
  const before=await evaluate("fixtureCalls.filter(x=>x.url==='/api/v1/config-capacity').length");await evaluate("Promise.all(Array.from({length:8},()=>refreshConfigStatus()))");assert.equal(await evaluate("fixtureCalls.filter(x=>x.url==='/api/v1/config-capacity').length"),before);
  await evaluate("finishRead('single',testCap);singlePoll");mark('polls are single-flight');
  await evaluate("holdRead('hung');window.hungPoll=refreshConfigStatus();void 0");await waitFor("fixtureHolds.some(x=>x.id==='hung')");
  await waitFor("document.querySelector('[data-read-status=configs]')",18000);assert.equal(await evaluate("fixtureHolds.find(x=>x.id==='hung').signal.aborted"),true);
  await evaluate("window.dispatchEvent(new Event('online'))");await waitFor("!document.querySelector('[data-read-status=configs]')");await evaluate("finishRead('hung',{...testCap,active:777});hungPoll");assert.equal(await evaluate("document.querySelector('#capacityStatusSlot').innerText.includes('777')"),false);mark('hung reads timeout, recover, and ignore late completion');
  // No successful initial render still recovers on its scheduled retry.
  await evaluate("load('output');fixtureQueue.push({url:'/api/v1/config-capacity',kind:'network'});load('configs')");await waitFor("activePageView.loadFailed");
  await waitFor("!!document.querySelector('#capacityStatusSlot')&&!activePageView.loadFailed",9000);mark('initial failed load recovers automatically');
  // Modal edits are left untouched; paused reads resume after visibility change.
  await evaluate("document.querySelector('#modalFields').innerHTML='<input id=fixtureEdit value=keep>';document.querySelector('#modal').showModal()");
  const reads=await evaluate('fixtureCalls.length');await evaluate('refreshConfigStatus()');assert.equal(await evaluate('fixtureCalls.length'),reads);assert.equal(await evaluate("document.querySelector('#fixtureEdit').value"),'keep');
  await evaluate("document.querySelector('#modal').close();document.dispatchEvent(new Event('visibilitychange'))");mark('modal edit preserved and visibility wake supported');
  // Confirmed empty inventory is unavailable, not an invented zero count.
  await waitFor("![...activePageView.polls.values()].some(x=>x.busy)");
  await evaluate("fixtureQueue.push({url:'/api/v1/config-capacity',kind:'json',body:{...testCap,inbounds:[],target:0,active:0,deficit:0}});refreshConfigStatus()");
  assert.equal(await evaluate("document.querySelector('#capacityStatusSlot .stat b').textContent"),'—');assert.equal(await evaluate("document.querySelector('#capacityStatusSlot').innerText.includes('Counts are unavailable')"),true);await evaluate("load('configs')");mark('empty snapshot explicitly unavailable');
  // One click is one mutation; recoveries issue no POSTs.
  const postBefore=await evaluate("fixtureCalls.filter(x=>x.method!=='GET').length");assert.equal(postBefore,0);
  await evaluate("document.querySelector('[data-action=resume-capacity]').click()");await waitFor("fixtureCalls.filter(x=>x.url==='/api/v1/config-capacity/resume').length===1&&!activePageView.loading");
  await evaluate("refreshConfigStatus()");assert.equal(await evaluate("fixtureCalls.filter(x=>x.method!=='GET').length"),1);mark('resume sent once; automatic recovery is read-only');
  // An expired admin session stops automatic reads, without automatically signing in.
  await evaluate("fixtureQueue.push({url:'/api/v1/config-capacity',kind:'json',status:401,body:{error:'unauthorized'}});refreshConfigStatus()");assert.equal(await evaluate("token===''&&!document.querySelector('#login').hidden&&activePageView===null"),true);
  const afterLogout=await evaluate('fixtureCalls.length');await evaluate("wakePageReads();refreshConfigStatus();refreshClientOpsStatus();refreshOutputAutomationStatus()");assert.equal(await evaluate('fixtureCalls.length'),afterLogout);mark('expired session invalidates reads and respects authentication');
  // Only the local fixture establishes a replacement session; production login is never bypassed.
  await evaluate("token='local-new-fixture';showApp()");await waitFor("!!document.querySelector('#capacityStatusSlot')&&!activePageView.loading");
  assert.deepEqual(errors,[]);await evaluate("document.querySelector('#capacityStatusSlot').scrollIntoView();document.querySelector('#toast').hidden=true");const shot=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:false});fs.writeFileSync(path.join(root,'configs-mobile.png'),Buffer.from(shot.data,'base64'));
  fs.writeFileSync(path.join(root,'result.json'),JSON.stringify({status:'PASS',passed,page_errors:errors,mutation_count:1},null,2));console.log('UI_REFRESH_RECOVERY_BROWSER_PASS');
 }finally{ws?.close();chrome.kill('SIGTERM');server.close();server.closeAllConnections();fs.closeSync(log)}
})().catch(e=>{console.error(e);process.exitCode=1});
