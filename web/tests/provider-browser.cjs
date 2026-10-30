// Exercise the real mobile UI with isolated control fixtures. Native networking
// and real profile separation are covered by TestProviderBrowserNativeSandbox.
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),http=require('node:http'),assert=require('node:assert/strict');
const {spawn}=require('node:child_process');
(async()=>{
 const root=fs.mkdtempSync(path.join(os.tmpdir(),'dob-provider-browser-ui-')),opened=[],actions=[],closed=[],requests=[];
 const frame={isolated:true,width:390,height:800,image:'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aN1cAAAAASUVORK5CYII=',url:'https://provider.fixture/login',title:'Provider fixture',target:'tab1',tabs:[{id:'tab1',title:'Provider fixture',url:'https://provider.fixture/login'}]};
 const server=http.createServer(async(req,res)=>{
  const u=new URL(req.url,'http://localhost');requests.push(u.pathname);
  const json=x=>{res.setHeader('Content-Type','application/json');res.end(JSON.stringify(x))};
  if(u.pathname==='/'){res.setHeader('Content-Type','text/html');res.end('<!doctype html><html><head><meta name="viewport" content="width=device-width, initial-scale=1"><link rel="stylesheet" href="/static/app.css"><link rel="stylesheet" href="/static/provider-browser.css"></head><body><script src="/static/proxy-country.js"></script><script src="/static/provider-browser.js"></script></body></html>');return}
  if(u.pathname.startsWith('/static/')){const name=path.basename(u.pathname),file=path.join(process.cwd(),'web/static',name);if(!fs.existsSync(file)){res.writeHead(404).end();return}res.setHeader('Content-Type',name.endsWith('.js')?'text/javascript':'text/css');res.end(fs.readFileSync(file));return}
  if(u.pathname==='/api/v1/proxies'){json([{id:'one',name:'First proxy',status:'healthy'},{id:'two',name:'Second proxy',status:'healthy'}]);return}
  if(u.pathname==='/api/v1/proxy-drivers'){json({country_options:[{code:'US',label:'United States'},{code:'DE',label:'Germany'},{code:'FR',label:'France'}]});return}
  let raw='';for await(const b of req)raw+=b;
  if(u.pathname==='/api/v1/provider-browser/sessions'){opened.push(JSON.parse(raw));json({id:'owned-session',country:'DE',exit_ip:'192.0.2.10',frame});return}
  if(u.pathname.endsWith('/action')){actions.push(JSON.parse(raw));json(frame);return}
  if(req.method==='DELETE'){closed.push(u.pathname);res.writeHead(204).end();return}
  res.writeHead(404).end();
 });await new Promise(r=>server.listen(0,'127.0.0.1',r));const base='http://127.0.0.1:'+server.address().port;
 const chrome=spawn(process.env.DOB_TEST_CHROMIUM||'/snap/bin/chromium',['--headless','--no-sandbox','--disable-dev-shm-usage','--remote-debugging-port=0','--user-data-dir='+path.join(root,'chrome'),'about:blank'],{stdio:['ignore','ignore','pipe']});
 let ws;
 try{
  const endpoint=await new Promise((resolve,reject)=>{let log='';chrome.stderr.on('data',x=>{log+=x;const m=log.match(/DevTools listening on (ws:\/\/\S+)/);if(m)resolve(m[1])});chrome.on('error',reject);chrome.on('exit',code=>reject(Error('Chrome exited '+code)));setTimeout(()=>reject(Error('Chrome timeout')),15000).unref()});
  ws=new WebSocket(endpoint);await new Promise((resolve,reject)=>{ws.onopen=resolve;ws.onerror=reject});let seq=0;const pending=new Map(),network=[];
  ws.onmessage=e=>{const m=JSON.parse(e.data);if(m.method==='Network.requestWillBeSent')network.push(m.params.request.url);if(m.id){const p=pending.get(m.id);pending.delete(m.id);if(m.error)p[1](Error(m.error.message));else p[0](m.result)}};
  const send=(method,params={},sessionId)=>new Promise((resolve,reject)=>{const id=++seq;pending.set(id,[resolve,reject]);ws.send(JSON.stringify({id,method,params,sessionId}))});
  const {targetId}=await send('Target.createTarget',{url:'about:blank'}),{sessionId}=await send('Target.attachToTarget',{targetId,flatten:true}),call=(m,p)=>send(m,p,sessionId);
  await call('Page.enable');await call('Network.enable');await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
  const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(r.exceptionDetails.exception?.description||r.exceptionDetails.text);return r.result.value};
  const wait=async expression=>{for(let i=0;i<100;i++){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,50))}throw Error('UI timeout '+expression+'; '+await evaluate('document.querySelector("[data-status]")?.textContent'))};
  await call('Page.navigate',{url:base+'/'});await wait('typeof ProviderBrowser!=="undefined"');
  await evaluate(`window.testApi=async(path,opt={})=>{const r=await fetch(path,opt);return r.status===204?null:r.json()};ProviderBrowser.open({api:testApi,provider:'vultr'})`);
  await wait('document.querySelector("[data-setup] button")');
  assert.deepEqual(await evaluate('ProxyCountry.names.map(n=>document.querySelector("[name="+n+"]").value)'),['AUTO','AUTO','AUTO']);
  await evaluate('document.querySelector("[data-setup] button").click()');assert.equal(opened.length,0);
  await evaluate('document.querySelector("[name=browser_proxy0]").value="one";document.querySelector("[name=browser_proxy1]").value="two";ProxyCountry.set(document.querySelector("[data-setup]"),["US","DE","FR"]);document.querySelector("[data-setup] button").click()');
  await wait('!document.querySelector("[data-browser]").hidden&&document.querySelector("[data-status]").textContent==="Provider fixture"');
  assert.deepEqual(opened[0].proxy_countries,['US','DE','FR']);assert.deepEqual(opened[0].proxy_ids,['one','two']);
  assert.equal(await evaluate('document.querySelector("[data-address] input").value'),'https://provider.fixture/login');
  assert.match(await evaluate('document.querySelector("[data-route]").textContent'),/Verified country: DE/);
  await evaluate('document.querySelector("[data-type] input").value="fixture text";document.querySelector("[data-type] button").click()');await wait('document.querySelector("[data-type] input").value===""');
  await new Promise(r=>setTimeout(r,100));assert(actions.some(x=>x.kind==='text'&&x.text==='fixture text'));
  assert(network.every(u=>u.startsWith(base)||u.startsWith('data:')||u==='about:blank'),'provider assets must never load in the local browser');
  assert.equal(await evaluate('document.querySelector("dialog").getBoundingClientRect().width<=innerWidth'),true);
  const screenshot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,'provider-browser-mobile.png'),Buffer.from(screenshot.data,'base64'));
  if(process.env.DOB_BROWSER_SCREENSHOT_LOG==='1')console.log('BROWSER_UI_SCREENSHOT_BASE64 '+screenshot.data);
  await evaluate('document.querySelector("[data-close]").click()');await wait('!document.querySelector("dialog")');await new Promise(r=>setTimeout(r,100));assert.equal(closed.length,1);
  console.log('PROVIDER_BROWSER_MOBILE_CONTROL_NO_LOCAL_PROVIDER_REQUESTS_AND_THREE_PRIORITY_BINDING PASS');
 }finally{ws?.close();chrome.kill('SIGTERM');await new Promise(r=>server.close(r))}
})().catch(e=>{console.error(e);process.exitCode=1});
