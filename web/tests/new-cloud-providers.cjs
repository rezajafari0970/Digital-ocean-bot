const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),{spawn}=require('node:child_process'),assert=require('node:assert/strict');
(async()=>{
 const base=process.env.DOB_UI_BASE,token='isolated-browser-fixture';
 if(!base?.startsWith('http://127.0.0.1:')||!token)throw Error('local ephemeral session required');
 const root=process.env.DOB_UI_ARTIFACT_DIR||os.tmpdir(),profile=fs.mkdtempSync(path.join(root,'ui-profile-'));
 const log=fs.openSync(path.join(root,'browser.log'),'w');
 const chrome=spawn(process.env.DOB_TEST_CHROMIUM||'/snap/bin/chromium',['--headless','--no-sandbox','--disable-dev-shm-usage','--disable-gpu','--remote-debugging-port=0','--remote-debugging-address=127.0.0.1','--user-data-dir='+profile,'about:blank'],{stdio:['ignore',log,log],env:{...process.env,TMPDIR:root}});
 let ws;try{
  const until=Date.now()+20000;let port;
  while(Date.now()<until){try{port=Number(fs.readFileSync(path.join(profile,'DevToolsActivePort'),'utf8').split('\n')[0]);break}catch{};await new Promise(r=>setTimeout(r,200))}
  if(!port)throw Error('browser did not start');
  const version=await(await fetch('http://127.0.0.1:'+port+'/json/version')).json();
  ws=new WebSocket(version.webSocketDebuggerUrl);await new Promise((r,j)=>{ws.onopen=r;ws.onerror=j});
  let seq=0;const pending=new Map();
  ws.onmessage=({data})=>{const m=JSON.parse(data);if(m.id){const h=pending.get(m.id);if(h){pending.delete(m.id);m.error?h[1](Error(m.error.message)):h[0](m.result)}}};
  const send=(method,params={},sessionId)=>new Promise((resolve,reject)=>{const id=++seq;pending.set(id,[resolve,reject]);ws.send(JSON.stringify({id,method,params,sessionId}))});
  const {targetId}=await send('Target.createTarget',{url:'about:blank'});
  const {sessionId}=await send('Target.attachToTarget',{targetId,flatten:true});
  const call=(m,p)=>send(m,p,sessionId);
  await call('Page.enable');await call('Runtime.enable');
  await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
  await call('Page.addScriptToEvaluateOnNewDocument',{source:'if(location.origin==='+JSON.stringify(base)+'){localStorage.setItem("token",'+JSON.stringify(token)+')}'});
  await call('Page.addScriptToEvaluateOnNewDocument',{source:'if(location.origin==='+JSON.stringify(base)+'){window.__authWindows=[];window.__authOpenCalls=0;window.open=()=>{window.__authOpenCalls++;if('+JSON.stringify(process.env.DOB_TEST_AUTH_POPUP_BLOCKED==='1')+')return null;const w={opener:window,closed:false,document:document.implementation.createHTMLDocument(""),location:{href:"about:blank",replace(url){this.href=url}},close(){this.closed=true}};window.__authWindows.push(w);return w}}'});

  const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error('page evaluation failed: '+(r.exceptionDetails.exception?.description||r.exceptionDetails.text));return r.result.value};

  await call('Page.navigate',{url:base+'/admin/#accounts'});
  async function waitFor(expression){const end=Date.now()+12000;while(Date.now()<end){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,100))}throw Error('UI assertion timeout: '+expression+'; state='+JSON.stringify(await evaluate('({url:location.href,ready:document.readyState,viewType:typeof activePageView,current:typeof current==="undefined"?null:current,loading:typeof activePageView==="undefined"?null:activePageView?.loading,appHidden:document.querySelector("#app")?.hidden,handler:typeof document.querySelector("#primaryAction")?.onclick,rendered:document.querySelector("#content")?.dataset.renderedPage})')))}
  // Wait for the route and event handlers, not just the static HTML button.
  await waitFor('typeof activePageView!=="undefined"&&current==="accounts"&&activePageView&&!activePageView.loading&&!document.querySelector("#app").hidden&&typeof document.querySelector("#primaryAction").onclick==="function"');
  await evaluate('document.querySelector("#primaryAction").click()');
  await waitFor('document.querySelector("#modalFields [name=provider]")!==null');
  assert.deepEqual((await evaluate('[...document.querySelector("#modalFields [name=provider]").options].map(x=>x.value)')).sort(),['aws','digitalocean','kamatera','linode','ovh','upcloud','vultr']);

  async function assertSaveVisible(label){
   const state=await evaluate('(()=>{const e=document.querySelector("#modalSubmit"),r=e.getBoundingClientRect(),v=window.visualViewport;return {hidden:e.hidden,width:r.width,height:r.height,top:r.top,bottom:r.bottom,right:r.right,viewportHeight:v?.height||innerHeight,viewportWidth:v?.width||innerWidth,hit:document.elementFromPoint(r.x+r.width/2,r.y+r.height/2)?.id}})()');
   assert.equal(state.hidden,false,label);assert.ok(state.width>20&&state.height>=40&&state.top>=0&&state.bottom<=state.viewportHeight+1&&state.right<=state.viewportWidth+1,label+JSON.stringify(state));assert.equal(state.hit,'modalSubmit',label);
  }
  await assertSaveVisible('initial Save is visible');
  await evaluate('document.querySelector("[name=provider]").value="kamatera";document.querySelector("[name=provider]").dispatchEvent(new Event("change"))');
  assert.deepEqual(await evaluate('[...document.querySelectorAll("[data-provider-credential]")].map(x=>({key:x.dataset.providerCredential,type:x.type}))'),[{key:'client_id',type:'password'},{key:'secret',type:'password'}]);
  await evaluate('(()=>{for(const [k,v] of Object.entries({client_id:"fixture-client",secret:"fixture-secret"})){const el=document.querySelector("[data-provider-credential="+k+"]");el.value=v;el.dispatchEvent(new Event("input"))}})()');
  assert.deepEqual(JSON.parse(await evaluate('document.querySelector("[name=token]").value')),{client_id:'fixture-client',secret:'fixture-secret'});
  assert.equal(await evaluate('document.querySelector("#modalSubmit").disabled'),true);

  // Reproduce the phone recording with the native scoped identity and long plan labels.
  async function prepareKamatera(){
   await evaluate('document.querySelector("[name=provider]").value="kamatera";document.querySelector("[name=provider]").dispatchEvent(new Event("change"));document.querySelector("[name=name]").value="Kamatera fixture";for(const [key,value] of Object.entries({client_id:"fixture-client",secret:"fixture-secret"})){const el=document.querySelector("[data-provider-credential="+key+"]");el.value=value;el.dispatchEvent(new Event("input"))}document.querySelector("#discoverAccount").click()');
   await waitFor('cache.accountPreview?.provider==="kamatera"&&!document.querySelector("#modalSubmit").disabled&&!document.querySelector("#discoverAccount").disabled');
  }
  await prepareKamatera();
  for(const size of [{width:320,height:480},{width:390,height:420},{width:844,height:390},{width:390,height:844}]){
   await call('Emulation.setDeviceMetricsOverride',{...size,deviceScaleFactor:1,mobile:true});
   assert.equal(await evaluate('(()=>{const e=document.querySelector("#modalFields");return e.scrollWidth<=e.clientWidth+1})()'),true,'Kamatera fields must not scroll horizontally '+JSON.stringify(await evaluate('(()=>{const e=document.querySelector("#modalFields"),r=e.getBoundingClientRect();return {viewport:innerWidth,width:e.clientWidth,scroll:e.scrollWidth,children:[...e.querySelectorAll("*")].filter(x=>x.getBoundingClientRect().right>r.right+1).slice(0,10).map(x=>({tag:x.tagName,name:x.name,id:x.id,width:x.clientWidth,scroll:x.scrollWidth,text:x.textContent.slice(0,80)}))}})()')));
   await assertSaveVisible('Kamatera '+JSON.stringify(size));
  }
  assert.equal(await evaluate('document.querySelector(".discoveryOk").textContent.includes("kamatera-key:")'),false);
  assert.equal(await evaluate('document.querySelector("[name=size2]").value'), '');
  assert.equal(await evaluate('document.querySelector("[name=size3]").value'), '');
  await evaluate('document.querySelector("[name=size2]").value="2B-4096-30";document.querySelector("[name=size2]").dispatchEvent(new Event("change"));document.querySelector("[name=size]").value="2B-4096-30";document.querySelector("[name=size]").dispatchEvent(new Event("change"))');
  assert.equal(await evaluate('document.querySelector("[name=size2]").value'), '');
  await evaluate('document.querySelector("[name=name]").value="";document.querySelector("#modalSubmit").click()');
  assert.equal(await evaluate('fetch("/fixture/save-count").then(r=>r.json())'),0,'missing name must not dispatch');
  await evaluate('document.querySelector("[name=name]").value="Kamatera fixture";cache.accounts=[{id:"stale-fixture"}];document.querySelector("#modalSubmit").click();document.querySelector("#modalSubmit").click();document.querySelector("#modalForm").dispatchEvent(new Event("submit",{cancelable:true}))');
  await waitFor('document.querySelector("#modalSubmit").textContent==="Saving…"');
  assert.equal(await evaluate('document.querySelector("#modalSubmit").disabled&&document.querySelector("[name=provider]").disabled&&document.querySelector("[name=size]").disabled'),true);
  await waitFor('document.querySelector("#modalError").textContent.includes("The fixture rejected")');
  assert.equal(await evaluate('fetch("/fixture/save-count").then(r=>r.json())'),1,'one request per pending save');
  assert.equal(await evaluate('cache.accounts===null'),true,'unconfirmed save invalidates cached account list');
  assert.equal(await evaluate('document.querySelector("#modalSubmit").textContent'),'Save');
  assert.equal(await evaluate('document.querySelector("#modalSubmit").disabled'),false);
  assert.equal(await evaluate('document.querySelector("[name=size]").value'),'2B-4096-30');
  assert.equal(await evaluate('document.querySelector("[data-provider-credential=secret]").value'),'fixture-secret');
  await assertSaveVisible('rejected save');
  await evaluate('fetch("/fixture/save-mode?mode=success",{method:"POST"})');
  await evaluate('document.querySelector("#modalSubmit").click()');
  await waitFor('!document.querySelector("#modal").open');
  assert.equal(await evaluate('fetch("/fixture/save-count").then(r=>r.json())'),2);
  await waitFor('!activePageView.loading');
  // Closing a pending form must detach both success and error callbacks.
  for(const mode of ['reject','success']){
   await evaluate('document.querySelector("#primaryAction").click()');
   await waitFor('document.querySelector("#modal").open');await prepareKamatera();
   await evaluate('fetch("/fixture/save-mode?mode='+mode+'",{method:"POST"})');
   await evaluate('document.querySelector("#modalSubmit").click();document.querySelector("#modalCancel").click();document.querySelector("#primaryAction").click()');
   await waitFor('document.querySelector("#modal").open&&document.querySelector("[name=provider]")?.disabled===false');
   await evaluate('document.querySelector("[name=provider]").value="linode";document.querySelector("[name=provider]").dispatchEvent(new Event("change"));document.querySelector("[name=name]").value="New untouched draft";cache.accounts=[{id:"stale-fixture"}]');
   await new Promise(r=>setTimeout(r,1000));
   assert.equal(await evaluate('document.querySelector("#modal").open&&document.querySelector("[name=provider]").value==="linode"&&document.querySelector("[name=name]").value==="New untouched draft"&&document.querySelector("#modalError").textContent===""&&document.querySelector("#modalSubmit").disabled'),true,'late '+mode+' altered the new form');
   assert.equal(await evaluate('cache.accounts===null'),true,'late '+mode+' invalidates accounts without touching the new form');
   await evaluate('document.querySelector("#modalCancel").click()');
  }
  await evaluate('document.querySelector("#primaryAction").click()');await waitFor('document.querySelector("#modal").open');
  console.log('KAMATERA_MOBILE_LAYOUT_UNIQUE_CHOICES_SINGLE_SAVE_AND_STALE_RESPONSE PASS');

  for(const provider of ['aws','ovh']){
   await evaluate('document.querySelector("#modalFields [name=provider]").value='+JSON.stringify(provider)+';document.querySelector("#modalFields [name=provider]").dispatchEvent(new Event("change"))');
   const fields=await evaluate('[...document.querySelectorAll("[data-provider-credential]")].map(x=>({key:x.dataset.providerCredential,type:x.type}))');
   assert.deepEqual(fields.map(f=>f.key).sort(),(provider==='aws'?['access_key_id','secret_access_key','session_token','region','subnet_id','security_group_ids']:['application_key','application_secret','consumer_key','endpoint','project_id','pricing_subsidiary']).sort());
   for(const f of fields.filter(x=>/secret|key|token/.test(x.key)&&x.key!=='project_id'))assert.equal(f.type,'password');
   const credentialSelector='[data-provider-credential="'+(provider==='aws'?'access_key_id':'application_key')+'"]';
   await evaluate('(()=>{const el=document.querySelector('+JSON.stringify(credentialSelector)+');el.value="fixture-credential";el.dispatchEvent(new Event("input"))})()');
   const serialized=JSON.parse(await evaluate('document.querySelector("#modalFields [name=token]").value'));
   assert.equal(serialized[provider==='aws'?'access_key_id':'application_key'],'fixture-credential');
   assert.equal(serialized.region,undefined);assert.equal(serialized.subnet_id,undefined);assert.equal(serialized.endpoint,undefined);assert.equal(await evaluate('document.querySelector("#providerCredentialFields details")?.open'),false);
   assert.equal(await evaluate('cache.accountPreview===null&&!document.querySelector("#modalSubmit").hidden&&document.querySelector("#modalSubmit").disabled'),true);
   assert.equal(await evaluate('document.documentElement.scrollWidth<=window.innerWidth+1'),true);
   const snap=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,provider+'-account-mobile.png'),Buffer.from(snap.data,'base64'));
  }
  for(const provider of ['aws','ovh']){
   await evaluate('document.querySelector("#modalFields [name=provider]").value='+JSON.stringify(provider)+';document.querySelector("#modalFields [name=provider]").dispatchEvent(new Event("change"))');
   await evaluate('(()=>{document.querySelector("#modalFields [name=name]").value="Automatic fixture";for(const el of document.querySelectorAll("[data-provider-credential]")){if(el.type==="password"){el.value=el.dataset.providerCredential.includes("secret")?"fixture-secret":"fixture-credential";el.dispatchEvent(new Event("input"))}}document.querySelector("#discoverAccount").click()})()');
   if(provider==='aws'){
    await waitFor('document.querySelector("#modalError").textContent.includes("InvalidParameterValue")');
    assert.equal(await evaluate('document.querySelector("#discoverAccount").textContent'),'Validate credential & load provider options');
    const message=await evaluate('document.querySelector("#modalError").textContent');
    for(const part of ['DescribeRegions','aws-browser-trace','HTTP 400'])assert.ok(message.includes(part));
    assert.equal(message.includes('private-provider-message'),false);
    assert.equal(await evaluate('cache.accountPreview===null&&!document.querySelector("#modalSubmit").hidden&&document.querySelector("#modalSubmit").disabled'),true);
    assert.equal(await evaluate('document.querySelector("[data-provider-credential=secret_access_key]").value'),'fixture-secret');
    assert.equal(await evaluate('document.documentElement.scrollWidth<=window.innerWidth+1'),true);
    await evaluate('document.querySelector("#modalError").scrollIntoView({block:"end"})');
    const errorSnap=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,'aws-error-mobile.png'),Buffer.from(errorSnap.data,'base64'));
    await evaluate('document.querySelector("#discoverAccount").click()');
   }
   if(provider==='ovh'){
    await waitFor('document.querySelector("[data-auth-link]")&&!document.querySelector("[data-auth-link]").hidden');
    assert.equal(await evaluate('document.querySelector("[data-provider-credential=consumer_key]").closest("label").hidden'),true);
    assert.equal(await evaluate('document.querySelector("#modalFields [name=token]").value'),'');
    assert.equal(await evaluate('document.querySelector("[data-auth-link]").rel'),'noopener noreferrer');
    assert.equal(await evaluate('document.querySelector("[data-auth-link]").href'),'https://auth.eu.ovhcloud.com/sso/api?credentialToken=fixture-validation');
    assert.equal(await evaluate('JSON.stringify(sessionStorage).includes("fixture-secret")'),false);
    assert.equal(await evaluate('document.querySelector("#providerCredentialFields").hidden'),true);
    assert.equal(await evaluate('document.querySelector("#discoverAccount").hidden'),true);
    assert.equal(await evaluate('window.__authOpenCalls'),1);
    if(process.env.DOB_TEST_AUTH_POPUP_BLOCKED!=='1'){
     assert.equal(await evaluate('window.__authWindows[0].opener===null'),true);
     assert.equal(await evaluate('window.__authWindows[0].document.querySelector("meta[name=referrer]").content'),'no-referrer');
     assert.equal(await evaluate('window.__authWindows[0].location.href'),'https://auth.eu.ovhcloud.com/sso/api?credentialToken=fixture-validation');
    }
    assert.equal(await evaluate('document.documentElement.scrollWidth<=window.innerWidth+1'),true);
    const authSnap=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,'ovh-approval-mobile.png'),Buffer.from(authSnap.data,'base64'));

    const saved=await evaluate('sessionStorage.getItem("provider-authorization:ovh:v1")');
    assert.match(saved,/^[0-9a-f-]{36}$/);
    // Close/reopen and then a full reload preserve the opaque request ID only.
    await evaluate('document.querySelector("#modalCancel").click();document.querySelector("#primaryAction").click()');
    await waitFor('document.querySelector("#modal").open');
    await evaluate('document.querySelector("#modalFields [name=provider]").value="ovh";document.querySelector("#modalFields [name=provider]").dispatchEvent(new Event("change"))');
    await waitFor('document.querySelector("[data-auth-link]")&&!document.querySelector("[data-auth-link]").hidden');
    await call('Page.reload',{ignoreCache:true});
    await waitFor('typeof activePageView!=="undefined"&&current==="accounts"&&activePageView&&!activePageView.loading&&!document.querySelector("#app").hidden');
    await evaluate('document.querySelector("#primaryAction").click()');
    await waitFor('document.querySelector("#modal").open');
    await evaluate('document.querySelector("#modalFields [name=provider]").value="ovh";document.querySelector("#modalFields [name=provider]").dispatchEvent(new Event("change"));document.querySelector("#modalFields [name=name]").value="Resumed fixture"');
    await waitFor('document.querySelector("[data-auth-link]")&&!document.querySelector("[data-auth-link]").hidden');
    assert.equal(await evaluate('sessionStorage.getItem("provider-authorization:ovh:v1")'),saved);
    assert.equal(await evaluate('document.querySelector("[data-provider-credential=application_secret]").value'),'');
    assert.equal(await evaluate('window.__authOpenCalls'),0);
    await evaluate('fetch("/fixture/approve",{method:"POST"}).then(()=>window.dispatchEvent(new Event("focus")))');
    // Activation is polled automatically; no second Connect click.
    await waitFor('document.querySelector("[data-discovery-key=project_id]")!==null');
    assert.equal(await evaluate('cache.accountPreview===null&&!document.querySelector("#modalSubmit").hidden&&document.querySelector("#modalSubmit").disabled'),true);
    await evaluate('document.querySelector("[data-discovery-key=project_id]").value="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb";document.querySelector("[data-discovery-key=project_id]").dispatchEvent(new Event("change"))');
   }
   await waitFor('cache.accountPreview!==null&&!document.querySelector("#modalSubmit").hidden');
   if(provider==='ovh')assert.match(await evaluate('document.querySelector("[data-auth-status]").textContent'),/Connected to OVH/);

   if(provider==='ovh'){
    assert.equal(await evaluate('document.querySelector("[name=region]").value'),'GRA11');
    assert.equal(await evaluate('[...document.querySelectorAll("#accountDiscovery select[name^=region] option")].some(o=>o.value==="SBG")'),false);
    assert.equal(await evaluate('document.querySelector("#accountSize").value'),'test-plan');
    assert.equal(await evaluate('document.querySelector("#modalSubmit").disabled'),false);
    await evaluate('document.querySelector("[name=region]").value="BHS5";document.querySelector("[name=region]").dispatchEvent(new Event("change"))');
    assert.equal(await evaluate('document.querySelector("[name=image]").value'),'ubuntu-bhs');
    await evaluate('fetch("/fixture/catalog?empty=1",{method:"POST"})');
    await evaluate('document.querySelector("#discoverAccount").click()');
    await waitFor('document.querySelector("#modalError").textContent.includes("no region currently")');
    assert.equal(await evaluate('cache.accountPreview===null&&!document.querySelector("#modalSubmit").hidden&&document.querySelector("#modalSubmit").disabled'),true);
    assert.equal(await evaluate('document.querySelector("[data-auth-status]").textContent.includes("options are ready")'),false);
    await evaluate('fetch("/fixture/catalog?empty=0",{method:"POST"})');
    await evaluate('document.querySelector("#discoverAccount").click()');
    await waitFor('cache.accountPreview!==null&&!document.querySelector("#modalSubmit").hidden&&!document.querySelector("#modalSubmit").disabled');
   }
   assert.equal(await evaluate('document.documentElement.scrollWidth<=window.innerWidth+1'),true);
   if(provider==='aws'){

    await assertSaveVisible('verified Save visible before network consent');
    for(const mode of ['gateway','truncated']){
     await evaluate('fetch("/fixture/preview-mode?mode='+mode+'",{method:"POST"})');
     await evaluate('document.querySelector("#discoverAccount").click()');
     await waitFor('!document.querySelector("#discoverAccount").disabled&&document.querySelector("#modalError").textContent.length>0');
     assert.equal(await evaluate('document.querySelector("#modalError").textContent.includes("<html")'),false);
     assert.equal(await evaluate('cache.accountPreview===null&&document.querySelector("#modalSubmit").disabled'),true);
     await assertSaveVisible(mode+' leaves Save visible and disabled');
    }
    await evaluate('fetch("/fixture/preview-mode?mode=slow",{method:"POST"})');
    await evaluate('document.querySelector("#discoverAccount").click()');
    await waitFor('document.querySelector("#accountPreviewStatus").textContent.includes("Loading available plans")');
    await assertSaveVisible('slow preview');
    assert.equal(await evaluate('document.querySelector("#modalSubmit").disabled&&document.querySelector("#discoverAccount").disabled'),true);
    await waitFor('cache.accountPreview!==null&&!document.querySelector("#discoverAccount").disabled');
    await evaluate('fetch("/fixture/preview-mode?mode=",{method:"POST"})');
    assert.match(await evaluate('document.querySelector("#accountSize").selectedOptions[0].textContent'),/USD 0.0114\/h compute/);
    assert.deepEqual(await evaluate('[...document.querySelectorAll("[data-discovery-key^=region_]")].map(el=>({value:el.value,empty:[...el.options].filter(o=>o.value==="").length,primaryDisabled:[...el.options].find(o=>o.value==="eu-west-1").disabled}))'),Array.from({length:4},()=>({value:'',empty:1,primaryDisabled:true})));
    await evaluate('(()=>{const change=(key,value)=>{const el=document.querySelector("[data-discovery-key="+key+"]");el.value=value;el.dispatchEvent(new Event("change"))};change("region_2","us-east-1");change("region","us-east-1")})()');
    assert.equal(await evaluate('document.querySelector("[data-discovery-key=region_2]").value'),'');
    assert.equal(await evaluate('document.querySelector("[data-discovery-key=subnet_id]").disabled'),true);
    assert.equal(await evaluate('document.querySelector("[data-discovery-key=subnet_id]").value'),'');
    assert.equal(await evaluate('cache.accountPreview===null&&!document.querySelector("#modalSubmit").hidden&&document.querySelector("#modalSubmit").disabled'),true);
    await evaluate('(()=>{const el=document.querySelector("[data-discovery-key=region]");el.value="eu-west-1";el.dispatchEvent(new Event("change"));document.querySelector("#discoverAccount").click()})()');
    await waitFor('cache.accountPreview!==null&&!document.querySelector("[data-discovery-key=subnet_id]").disabled');
    // Reproduce the reported four identical selections, including stale state
    // that arrives without change events. None of them may reach the API.
    await evaluate('document.querySelectorAll("[data-discovery-key^=region_]").forEach(el=>el.value="eu-west-1");document.querySelector("#discoverAccount").click()');
    await waitFor('cache.accountPreview!==null&&!document.querySelector("#modalSubmit").hidden');
    assert.deepEqual(await evaluate('[...document.querySelectorAll("[data-discovery-key^=region_]")].map(el=>el.value)'),['','','','']);
    assert.equal(await evaluate('document.querySelector("#modalError").textContent'),'');
    await evaluate('document.querySelector("[data-discovery-key=region_2]").scrollIntoView({block:"center"})');
    const discoverySnap=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,'aws-distinct-discovery-mobile.png'),Buffer.from(discoverySnap.data,'base64'));

    await evaluate('document.querySelector("[name=lifetime_min_minutes]").value="1500";document.querySelector("[name=lifetime_max_minutes]").value="1600";document.querySelector("[name=desired_server_count]").value="10";document.querySelector("[data-discovery-key=region_2]").value="us-east-1";document.querySelector("[data-discovery-key=region_2]").dispatchEvent(new Event("change"));document.querySelector("#discoverAccount").click()');
    await waitFor('cache.accountPreview!==null&&cache.accountPreview.regions.length===2&&!document.querySelector("#modalSubmit").hidden');

    await evaluate('(()=>{const el=document.querySelector("[data-discovery-key=region_2]");el.value="us-west-1";el.dispatchEvent(new Event("change"))})()');
    assert.equal(await evaluate('document.querySelector("[data-discovery-key=subnet_2]").disabled'),true);
    assert.equal(await evaluate('document.querySelector("[data-discovery-key=subnet_2]").value'),'');
    await evaluate('(()=>{const el=document.querySelector("[data-discovery-key=region_2]");el.value="";el.dispatchEvent(new Event("change"));document.querySelector("#discoverAccount").click()})()');
    await waitFor('cache.accountPreview!==null&&cache.accountPreview.regions.length===1');
    assert.equal(await evaluate('document.querySelector("[data-discovery-key=subnet_2]")===null'),true);
    await evaluate('(()=>{const el=document.querySelector("[data-discovery-key=region_2]");el.value="us-east-1";el.dispatchEvent(new Event("change"));document.querySelector("#discoverAccount").click()})()');
    await waitFor('cache.accountPreview!==null&&cache.accountPreview.regions.length===2');
    assert.equal(await evaluate('document.querySelector("[name=lifetime_min_minutes]").value'),'1500');
    assert.equal(await evaluate('document.querySelector("[name=lifetime_max_minutes]").value'),'1600');
    assert.equal(await evaluate('document.querySelector("[name=desired_server_count]").value'),'10');
    assert.equal(await evaluate('document.querySelector("#modalSubmit").disabled'),true);
    assert.equal(await evaluate('document.querySelector("#accountDiscovery .networkReview [name=approve_managed_network]")!==null'),true);
    assert.equal(await evaluate('document.querySelector("[name=region2]").value'), '');
    assert.equal(await evaluate('document.querySelector("[name=size2]").value'), '');
    assert.equal(await evaluate('document.querySelector("[name=image2]").value'), '');
    assert.equal(await evaluate('document.querySelector("#accountSize").selectedOptions[0].textContent.includes("GB SSD")'),false);
    await evaluate('document.querySelector("[name=region2]").value="us-east-1";document.querySelector("[name=region2]").dispatchEvent(new Event("change"))');
    assert.equal(await evaluate('document.querySelector("[name=region2]").value'),'us-east-1');
    await evaluate('document.querySelector("[name=approve_managed_network]").click()');
    assert.equal(await evaluate('document.querySelector("#modalSubmit").disabled'),false);
    await evaluate('document.querySelector("[name=approve_managed_network]").scrollIntoView({block:"center"})');
    const networkSnap=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,'aws-unified-network-mobile.png'),Buffer.from(networkSnap.data,'base64'));
   }

   for(const size of [{width:320,height:480},{width:390,height:420},{width:844,height:390},{width:390,height:844}]){
    await call('Emulation.setDeviceMetricsOverride',{...size,deviceScaleFactor:1,mobile:true});
    await evaluate('document.querySelector("#modalFields").scrollTop=0');
    await assertSaveVisible(provider+' top '+JSON.stringify(size));
    await evaluate('document.querySelector("#modalFields").scrollTop=99999');
    await assertSaveVisible(provider+' bottom '+JSON.stringify(size));
   }
   const footerSnap=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(root,provider+'-visible-save-mobile.png'),Buffer.from(footerSnap.data,'base64'));
   const point=await evaluate('(()=>{const r=document.querySelector("#modalSubmit").getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()');
   await call('Input.dispatchMouseEvent',{type:'mousePressed',...point,button:'left',clickCount:1});
   await call('Input.dispatchMouseEvent',{type:'mouseReleased',...point,button:'left',clickCount:1});

   await waitFor('!document.querySelector("#modal").open');
   await evaluate('document.querySelector("#primaryAction").click()');
   await waitFor('document.querySelector("#modal").open');
  }
  // An old preview must not repopulate credentials or Save after switching providers.
  await evaluate('document.querySelector("#modalFields [name=provider]").value="aws";document.querySelector("#modalFields [name=provider]").dispatchEvent(new Event("change"));document.querySelector("#discoverAccount").click();document.querySelector("#modalFields [name=provider]").value="linode";document.querySelector("#modalFields [name=provider]").dispatchEvent(new Event("change"))');
  await new Promise(r=>setTimeout(r,400));
  assert.equal(await evaluate('cache.accountPreview===null&&document.querySelector("#providerDiscoveryChoices").textContent===""'),true);
  await evaluate('document.querySelector("#modalFields [name=provider]").value="linode";document.querySelector("#modalFields [name=provider]").dispatchEvent(new Event("change"))');
  assert.equal(await evaluate('document.querySelector("#modalFields [name=token]").value'),'');
  assert.equal(await evaluate('document.querySelectorAll("[data-provider-credential]").length'),0);
  assert.equal(await evaluate('document.querySelector("#modalFields [name=token]").type'),'password');
  assert.equal(await evaluate('sessionStorage.getItem("provider-authorization:ovh:v1")'),null);
  console.log('NEW_CLOUD_DRIVER_CREDENTIAL_FIELDS_ISOLATION_MOBILE_AND_OVH_AUTOMATIC_AUTHORIZATION PASS popup=' + (process.env.DOB_TEST_AUTH_POPUP_BLOCKED==='1'?'blocked':'available') + '');
 }finally{
  if(ws)ws.close();chrome.kill('SIGTERM');fs.closeSync(log);
  await new Promise(r=>setTimeout(r,500));fs.rmSync(profile,{recursive:true,force:true});
 }
})().catch(e=>{console.error(e.stack);process.exitCode=1});
