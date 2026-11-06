// Real standalone tabs, mobile input, reload and parent-form preservation.
// Native proxy/cookie isolation is exercised separately by the sandbox gate.
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),http=require('node:http'),assert=require('node:assert/strict'),{spawn}=require('node:child_process');
(async()=>{
 const root=process.env.DOB_UI_ARTIFACT_DIR||fs.mkdtempSync(path.join(os.tmpdir(),'dob-provider-page-'));fs.mkdirSync(root,{recursive:true});
 let capacityBlocked=false;
 const opened=[],actions=[],closed=[],sessions=new Map(),requests=[],network=[];
 const countryOptions=[{code:'US',label:'United States'},{code:'DE',label:'Germany'},{code:'FR',label:'France'}];
 const app=fs.readFileSync('web/static/app.js','utf8');
 const entry=app.slice(app.indexOf('function reserveProviderPage()'));
 const draft=app.match(/\$\('#openDraftProviderBrowser'\)\.onclick=(async\(\)=>\{[\s\S]*?\});\$\('#discoverAccount'\)/)[1];
 let fixtureImage='iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aN1cAAAAASUVORK5CYII=';
 const frame=(s)=>({isolated:true,format:'png',frame_id:'fixture-'+s.width+'-'+s.height,scale_factor:s.scale_factor||1,width:s.width,height:s.height,image:fixtureImage,url:'https://provider.fixture/login',title:'Provider fixture',target:'tab1',tabs:[{id:'tab1',title:'Provider fixture',url:'https://provider.fixture/login'}],editable:[{x:30,y:120,width:280,height:44}]});
 const server=http.createServer(async(req,res)=>{
  const u=new URL(req.url,'http://localhost');requests.push({path:u.pathname,method:req.method,authorization:req.headers.authorization});
  const json=(x,status=200)=>{res.writeHead(status,{'Content-Type':'application/json','Cache-Control':'no-store'});res.end(JSON.stringify(x))};
  if(u.pathname==='/'){
   res.setHeader('Content-Type','text/html; charset=utf-8');
   res.end('<!doctype html><meta name="viewport" content="width=device-width,initial-scale=1"><form id="modalForm"><input name="provider" value="vultr"><input name="token" value="unsaved-provider-secret"><input name="proxy_id" value="one"><input name="proxy_id2" value="two"><input name="proxy_id3" value=""><div id="countries"></div></form><button id="draft">Open draft website</button><button id="saved">Open saved website</button><p id="modalError"></p><p id="toast"></p><script src="/static/proxy-country.js"></script><script src="/static/provider-browser.js"></script><script src="/fixture-entry.js"></script>');return
  }
  if(u.pathname==='/fixture-entry.js'){res.setHeader('Content-Type','text/javascript');res.end('const token="panel-session-fixture";const $=s=>document.querySelector(s);const countryOptions='+JSON.stringify(countryOptions)+';const ensureAccountModule=async()=>new Promise(r=>setTimeout(r,200));const toast=t=>$("#toast").textContent=t;const api=async(p,o={})=>{const r=await fetch(p,o);if(!r.ok)throw Error("read failed");return r.json()};'+entry+';$("#countries").innerHTML=ProxyCountry.field(countryOptions,["US","DE","FR"]);$("#draft").onclick='+draft+';$("#saved").onclick=()=>openProviderBrowser("account-one");');return}
  if(u.pathname==='/fixture-website'){res.setHeader('Content-Type','text/html');res.end('<!doctype html><meta name="viewport" content="width=device-width,initial-scale=1"><body style="margin:0;font:16px system-ui;color:#172033"><header style="background:#1655d4;color:white;padding:20px 30px">Provider fixture</header><main style="padding:24px 30px"><h1 style="font-size:24px">Welcome back</h1><label>Email<input style="display:block;margin-top:10px;width:280px;height:44px"></label><p>Browse your account securely through the selected proxy.</p><button>Continue</button></main></body>');return}
  if(u.pathname.startsWith('/static/')){
   const name=path.basename(u.pathname),file=path.join(process.cwd(),'web/static',name);
   if(!fs.existsSync(file)){res.writeHead(404).end();return}
   res.setHeader('Content-Type',name.endsWith('.js')?'text/javascript':name.endsWith('.html')?'text/html; charset=utf-8':'text/css');
   if(name==='provider-browser.html')res.setHeader('Content-Security-Policy',"default-src 'none'; script-src 'self'; style-src 'self'; img-src data:; connect-src 'self'; form-action 'self'; base-uri 'none'");
   res.end(fs.readFileSync(file));return
  }
  if(u.pathname==='/api/v1/accounts'){json([{id:'account-one',provider:'vultr',proxy_ids:['one','two']}]);return}
  if(u.pathname.endsWith('/proxy-country-policy')){json({policy:{countries:['US','DE','FR']},country_options:countryOptions});return}
  if(u.pathname==='/api/v1/proxies'){json([{id:'one',name:'First proxy',status:'healthy'},{id:'two',name:'Second proxy',status:'healthy'}]);return}
  if(u.pathname==='/api/v1/proxy-drivers'){json({country_options:countryOptions});return}
  if(u.pathname.startsWith('/api/v1/provider-browser/')&&req.headers.authorization!=='Bearer panel-session-fixture'){json({error:'unauthorized'},401);return}
  let raw='';for await(const b of req)raw+=b;
  if(u.pathname==='/api/v1/provider-browser/sessions'&&req.method==='GET'){json(capacityBlocked?[{id:'abandoned-owned',provider:'vultr',country:'US'}]:[]);return}
  if(u.pathname.endsWith('/abandoned-owned')&&req.method==='DELETE'){assert(capacityBlocked);capacityBlocked=false;res.writeHead(204).end();return}
  if(u.pathname==='/api/v1/provider-browser/sessions'){
   if(capacityBlocked){json({error:'browser_capacity_full',detail:'Close an existing provider browser before opening another.'},429);return}
   const s=JSON.parse(raw);opened.push(s);if(s.proxy_countries.includes('BO')){json({error:'browser_route_unverified',detail:'Selected country could not be verified.'},409);return}const id='owned-session-'+opened.length;sessions.set(id,s);
   json({id,proxy_id:s.proxy_id,country:'DE',exit_ip:'192.0.2.10',expires_at:new Date(Date.now()+900000).toISOString(),frame:frame(s)},201);return
  }
  const id=u.pathname.split('/')[5],s=sessions.get(id);
  if(!s){json({error:'browser_session_unavailable'},404);return}
  if(u.pathname.endsWith('/action')){
   const a=JSON.parse(raw);actions.push({id,...a});if(a.kind==='resize'){s.width=a.width;s.height=a.height}json(frame(s));return
  }
  if(req.method==='DELETE'){closed.push(id);sessions.delete(id);res.writeHead(204).end();return}
  res.writeHead(404).end();
 });await new Promise(r=>server.listen(0,'127.0.0.1',r));const base='http://127.0.0.1:'+server.address().port;
 const chrome=spawn(process.env.DOB_TEST_CHROMIUM||'/snap/bin/chromium',['--headless','--no-sandbox','--disable-dev-shm-usage','--remote-debugging-port=0','--user-data-dir='+path.join(root,'chrome'),'about:blank'],{stdio:['ignore','ignore','pipe']});
 let ws;
 try{
  const endpoint=await new Promise((resolve,reject)=>{let log='';chrome.stderr.on('data',x=>{log+=x;const m=log.match(/DevTools listening on (ws:\/\/\S+)/);if(m)resolve(m[1])});chrome.on('error',reject);chrome.on('exit',code=>reject(Error('Chrome exited '+code)));setTimeout(()=>reject(Error('Chrome timeout')),15000).unref()});
  ws=new WebSocket(endpoint);await new Promise((r,j)=>{ws.onopen=r;ws.onerror=j});let seq=0;const pending=new Map();
  ws.onmessage=({data})=>{const m=JSON.parse(data);if(m.method==='Network.requestWillBeSent')network.push(m.params.request.url);if(m.id){const p=pending.get(m.id);if(p){pending.delete(m.id);m.error?p[1](Error(m.error.message)):p[0](m.result)}}};
  const send=(method,params={},sessionId)=>new Promise((r,j)=>{const id=++seq;pending.set(id,[r,j]);ws.send(JSON.stringify({id,method,params,sessionId}))});
  async function attach(targetId){
   const {sessionId}=await send('Target.attachToTarget',{targetId,flatten:true}),call=(m,p)=>send(m,p,sessionId);await call('Page.enable');await call('Network.enable');
   const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true,userGesture:true});if(r.exceptionDetails)throw Error(r.exceptionDetails.exception?.description||r.exceptionDetails.text);return r.result.value};
   const wait=async expression=>{for(let i=0;i<160;i++){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,50))}throw Error('UI timeout '+expression+'; '+await evaluate('document.querySelector("[data-status]")?.textContent'))};
   return {targetId,call,evaluate,wait};
  }
  const {targetId}=await send('Target.createTarget',{url:'about:blank'}),parent=await attach(targetId);
  await parent.call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
  await parent.call('Page.navigate',{url:base+'/fixture-website'});await parent.wait('document.querySelector("h1")');
  fixtureImage=(await parent.call('Page.captureScreenshot',{format:'png'})).data;
  await parent.call('Page.navigate',{url:base+'/'});await parent.wait('typeof ProviderBrowser!=="undefined"&&document.querySelector("[name=proxy_country]")');
  await parent.evaluate('document.querySelector("#draft").click()');
  let childId;for(let i=0;i<100;i++){const list=(await send('Target.getTargets')).targetInfos;childId=list.find(x=>x.type==='page'&&x.url===base+'/static/provider-browser.html')?.targetId;if(childId)break;await new Promise(r=>setTimeout(r,50))}
  assert(childId,'draft action did not open a separate page');
  const child=await attach(childId);await child.call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});await child.wait('document.querySelector("canvas")&&!document.querySelector("canvas").hidden');
  assert.equal(await child.evaluate('window.opener===null'),true);
  assert.equal(await parent.evaluate('document.querySelector("[name=token]").value'),'unsaved-provider-secret');
  assert.equal(await parent.evaluate('document.querySelector("dialog.providerBrowser")===null'),true);
  assert.equal(await child.evaluate('sessionStorage.getItem("providerBrowser.launch").includes("unsaved-provider-secret")'),false);
  assert.deepEqual(opened[0].proxy_countries,['US','DE','FR']);assert.deepEqual(opened[0].proxy_ids,['one','two']);
  assert.equal(opened[0].account_id,'');assert.equal(opened.length,1);
  assert.match(await child.evaluate('document.querySelector("[data-route]").textContent'),/^DE/);
  assert.equal(await child.evaluate('document.querySelector("[data-address] input").value'),'https://provider.fixture/login');
  const rect=await child.evaluate('(()=>{const r=document.querySelector("canvas").getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height}})()');
  assert(rect.height>=680&&rect.width===390,'website does not fill mobile page');
  await child.evaluate('document.querySelector("[data-keyboard]").click()');
  await child.call('Input.insertText',{text:'سلام fixture'});
  await child.wait('document.querySelector("[data-ime]").value==="\u200b"');
  await new Promise(r=>setTimeout(r,100));assert(actions.some(a=>a.kind==='text'&&a.text==='سلام fixture'),'native text input was not forwarded');
  await child.evaluate('document.querySelector("canvas").dispatchEvent(new WheelEvent("wheel",{clientX:120,clientY:240,deltaY:320,bubbles:true,cancelable:true}))');
  await new Promise(r=>setTimeout(r,100));assert(actions.some(a=>a.kind==='scroll'&&a.delta===320));
  const mobile=(await child.call('Page.captureScreenshot',{format:'png'})).data;fs.writeFileSync(path.join(root,'provider-page-mobile.png'),Buffer.from(mobile,'base64'));
  if(process.env.DOB_BROWSER_SCREENSHOT_LOG==='1')console.log('BROWSER_UI_SCREENSHOT_BASE64 '+mobile);
  await child.call('Page.reload');await child.wait('document.querySelector("canvas")&&!document.querySelector("canvas").hidden');assert.equal(opened.length,1,'reload created a duplicate session');assert.equal(closed.length,0,'reload destroyed cookies/session');
  await child.evaluate('document.querySelector("[data-desktop]").click()');await new Promise(r=>setTimeout(r,450));
  assert(actions.some(a=>a.kind==='resize'&&a.width===1280&&a.mobile===false),'desktop site did not request wide layout');
  assert.equal(await child.evaluate('sessionStorage.getItem("providerBrowser.display")'),'desktop');
  await child.call('Page.reload');await child.wait('document.querySelector("canvas")&&!document.querySelector("canvas").hidden');
  assert.equal(await child.evaluate('document.querySelector("[data-desktop]").getAttribute("aria-pressed")'),'true','desktop preference lost on reload');
  await child.evaluate('document.querySelector("[data-desktop]").click()');await new Promise(r=>setTimeout(r,400));
  assert(actions.some(a=>a.kind==='resize'&&a.width===390&&a.mobile===true),'mobile view not restored');
  await child.call('Emulation.setDeviceMetricsOverride',{width:1365,height:900,deviceScaleFactor:1,mobile:false});await new Promise(r=>setTimeout(r,800));
  assert(actions.some(a=>a.kind==='resize'&&a.width===1365),'desktop resize not applied');
  assert.equal(await child.evaluate('document.documentElement.scrollWidth<=innerWidth&&document.documentElement.scrollHeight<=innerHeight'),true);
  fs.writeFileSync(path.join(root,'provider-page-desktop.png'),Buffer.from((await child.call('Page.captureScreenshot',{format:'png'})).data,'base64'));
  await child.call('Input.dispatchMouseEvent',{type:'mousePressed',x:100,y:200,button:'left',clickCount:1});
  await child.call('Input.dispatchMouseEvent',{type:'mouseReleased',x:100,y:200,button:'left',clickCount:1});
  await new Promise(r=>setTimeout(r,100));assert(actions.some(a=>a.kind==='pointer'&&a.phase==='down'));
  const before=opened.length;await parent.evaluate('window.realOpen=window.open;window.open=()=>null;document.querySelector("#saved").click()');await parent.wait('document.querySelector("#toast").textContent.includes("Allow a new tab")');assert.equal(opened.length,before);await parent.evaluate('window.open=window.realOpen;document.querySelector("#saved").click()');
  let secondId;for(let i=0;i<100;i++){secondId=(await send('Target.getTargets')).targetInfos.find(x=>x.type==='page'&&x.targetId!==childId&&x.url===base+'/static/provider-browser.html')?.targetId;if(secondId)break;await new Promise(r=>setTimeout(r,50))}
  assert(secondId);const second=await attach(secondId);await second.wait('document.querySelector("canvas")&&!document.querySelector("canvas").hidden');assert.equal(opened[1].account_id,'account-one');assert.equal(opened.length,2);
  await send('Target.closeTarget',{targetId:parent.targetId});
  await second.evaluate('document.querySelector("[data-nav=reload]").click()');await new Promise(r=>setTimeout(r,100));assert(actions.some(a=>a.id==='owned-session-2'&&a.kind==='reload'),'parent closure interrupted independent website');
  await child.evaluate('document.querySelector("[data-close]").click()');await child.wait('document.querySelector("[data-heading]").textContent==="Session closed"');assert.deepEqual(closed,['owned-session-1']);assert(sessions.has('owned-session-2'));
  await second.evaluate('document.querySelector("[data-close]").click()');await second.wait('document.querySelector("[data-heading]").textContent==="Session closed"');
  await second.evaluate('const config=JSON.parse(sessionStorage.getItem("providerBrowser.launch"));config.countries=["BO"];sessionStorage.setItem("providerBrowser.launch",JSON.stringify(config))');
  await second.call('Page.reload');await second.wait('document.querySelector("[data-setup]")&&!document.querySelector("[data-setup]").hidden&&document.querySelector("[data-message]").textContent.includes("Selected country")');
  assert.deepEqual(opened.at(-1).proxy_countries,['BO'],'missing country option widened the route');
  assert.equal(sessions.size,0,'failed route started a session');
  assert.equal(await second.evaluate('document.querySelector("[name=proxy_country]").value'),'BO');
  capacityBlocked=true;
  await second.evaluate('const x=JSON.parse(sessionStorage.getItem("providerBrowser.launch"));x.countries=["US"];sessionStorage.setItem("providerBrowser.launch",JSON.stringify(x))');
  await second.call('Page.reload');await second.wait('document.querySelector("[data-sessions] button")');
  assert.equal(await second.evaluate('document.querySelector("[data-address] input").disabled'),true,'inactive address bar accepted navigation');
  await second.evaluate('document.querySelector("[data-sessions] button").click()');await second.wait('!document.querySelector("canvas").hidden');
  assert.equal(capacityBlocked,false);assert.equal(sessions.size,1,'explicit owned-session close did not recover capacity');
  await second.evaluate('document.querySelector("[data-close]").click()');await second.wait('document.querySelector("[data-heading]").textContent==="Session closed"');
  assert(network.every(u=>u.startsWith(base)||u.startsWith('data:')||u==='about:blank'),'provider assets loaded in local browser');
  assert(network.every(u=>!u.includes('panel-session-fixture')&&!u.includes('unsaved-provider-secret')),'credential leaked into URL');
  console.log('STANDALONE_PROVIDER_TAB_DRAFT_SAVED_RELOAD_INPUT_RESIZE_PARENT_CLOSE_NO_LOCAL_PROVIDER_REQUESTS PASS');
 }finally{ws?.close();chrome.kill('SIGTERM');await new Promise(r=>server.close(r))}
})().catch(e=>{console.error(e.stack);process.exitCode=1});
