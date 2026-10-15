const fs=require('node:fs'),path=require('node:path'),http=require('node:http'),os=require('node:os'),{spawn}=require('node:child_process'),assert=require('node:assert/strict');
(async()=>{
 const root=process.env.DOB_UI_ARTIFACT_DIR||fs.mkdtempSync(path.join(os.tmpdir(),'dob-view-test-'));
 fs.mkdirSync(root,{recursive:true});
 const staticRoot=path.resolve(__dirname,'../static');
 const profileData=[{route_class:'RESIDENTIAL',enabled:true,master_enabled:true,ports:[443],target_users_per_inbound:20,user_lifetime_seconds:600,users_per_second:1,revision:1}];
 const capacity={target:20,active:7,expired:0,quota_exhausted:0,deficit:13,created_last_cycle:1,inbounds:[{panel_id:'fixture',inbound_id:1,base_url:'http://fixture.invalid',port:443,target:20,active:7,deficit:13,created_last_cycle:1}],automation:{running:true,panels:[],cooldown_panels:0,quarantined_panels:0}};
 const outputHTML=fs.readFileSync(path.resolve(__dirname,'../../internal/adminapi/output_view.go'),'utf8').match(/io.WriteString\(w, `([\s\S]*?)`\)/)[1];
 const preset={"network": "xhttp", "mode": "stream-up", "host": "apart.com", "path_bytes": 4096, "extra": {"xPaddingBytes": "100-1000", "noGRPCHeader": true, "scStreamUpServerSecs": "20-80", "scMaxEachPostBytes": 1000000, "scMinPostsIntervalMs": 30, "scMaxConcurrentPosts": 4}, "finalmask": {"tcp": [{"type": "fragment", "settings": {"packets": "tlshello", "lengths": ["48-80", "80-128", "128-192", "192-288", "288-448"], "delays": ["0-1", "1-2", "1-2", "0-1", "0-1"], "maxSplit": "3-5"}}], "udp": [{"type": "noise", "settings": {"reset": "90-150", "noise": [{"type": "exp", "packet": "<t><c><r 12-24>", "delay": "0-1"}, {"type": "exp", "packet": "<n><rc 8-16><rd 4-8>", "delay": "0-2"}, {"rand": "12-32", "randRange": "0-255", "delay": "0-1"}]}}], "quicParams": {"congestion": "bbr", "bbrProfile": "standard", "debug": false, "brutalUp": 0, "brutalDown": 0, "initStreamReceiveWindow": 8388608, "maxStreamReceiveWindow": 16777216, "initConnectionReceiveWindow": 20971520, "maxConnectionReceiveWindow": 41943040, "maxIdleTimeout": 45, "keepAlivePeriod": 15, "disablePathMTUDiscovery": false, "maxIncomingStreams": 512}}, "finalmask_enabled": false};preset.flow="xtls-rprx-vision-udp443";preset.finalmask={tcp:preset.finalmask.tcp};preset.finalmask_enabled=true;preset.finalmask_scope="both";preset.browser_dialer="WebView";let transport={policy:{revision:1,config:{network:'tcp',finalmask_enabled:false}},preset,max_path_bytes:4096,scope:'Saved defaults apply to new listeners. Select Apply to existing to update managed listeners in sequence. Existing client IDs and limits are preserved; clients must refresh subscriptions.',finalmask_status:'TCP Fragment requires verified core; UDP and QUIC settings do not act on HTTP/2.',compatibility:'Vision uses VLESS Encryption; Browser Dialer requires TLS.'};const saves=[],actions=[];
 let outputPolicy={revision:1,include_xhttp_extra:false,include_finalmask:false};const outputSaves=[];
 const server=http.createServer((req,res)=>{
  const url=new URL(req.url,'http://127.0.0.1');
  if(url.pathname==='/api/v1/configs/output-settings'){
   if(req.method==='PUT'){let body='';req.on('data',x=>body+=x);req.on('end',()=>{const p=JSON.parse(body);outputSaves.push(p);if(p.revision!==outputPolicy.revision){res.writeHead(409);res.end(JSON.stringify({error:'output_settings_changed'}));return}outputPolicy={...p,revision:p.revision+1};res.setHeader('Content-Type','application/json');res.end(JSON.stringify(outputPolicy))});return}
   res.setHeader('Content-Type','application/json');res.end(JSON.stringify(outputPolicy));return;
  }
  if(url.pathname==='/api/v1/configs/transport/apply'||url.pathname.startsWith('/api/v1/configs/transport/rollback/')){
   actions.push(url.pathname);transport.applications=[{id:'00000000-0000-4000-8000-000000000001',panel_id:'fixture-panel',managed_key:'dob:global-reality:00443',state:actions.length===1?'SUCCEEDED':'PENDING',error:''}];res.setHeader('Content-Type','application/json');res.end(JSON.stringify(transport));return;
  }
  if(url.pathname==='/api/v1/configs/transport'){
   if(req.method==='PUT'){let body='';req.on('data',x=>body+=x);req.on('end',()=>{const p=JSON.parse(body);saves.push(p);if(p.revision!==transport.policy.revision){res.writeHead(409);res.end(JSON.stringify({error:'transport_revision_changed'}));return}transport.policy={revision:p.revision+1,config:p.config};res.setHeader('Content-Type','application/json');res.end(JSON.stringify(transport))});return}
   res.setHeader('Content-Type','application/json');res.end(JSON.stringify(transport));return;
  }
  if(url.pathname.startsWith('/static/')||url.pathname==='/admin/'){
   const name=url.pathname==='/admin/'?'index.html':path.basename(url.pathname);
   const file=path.join(staticRoot,name);if(!fs.existsSync(file)){res.writeHead(404);res.end();return}
   res.setHeader('Content-Type',name.endsWith('.js')?'text/javascript':name.endsWith('.css')?'text/css':'text/html');res.end(fs.readFileSync(file));return;
  }
  if(url.pathname==='/share/output/view-fixture'){res.setHeader('Content-Type',url.searchParams.get('view')==='1'?'text/html':'text/plain');res.end(url.searchParams.get('view')==='1'?outputHTML:'vless://fixture@host.test:443?note='+('long'.repeat(200))+'\n');return}
  const data={'/api/v1/project-update':{available:true,status:'succeeded',current_revision:'runtime-new-revision',installed_revision:'historical-old-revision',ended_at:'2026-10-12T10:00:00Z'},'/api/v1/residential-proxies':[],'/api/v1/residential-routing':{enabled:true,verified_panels:1,scoped_panels:1,configured_proxies:1,healthy_proxies:1},'/api/v1/residential-performance':{panels:[],experiment:null,routing_enabled:true,defaults:{}},'/api/v1/configs':profileData,'/api/v1/config-capacity':capacity,'/api/v1/config-capacity/cleanup/current':null,'/api/v1/client-mutations':{counts:{SUCCEEDED:1},recent:[],execution_enabled:true},'/api/v1/server-protection':{control:{enabled:false,scope:'fleet',revision:1},panels:[]},'/api/v1/accounts':[],'/api/v1/build-activity':[],'/api/v1/provision-errors':[],'/api/v1/config-capacity/resume':{detail:'Fixture resumed'}};
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

  await call('Page.navigate',{url:base+'/admin/#configs'});await waitFor("!!document.querySelector('#editTransport')");
  await waitFor("!!document.querySelector('#editOutputSettings')");
  for(const [extra,mask] of [[false,false],[true,false],[false,true],[true,true]]){
   await evaluate("document.querySelector('#editOutputSettings').click()");
   await waitFor("document.querySelector('#modal').open");
   assert.equal(await evaluate("document.querySelector('#modal').innerText.includes('server-only')"),true);
   await evaluate('document.querySelector("[name=include_xhttp_extra]").checked='+extra+';document.querySelector("[name=include_finalmask]").checked='+mask+';document.querySelector("#modalSubmit").click()');
   await waitFor("!document.querySelector('#modal').open");
   assert.equal(outputPolicy.include_xhttp_extra,extra);assert.equal(outputPolicy.include_finalmask,mask);
   assert.equal(saves.length,0);assert.equal(actions.length,0);
   await waitFor("!!document.querySelector('#editOutputSettings')");
  }
  await evaluate("document.querySelector('#editOutputSettings').click()");
  await waitFor("document.querySelector('#modal').open");
  await evaluate("fixtureQueue.push({url:'/api/v1/configs/output-settings',kind:'json',status:409,body:{error:'output_settings_changed',detail:'Reload output settings before saving.'}});document.querySelector('#modalSubmit').click()");
  await waitFor("document.querySelector('#modalError').innerText.includes('Reload')");
  assert.equal(await evaluate("document.querySelector('#modal').open&&!document.querySelector('#modalSubmit').disabled"),true);
  const outputShot=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:false});fs.writeFileSync(path.join(root,'output-fields-mobile.png'),Buffer.from(outputShot.data,'base64'));
  assert.equal(await evaluate('document.documentElement.scrollWidth<=innerWidth+1'),true);
  await evaluate("document.querySelector('#modalCancel').click()");
  mark('independent output toggles, reload, conflict, mobile, no listener mutations');
  await evaluate("document.querySelector('#editTransport').click()");
  await waitFor("document.querySelector('#modal').open");
  await evaluate("var sel=document.querySelector('[name=transport_network]');sel.value='xhttp';sel.dispatchEvent(new Event('change'))");
  assert.equal(await evaluate("document.querySelector('[name=xhttp_host]').value"),'apart.com');
  assert.equal(await evaluate("document.querySelector('[name=xhttp_host]').readOnly"),false);
  assert.equal(await evaluate("document.querySelector('[name=path_bytes]').value"),'4096');
  assert.equal(await evaluate("document.querySelectorAll('#xhttpFields select[disabled]:not([name=browser_dialer])').length"),0);
  assert.equal(await evaluate("document.querySelector('[name=browser_dialer]').disabled"),true);
  assert.equal(await evaluate("document.querySelector('#browserDialerNotice').innerText.includes('REALITY')"),true);
  assert.equal(await evaluate('document.documentElement.scrollWidth<=innerWidth+1'),true);
  const shot=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:false});fs.writeFileSync(path.join(root,'xhttp-mobile.png'),Buffer.from(shot.data,'base64'));
  await evaluate("document.querySelector('#modalSubmit').click()");
  await waitFor("!document.querySelector('#modal').open&&document.querySelector('#configTransportSlot').innerText.includes('apart.com')");
  assert.equal(saves.length,1);assert.deepEqual(saves[0].config,preset);assert.equal(saves[0].apply_existing,true);
  await evaluate("document.querySelector('#editTransport').click()");
  await waitFor("document.querySelector('#modal').open");
  await evaluate("document.querySelector('[name=path_bytes]').value='4097';document.querySelector('#modalSubmit').click()");
  assert.equal(saves.length,1);
  await evaluate("document.querySelector('[name=path_bytes]').value='1024';document.querySelector('[name=xhttp_host]').value='custom.example.org';document.querySelector('[name=xhttp_mode]').value='packet-up';document.querySelector('[name=xhttp_extra]').value=JSON.stringify({xPaddingBytes:'256-512',noGRPCHeader:false});document.querySelector('#modalSubmit').click()");
  await waitFor("!document.querySelector('#modal').open");
  assert.equal(saves.length,2);assert.equal(saves[1].config.path_bytes,1024);assert.equal(saves[1].config.host,'custom.example.org');assert.equal(saves[1].config.mode,'packet-up');assert.equal(saves[1].config.browser_dialer,'WebView');assert.deepEqual(saves[1].config.extra,{xPaddingBytes:'256-512',noGRPCHeader:false});
  await evaluate("document.querySelector('#editTransport').click()");
  await waitFor("document.querySelector('#modal').open");
  assert.equal(await evaluate("document.querySelector('[name=xhttp_host]').value"),'custom.example.org');
  assert.equal(await evaluate("document.querySelector('[name=browser_dialer]').value"),'WebView');
  assert.equal(await evaluate("document.querySelector('#browserDialerNotice').innerText.includes('not active or exported for REALITY')"),true);
  await evaluate("var sel=document.querySelector('[name=transport_network]');sel.value='tcp';sel.dispatchEvent(new Event('change'));document.querySelector('#modalSubmit').click()");
  await waitFor("!document.querySelector('#modal').open&&document.querySelector('#configTransportSlot').innerText.includes('TCP Reality')");
  assert.deepEqual(saves[2].config,{network:'tcp',finalmask_enabled:false});
  await evaluate("document.querySelector('#applyExistingTransport').click()");
  await waitFor("!!document.querySelector('[data-transport-rollback]')");
  assert.equal(await evaluate("document.querySelector('#configTransportSlot').innerText.includes('Verified: 1')"),true);
  await evaluate("document.querySelector('[data-transport-rollback]').click()");
  await waitFor("document.querySelector('#configTransportSlot').innerText.includes('Queued: 1')");
  assert.equal(actions.length,2);
  assert.deepEqual(errors,[]);
  fs.writeFileSync(path.join(root,'transport-result.json'),JSON.stringify({status:'PASS',cases:['independent output toggles and conflict','mobile layout','exact preset','editable settings survive reload','dialer limitation explicit','boundary validation','revision save','TCP creation-default rollback'],page_errors:errors},null,2));console.log('XHTTP_TRANSPORT_BROWSER_PASS');
 }finally{ws?.close();chrome.kill('SIGTERM');server.close();server.closeAllConnections();fs.closeSync(log)}
})().catch(e=>{console.error(e);process.exitCode=1});
