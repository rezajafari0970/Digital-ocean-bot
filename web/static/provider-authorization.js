/* OVH authorization owns only temporary connection state; credentials stay server-side. */
(() => {
 'use strict';
 // Keep this boundary aligned with the driver's region-specific allowlist.
 function validApprovalURL(raw){
  if(typeof raw!=='string'||raw.length>4096)return false;
  const routes={
   'eu.api.ovh.com':['/auth','/auth/'],'api.ovh.com':['/auth','/auth/'],
   'api.eu.ovhcloud.com':['/auth','/auth/'],'www.ovh.com':['/auth/sso/api'],
   'ca.api.ovh.com':['/auth','/auth/'],'api.ca.ovhcloud.com':['/auth','/auth/'],
   'api.us.ovhcloud.com':['/auth','/auth/'],
   'auth.eu.ovhcloud.com':['/sso/api','/api/credential','/api/credential/'],
   'auth.ca.ovhcloud.com':['/sso/api','/api/credential','/api/credential/'],
   'auth.us.ovhcloud.com':['/sso/api','/api/credential','/api/credential/']
  };
  try{
   const u=new URL(raw);
   return u.protocol==='https:'&&!u.username&&!u.password&&!u.port&&!u.hash&&
    routes[u.hostname]?.includes(u.pathname)===true&&
    /^\?credentialToken=[A-Za-z0-9_-]{8,512}$/.test(u.search)&&
    raw==='https://'+u.hostname+u.pathname+u.search;
  }catch{return false}
 }
 if(typeof module==='object'&&module.exports){module.exports={validApprovalURL};return}
 const storageKey='provider-authorization:ovh:v1';
 const terminal=new Set(['FAILED','UNKNOWN','EXPIRED','CANCELLED','USED']);
 function uuid(){const b=crypto.getRandomValues(new Uint8Array(16));b[6]=(b[6]&15)|64;b[8]=(b[8]&63)|128;const h=Array.from(b,x=>x.toString(16).padStart(2,'0')).join('');return h.slice(0,8)+'-'+h.slice(8,12)+'-'+h.slice(12,16)+'-'+h.slice(16,20)+'-'+h.slice(20)}
 window.ProviderAuthorization={
  mount({form,meta,api,onReady,onInvalidate}) {
   if(!meta?.automatic_authorization)return null;
   const box=form.querySelector('#providerCredentialFields'),raw=form.querySelector('[name=token]'),ck=box.querySelector('[data-provider-credential=consumer_key]');
   const credentialHeading=raw.closest('label');
   const ui=document.createElement('section');ui.id='providerAuthorization';ui.innerHTML='<label class="checkRow"><input type="checkbox" data-auth-manual>Use an existing authorized Consumer key</label><p class="muted" data-auth-help>Enter your two application keys. OVH opens for one-time approval; projects and server options then load automatically.</p><p role="status" aria-live="polite" data-auth-status></p><a class="providerAuthAction" data-auth-link target="_blank" rel="noopener noreferrer" referrerpolicy="no-referrer" hidden>Open OVH and approve access</a><button type="button" class="ghost" data-auth-reset hidden>Cancel this connection request</button>';
   box.before(ui);
   const manual=ui.querySelector('[data-auth-manual]'),status=ui.querySelector('[data-auth-status]'),link=ui.querySelector('[data-auth-link]'),reset=ui.querySelector('[data-auth-reset]'),button=form.querySelector('#discoverAccount');
   const originalButtonLabel=button.textContent;
   const countryFields=ProxyCountry.names.map(n=>form.querySelector('[name='+n+']'));
 const countryValue=()=>ProxyCountry.read(form);
 const proxyValue=()=>proxies.map(x=>x.value).filter((v,i,a)=>v&&a.indexOf(v)===i);
   const proxies=['proxy_id','proxy_id2','proxy_id3'].map(n=>form.querySelector('[name='+n+']'));
   let id='',state='',timer=0,closed=false,busy=false,readyNotified=false,route='',routeCountries=['AUTO'],routeProxies=[],approvalWindow=null;
   function remember(value){try{value?sessionStorage.setItem(storageKey,value):sessionStorage.removeItem(storageKey)}catch{}}
   function lock(active){manual.disabled=active;box.querySelectorAll('input').forEach(x=>{x.readOnly=active;if(active)x.required=false;else{x.required=!(meta.credential_fields.find(f=>f.key===x.dataset.providerCredential)?.optional)}});[...proxies,...countryFields].filter(Boolean).forEach(x=>{x.inert=active;x.setAttribute('aria-disabled',String(active))});mode()}
   function mode(){const auto=!manual.checked,active=auto&&!!id;ck.closest('label').hidden=auto;ck.required=!auto&&!id;box.hidden=active;credentialHeading.hidden=active;manual.closest('label').hidden=active;ui.querySelector('[data-auth-help]').hidden=!auto||active;button.hidden=active&&['DISPATCHED','PENDING'].includes(state);button.textContent=auto&&!id?'Connect automatically to OVH':'Reload provider options'}
   function reserveApprovalWindow(){
    try{approvalWindow=window.open('about:blank','_blank');if(!approvalWindow)return;approvalWindow.opener=null;const d=approvalWindow.document;d.title='Connecting to OVH';const m=d.createElement('meta');m.name='referrer';m.content='no-referrer';d.head.append(m);d.body.textContent='Preparing your OVH approval page. Keep the panel open; it will continue automatically after approval.'}catch{closeBlankApprovalWindow()}
   }
   function closeBlankApprovalWindow(){try{if(approvalWindow&&!approvalWindow.closed&&approvalWindow.location.href==='about:blank')approvalWindow.close()}catch{}approvalWindow=null}
   function openPreparedApproval(url){
    if(!approvalWindow)return;
    try{if(!approvalWindow.closed&&approvalWindow.location.href==='about:blank')approvalWindow.location.replace(url)}catch{}finally{approvalWindow=null}
   }
   function resume(){if(!closed&&id&&!document.hidden&&!terminal.has(state)&&!['READY','VERIFIED'].includes(state))poll()}
   window.addEventListener('focus',resume);window.addEventListener('online',resume);document.addEventListener('visibilitychange',resume);
   function stop(){clearTimeout(timer);timer=0}
   function schedule(){stop();if(!closed&&!terminal.has(state)&&!['READY','VERIFIED'].includes(state))timer=setTimeout(poll,5000)}
   function clearSecrets(){box.querySelectorAll('input[data-provider-credential]').forEach(x=>x.value='');raw.value=''}
   function paint(x){
    if(closed||x.id!==id)return;
    state=x.state;if(['PENDING','READY','VERIFIED'].includes(state))clearSecrets();route=x.proxy_id||'';routeCountries=x.proxy_countries||[x.proxy_country||'AUTO'];routeProxies=x.proxy_ids||[route].filter(Boolean);ProxyCountry.set(form,routeCountries);proxies.forEach((e,i)=>e.value=routeProxies[i]||'');link.hidden=true;link.removeAttribute('href');reset.hidden=false;mode();
    const labels={DISPATCHED:'Step 1 of 3: preparing secure OVH access…',PENDING:'Step 2 of 3: approve access once on OVH, then return here. The next steps continue automatically.',READY:'Step 3 of 3: access approved. Loading projects, capacity and server options…',VERIFIED:'OVH access is verified. Checking current projects and server options…',FAILED:'OVH connection failed. '+(x.diagnostic||'Check the application keys and permissions.'),UNKNOWN:'OVH may have received the request. It will not be sent again automatically. Check your OVH API credentials before starting a new request.',EXPIRED:'This connection request expired. Start a new request.',CANCELLED:'Connection request cancelled.',USED:'This connection has already been saved as an account.'};
    status.textContent=labels[state]||'Checking connection…';if(['FAILED','UNKNOWN'].includes(state)){if(x.detail)status.textContent+=' '+x.detail;status.textContent+=' Reference: '+id+(x.diagnostic?' · '+x.diagnostic:'')}reset.textContent=terminal.has(state)?'Clear this connection request':'Cancel this connection request';
    if(state==='PENDING'&&validApprovalURL(x.validation_url)){link.href=x.validation_url;link.hidden=false;openPreparedApproval(x.validation_url)}
    if(terminal.has(state)||['READY','VERIFIED'].includes(state))closeBlankApprovalWindow();
    if(['READY','VERIFIED'].includes(state)&&!readyNotified){readyNotified=true;queueMicrotask(()=>{if(!closed)onReady()})}
    schedule();
   }
   async function poll(){
    if(closed||!id||busy)return;busy=true;
    try{const x=await api('/api/v1/provider-authorizations/'+id);if(closed)return;
     if(!Array.from(proxies[0].options).some(o=>o.value===(x.proxy_id||''))){status.textContent='The original proxy is unavailable. Restore it or cancel this request.';stop();return}
     proxies[0].value=x.proxy_id||'';paint(x);
    }catch(e){if(!closed){status.textContent='Unable to read connection status. The existing request is preserved. '+e.message;schedule()}}
    finally{busy=false}
   }
   manual.onchange=()=>{ck.value='';ck.dispatchEvent(new Event('input',{bubbles:true}));mode();onInvalidate()};
   reset.onclick=async()=>{
    if(busy)return;busy=true;reset.disabled=true;
    try{if(id&&!terminal.has(state))await api('/api/v1/provider-authorizations/'+id,{method:'DELETE'});if(closed)return;stop();remember('');closeBlankApprovalWindow();id='';state='';readyNotified=false;clearSecrets();lock(false);status.textContent='';link.hidden=true;link.removeAttribute('href');reset.hidden=true;onInvalidate()}
    catch(e){status.textContent=e.message}
    finally{busy=false;reset.disabled=false}
   };
   mode();
   try{id=sessionStorage.getItem(storageKey)||''}catch{}
   if(id&&!/^[0-9a-f-]{36}$/.test(id)){remember('');id=''}
   if(id){state='DISPATCHED';lock(true);status.textContent='Resuming the previous connection request…';queueMicrotask(poll)}
   return {
    async beforePreview(){
     if(manual.checked)return {token:raw.value};
     if(id){if(!['READY','VERIFIED'].includes(state)){await poll();return null}if(proxies[0].value!==route||JSON.stringify(countryValue())!==JSON.stringify(routeCountries)||JSON.stringify(proxyValue())!==JSON.stringify(routeProxies))throw Error('Connection changed. Cancel this request before changing proxy.');return {authorization_id:id,token:''}}
     if(busy)return null;
     const values=JSON.parse(raw.value||'{}');
     if(!values.application_key||!values.application_secret)throw Error('Enter Application key and Application secret.');
     if(!proxies[0].value&&proxies.slice(1).some(x=>x.value))throw Error('Select the primary proxy first, or clear all proxies for a direct connection.');
     reserveApprovalWindow();delete values.consumer_key;id=uuid();state='DISPATCHED';route=proxies[0].value;routeCountries=countryValue();routeProxies=proxyValue();remember(id);lock(true);busy=true;status.textContent='Step 1 of 3: preparing secure OVH access…';
     try{const x=await api('/api/v1/providers/'+meta.name+'/authorizations',{method:'POST',body:JSON.stringify({id,token:JSON.stringify(values),proxy_id:route,proxy_countries:routeCountries,proxy_ids:routeProxies})});if(!closed){clearSecrets();paint(x)}}
     catch(e){if(!closed){status.textContent='Checking the existing request after an interrupted response. '+e.message;schedule()}}
     finally{busy=false}
     return null;
    },
    selectionRequired(){if(id)status.textContent='Step 3 of 3: choose the Public Cloud project. Its server options will load automatically.'},
    previewLoading(){if(id&&['READY','VERIFIED'].includes(state))status.textContent='OVH access is approved. Checking current server options…'},
    previewComplete(ready){if(id)status.textContent=ready?'Connected to OVH. Server options are ready; review and save your account.':'OVH access is approved. No compatible server options are available in this region; choose another region or reload provider options.'},
    previewFailed(){if(id&&['READY','VERIFIED'].includes(state))status.textContent='OVH access is approved. Provider options could not be loaded; use Reload provider options to retry.'},
    credential(){if(manual.checked)return null;if(!id||!['READY','VERIFIED'].includes(state))throw Error('Complete OVH authorization first.');if(proxies[0].value!==route||JSON.stringify(countryValue())!==JSON.stringify(routeCountries)||JSON.stringify(proxyValue())!==JSON.stringify(routeProxies))throw Error('Connection changed. Reconnect before saving.');return {authorization_id:id,token:''}},
    complete(){remember('');this.dispose()},
    dispose(){closed=true;stop();closeBlankApprovalWindow();window.removeEventListener('focus',resume);window.removeEventListener('online',resume);document.removeEventListener('visibilitychange',resume);box.hidden=false;credentialHeading.hidden=false;button.hidden=false;button.textContent=originalButtonLabel;[...proxies,...countryFields].filter(Boolean).forEach(x=>{x.inert=false;x.removeAttribute('aria-disabled')});ui.remove()}
   };
  }
 };
})();
