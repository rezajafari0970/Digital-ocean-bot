/* OVH authorization owns only temporary connection state; credentials stay server-side. */
(() => {
 'use strict';
 const storageKey='provider-authorization:ovh:v1';
 const terminal=new Set(['FAILED','UNKNOWN','EXPIRED','CANCELLED','USED']);
 function uuid(){const b=crypto.getRandomValues(new Uint8Array(16));b[6]=(b[6]&15)|64;b[8]=(b[8]&63)|128;const h=Array.from(b,x=>x.toString(16).padStart(2,'0')).join('');return h.slice(0,8)+'-'+h.slice(8,12)+'-'+h.slice(12,16)+'-'+h.slice(16,20)+'-'+h.slice(20)}
 window.ProviderAuthorization={
  mount({form,meta,api,onReady,onInvalidate}) {
   if(!meta?.automatic_authorization)return null;
   const box=form.querySelector('#providerCredentialFields'),raw=form.querySelector('[name=token]'),ck=box.querySelector('[data-provider-credential=consumer_key]');
   const ui=document.createElement('section');ui.id='providerAuthorization';ui.innerHTML='<label class="checkRow"><input type="checkbox" data-auth-manual>Enter an existing Consumer key manually</label><p class="muted" data-auth-help>Enter Application key and Application secret once. The panel requests the required permissions and securely stores the Consumer key. Approve access on the official OVH page; project discovery then continues automatically.</p><p role="status" aria-live="polite" data-auth-status></p><a data-auth-link target="_blank" rel="noopener noreferrer" referrerpolicy="no-referrer" hidden>Approve access on OVH</a><button type="button" class="ghost" data-auth-reset hidden>Cancel this connection request</button>';
   box.before(ui);
   const manual=ui.querySelector('[data-auth-manual]'),status=ui.querySelector('[data-auth-status]'),link=ui.querySelector('[data-auth-link]'),reset=ui.querySelector('[data-auth-reset]'),button=form.querySelector('#discoverAccount');
   const proxies=['proxy_id','proxy_id2','proxy_id3'].map(n=>form.querySelector('[name='+n+']'));
   let id='',state='',timer=0,closed=false,busy=false,readyNotified=false,route='';
   function remember(value){try{value?sessionStorage.setItem(storageKey,value):sessionStorage.removeItem(storageKey)}catch{}}
   function lock(active){manual.disabled=active;box.querySelectorAll('input').forEach(x=>{x.readOnly=active;if(active)x.required=false;else{x.required=!(meta.credential_fields.find(f=>f.key===x.dataset.providerCredential)?.optional)}});proxies.forEach(x=>{x.inert=active;x.setAttribute('aria-disabled',String(active))});mode()}
   function mode(){const auto=!manual.checked;ck.closest('label').hidden=auto;ck.required=!auto&&!id;ui.querySelector('[data-auth-help]').hidden=!auto;button.textContent=auto&&!id?'Connect automatically to OVH':'Validate credential & load provider options'}
   function stop(){clearTimeout(timer);timer=0}
   function schedule(){stop();if(!closed&&!terminal.has(state)&&!['READY','VERIFIED'].includes(state))timer=setTimeout(poll,5000)}
   function clearSecrets(){box.querySelectorAll('input[data-provider-credential]').forEach(x=>x.value='');raw.value=''}
   function paint(x){
    if(closed||x.id!==id)return;
    state=x.state;if(['PENDING','READY','VERIFIED'].includes(state))clearSecrets();route=x.proxy_id||'';link.hidden=true;link.removeAttribute('href');reset.hidden=false;
    const labels={DISPATCHED:'Request submitted. Checking its outcome…',PENDING:'Waiting for your approval on OVH. This page checks automatically.',READY:'Access approved. Loading projects and provider options…',VERIFIED:'Access verified. Review the account settings and save.',FAILED:'OVH connection failed. '+(x.diagnostic||'Check the application keys and permissions.'),UNKNOWN:'OVH may have received the request. It will not be sent again automatically. Check your OVH API credentials before starting a new request.',EXPIRED:'This connection request expired. Start a new request.',CANCELLED:'Connection request cancelled.',USED:'This connection has already been saved as an account.'};
    status.textContent=labels[state]||'Checking connection…';reset.textContent=terminal.has(state)?'Clear this connection request':'Cancel this connection request';
    if(state==='PENDING'&&x.validation_url){const u=new URL(x.validation_url);if(u.protocol==='https:'&&/^(?:(?:eu|ca)\.api\.ovh\.com|api\.(?:eu|ca|us)\.ovhcloud\.com|auth\.(?:eu|ca|us)\.ovhcloud\.com)$/.test(u.hostname)){link.href=u.href;link.hidden=false}}
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
    try{if(id&&!terminal.has(state))await api('/api/v1/provider-authorizations/'+id,{method:'DELETE'});if(closed)return;stop();remember('');id='';state='';readyNotified=false;clearSecrets();lock(false);status.textContent='';link.hidden=true;link.removeAttribute('href');reset.hidden=true;onInvalidate()}
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
     if(id){if(!['READY','VERIFIED'].includes(state)){await poll();return null}if(proxies[0].value!==route)throw Error('Connection changed. Cancel this request before changing proxy.');return {authorization_id:id,token:''}}
     if(busy)return null;
     const values=JSON.parse(raw.value||'{}');
     if(!values.application_key||!values.application_secret)throw Error('Enter Application key and Application secret.');
     if(!proxies[0].value&&proxies.slice(1).some(x=>x.value))throw Error('Select the primary proxy first, or clear all proxies for a direct connection.');
     delete values.consumer_key;id=uuid();state='DISPATCHED';route=proxies[0].value;remember(id);lock(true);busy=true;status.textContent='Requesting OVH access…';
     try{const x=await api('/api/v1/providers/'+meta.name+'/authorizations',{method:'POST',body:JSON.stringify({id,token:JSON.stringify(values),proxy_id:route})});if(!closed){clearSecrets();paint(x)}}
     catch(e){if(!closed){status.textContent='Checking the existing request after an interrupted response. '+e.message;schedule()}}
     finally{busy=false}
     return null;
    },
    credential(){if(manual.checked)return null;if(!id||!['READY','VERIFIED'].includes(state))throw Error('Complete OVH authorization first.');if(proxies[0].value!==route)throw Error('Connection changed. Reconnect before saving.');return {authorization_id:id,token:''}},
    complete(){remember('');this.dispose()},
    dispose(){closed=true;stop();proxies.forEach(x=>{x.inert=false;x.removeAttribute('aria-disabled')});ui.remove()}
   };
  }
 };
})();
