// Actual touchscreen/clipboard -> standalone page -> isolated Go Worker -> real provider DOM.
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict'),{spawn}=require('node:child_process');
(async()=>{
 const base=process.env.DOB_NATIVE_INPUT_URL,dir=fs.mkdtempSync(path.join(os.tmpdir(),'dob-native-input-'));
 const log=fs.openSync(path.join(dir,'chrome.log'),'w');
 const chrome=spawn(process.env.DOB_TEST_CHROMIUM,['--headless','--no-sandbox','--disable-dev-shm-usage','--remote-debugging-port=0','--user-data-dir='+dir,'about:blank'],{stdio:['ignore',log,log]});
 const delay=ms=>new Promise(r=>setTimeout(r,ms));let ws;
 try{
  let port;for(let i=0;i<100;i++){try{port=Number(fs.readFileSync(path.join(dir,'DevToolsActivePort'),'utf8').split('\n')[0]);break}catch{}await delay(100)}
  assert(port,'client Chrome failed to start');
  const v=await(await fetch('http://127.0.0.1:'+port+'/json/version')).json();ws=new WebSocket(v.webSocketDebuggerUrl);await new Promise((r,j)=>{ws.onopen=r;ws.onerror=j});
  let seq=0;const requests=new Map(),local=[];ws.onmessage=({data})=>{const m=JSON.parse(data);if(m.method==='Network.requestWillBeSent')local.push(m.params.request.url);if(m.id){const p=requests.get(m.id);requests.delete(m.id);m.error?p[1](Error(m.error.message)):p[0](m.result)}};
  const send=(method,params={},sessionId)=>new Promise((r,j)=>{const id=++seq;requests.set(id,[r,j]);ws.send(JSON.stringify({id,method,params,sessionId}))});
  const {targetId}=await send('Target.createTarget',{url:'about:blank'}),{sessionId}=await send('Target.attachToTarget',{targetId,flatten:true}),call=(m,p)=>send(m,p,sessionId);
  await call('Page.enable');await call('Network.enable');
  const ev=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true,userGesture:true});if(r.exceptionDetails)throw Error(r.exceptionDetails.exception?.description||r.exceptionDetails.text);return r.result.value};
  async function wait(expression){const end=Date.now()+18000;while(Date.now()<end){if(await ev(expression))return;await delay(100)}throw Error('Native input timeout: '+expression+'; state='+JSON.stringify(await ev('({title:document.title,status:document.querySelector("[data-status]")?.textContent,active:document.activeElement?.outerHTML.slice(0,300),singleHidden:document.querySelector("[data-ime]")?.hidden,multiHidden:document.querySelector("[data-ime-multiline]")?.hidden})')))}
  await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
  await call('Emulation.setTouchEmulationEnabled',{enabled:true,maxTouchPoints:5});
  await call('Page.addScriptToEvaluateOnNewDocument',{source:'sessionStorage.setItem("providerBrowser.token","input-fixture");sessionStorage.setItem("providerBrowser.launch",JSON.stringify({provider:"vultr",proxyIDs:["fixture"],countries:["DE"]}));'});
  await call('Page.navigate',{url:base+'/static/provider-browser.html'});await wait('document.querySelector("canvas")&&!document.querySelector("canvas").hidden&&document.title.includes("Native input fixture")');
  assert.match(await ev('document.querySelector("meta[name=viewport]").content'),/interactive-widget=resizes-content/);
  async function tap(x,y){
   const p=await ev('(()=>{const c=document.querySelector("canvas"),r=c.getBoundingClientRect(),s=Math.min(r.width/Number(c.dataset.viewportWidth),r.height/Number(c.dataset.viewportHeight));return{x:r.x+'+x+'*s,y:r.y+'+y+'*s}})()');
   await call('Input.dispatchTouchEvent',{type:'touchStart',touchPoints:[{x:p.x,y:p.y,id:1,radiusX:1,radiusY:1,force:1}]});
   await delay(25);
   await call('Input.dispatchTouchEvent',{type:'touchEnd',touchPoints:[]});
  }
  async function key(key,code,vk,modifiers=0){await call('Input.dispatchKeyEvent',{type:'keyDown',key,code,windowsVirtualKeyCode:vk,modifiers});await call('Input.dispatchKeyEvent',{type:'keyUp',key,code,windowsVirtualKeyCode:vk,modifiers})}
  await tap(190,210);
  await wait('document.title.includes("consent-dismissed")');
  assert.equal(await ev('document.querySelector("[data-ime]").hidden'),true,'occluded field opened keyboard under consent button');
  await tap(120,100);
  await wait('document.activeElement===document.querySelector("[data-ime]")');
  await delay(400);
  await call('Emulation.setDeviceMetricsOverride',{width:390,height:500,deviceScaleFactor:1,mobile:true});
  await wait('Number(document.querySelector("canvas").dataset.viewportHeight)<450');
  await call('Input.insertText',{text:'سلام fixture'});
  await wait('document.title.includes("input:سلام fixture|trusted")');
  await key('a','KeyA',65,2);await key('c','KeyC',67,2);await key('End','End',35);await key('v','KeyV',86,2);
  await wait('document.title.includes("input:سلام fixtureسلام fixture|trusted")');
  await ev('document.querySelector("[data-ime]").setSelectionRange(0,4)');
  await call('Input.insertText',{text:'Hello'});
  await wait('document.title.includes("input:Hello fixtureسلام fixture|trusted")');
  await ev('document.querySelector("[data-keyboard]").click()');
  assert.equal(await ev('document.querySelector("[data-ime]").value'),'Hello fixtureسلام fixture','Keyboard must not duplicate field content');
  await key('End','End',35);
  await call('Input.insertText',{text:'!'});
  await delay(45); // First edit is in flight when the next character and tap arrive.
  await call('Input.insertText',{text:'?'});
  await tap(190,310);
  await wait('document.title.includes("commit:Hello fixtureسلام fixture!?")');
  await tap(120,160);
  await wait('document.querySelector("[data-ime]").type==="password"&&!document.querySelector("[data-ime]").hidden');
  await call('Input.insertText',{text:'fixture-only-password'});
  await wait('document.title.includes("password-edited")');
  assert.equal(await ev('document.querySelector("[data-ime]").value'),'fixture-only-password');
  await key('Tab','Tab',9);
  await call('Input.insertText',{text:'line one'}); // Buffer typing while the native focus response is in flight.
  await wait('document.activeElement===document.querySelector("[data-ime-multiline]")');
  await call('Input.insertText',{text:'\nخط دوم'});
  await wait('document.title.includes("multiline:line one")&&document.title.includes("خط دوم")');
  await key('Tab','Tab',9,8);
  await wait('document.activeElement===document.querySelector("[data-ime]")&&document.querySelector("[data-ime]").type==="password"');
  assert.equal(await ev('document.querySelector("[data-ime]").value'),'fixture-only-password','reverse Tab must retain only the target field value');
  await ev('document.querySelector("[data-desktop]").click()');
  await wait('Number(document.querySelector("canvas").dataset.viewportWidth)===1280');
  await tap(190,310);
  await wait('document.title.includes("commit:Hello fixtureسلام fixture!?")');
  await ev('document.querySelector("[data-close]").click()');await wait('document.querySelector("[data-heading]").textContent==="Session closed"');
  assert.equal(await ev('document.querySelector("[data-ime]").value+document.querySelector("[data-ime-multiline]").value'),'');
  assert.equal(await ev('JSON.stringify(sessionStorage).includes("fixture-only-password")'),false);
  assert(local.every(u=>u.startsWith(base+'/')||u.startsWith('data:')||u==='about:blank'),'device requested provider content directly');
  console.log('NATIVE_TOUCH_CONSENT_KEYBOARD_RESIZE_UNICODE_COPY_PASTE_SELECTION_PASSWORD_TAB_MULTILINE_END PASS');
 }finally{ws?.close();chrome.kill('SIGTERM');fs.closeSync(log);await delay(200);fs.rmSync(dir,{recursive:true,force:true})}
})().catch(e=>{console.error(e.stack);process.exitCode=1});
