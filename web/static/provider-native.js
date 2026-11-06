(() => {
 'use strict';
 const q=s=>document.querySelector(s),form=q('[data-setup]'),token=sessionStorage.getItem('providerBrowser.token');
 let launch,session=null,config='',timer=0,busy=false,statusError=false,proxies=[];
 try{launch=JSON.parse(sessionStorage.getItem('providerBrowser.launch')||'null')}catch{}
 window.opener=null;
 function message(s){q('[data-status]').textContent=s||''}
 const reasons={expired:'The connection expired. Create a new profile to continue.',signed_out:'Panel sign-in ended. Sign in again.',ended_by_user:'The proxy connection has ended.',proxy_identity_unverified:'The selected proxy IP or country could no longer be verified. Traffic was stopped.',account_changed:'The account changed or entered cleanup. Traffic was stopped.',authorization_ended:'Provider authorization ended.',authorization_unavailable:'Panel authorization could not be verified. Traffic was stopped.'};
 async function api(path,options={}){
  const abort=new AbortController(),t=setTimeout(()=>abort.abort(),options.method==='POST'?110000:20000);
  try{const r=await fetch('/api/v1/provider-browser/native-sessions'+path,{...options,headers:{Authorization:'Bearer '+token,'Content-Type':'application/json'},cache:'no-store',signal:abort.signal});const value=r.status===204?null:await r.json();if(!r.ok)throw Error(value?.detail||value?.error||'Connection unavailable.');return value}finally{clearTimeout(t)}
 }
 function forget(){sessionStorage.removeItem('providerBrowser.native');session=null;config='';statusError=false;clearTimeout(timer);q('[data-session]').hidden=true;form.hidden=false}
 function show(data){
  session=data;config=JSON.stringify(ProviderNativeConfig.build(session,location.origin),null,2);
  q('[data-session]').hidden=false;form.hidden=true;
  const proxy=proxies.find(x=>x.id===session.proxy_id)?.name||'Selected proxy';
  q('[data-route]').textContent=session.country+' · '+proxy+' · '+session.exit_ip;
  q('[data-expiry]').textContent='Expires '+new Date(session.expires_at).toLocaleTimeString();
  const ready=Number(session.successful_connections)>0;
  q('[data-open]').disabled=q('[data-site-copy]').disabled=!ready;
  q('[data-client]').textContent=ready?'A client response was received. Keep V2RayNG active; this page cannot check Android’s VPN switch.':'Waiting for V2RayNG: connect and run its real connection test.';
 }
 async function poll(){
  clearTimeout(timer);if(!session)return;const current=session;
  try{
   const status=await api('/'+current.id);if(session!==current)return;
   if(status.state!=='active'){const reason=reasons[status.reason]||'The connection has ended.';forget();message(reason);return}
   show({...session,...status});if(statusError){message('');statusError=false}
  }catch(e){if(session!==current)return;statusError=true;q('[data-open]').disabled=q('[data-site-copy]').disabled=true;message('Could not verify the connection: '+e.message)}
  if(session)timer=setTimeout(poll,2500);
 }
 async function start(){
  if(busy||session)return;busy=true;form.querySelector('button').disabled=true;
  try{
   const ids=[0,1,2].map(i=>form.elements['browser_proxy'+i].value),countries=ProxyCountry.read(form);ProxyCountry.validate(countries,ids[0],ids.slice(1));if(!ids[0])throw Error('Select a proxy.');
   message('Verifying the selected proxy, country and stable exit IP…');
   const data=await api('',{method:'POST',body:JSON.stringify({provider:launch.provider,account_id:launch.accountID||'',authorization_id:launch.authorizationID||'',proxy_id:ids[0],proxy_ids:[...new Set(ids.filter(Boolean))],proxy_countries:countries})});
   sessionStorage.setItem('providerBrowser.native',JSON.stringify(data));show(data);message('Temporary connection ready. Import it into V2RayNG.');poll();
  }catch(e){message(e.message)}finally{busy=false;form.querySelector('button').disabled=false}
 }
 async function copy(text){
  if(navigator.clipboard?.writeText){try{await navigator.clipboard.writeText(text);return}catch{}}
  const t=document.createElement('textarea');t.value=text;t.style.cssText='position:fixed;top:0;left:0';document.body.append(t);t.select();const copied=document.execCommand('copy');t.remove();if(!copied)throw Error('Clipboard unavailable. Use Download JSON.');
 }
 q('[data-copy]').onclick=()=>{if(config)copy(config).then(()=>message('Copied. Import as a custom config in V2RayNG.')).catch(e=>message(e.message))};
 q('[data-download]').onclick=()=>{if(!config)return;const url=URL.createObjectURL(new Blob([config],{type:'application/json'})),a=document.createElement('a');a.href=url;a.download='browser-'+session.provider+'-'+session.country+'.json';a.click();setTimeout(()=>URL.revokeObjectURL(url),1000)};
 q('[data-open]').onclick=()=>{if(!session||q('[data-open]').disabled)return;const tab=window.open(session.website,'_blank','noopener,noreferrer');if(tab)tab.opener=null};
 q('[data-site-copy]').onclick=()=>{if(session&&!q('[data-site-copy]').disabled)copy(session.website).then(()=>message('Copied. Open it in a fresh Chrome Incognito session while the VPN is active.')).catch(e=>message(e.message))};
 q('[data-end]').onclick=async()=>{if(!session)return;try{await api('/'+session.id,{method:'DELETE'});forget();message('Connection ended. Switch V2RayNG back to your normal profile when finished.')}catch(e){message(e.message)}};
 window.addEventListener('pagehide',()=>clearTimeout(timer));window.addEventListener('pageshow',e=>{if(e.persisted&&session)poll()});
 document.addEventListener('visibilitychange',()=>{if(!document.hidden&&session)poll()});
 async function init(){
  if(!launch?.provider||!token){message('Open this page from an account in the panel.');return}
  if(location.protocol!=='https:'){message('Open the panel over HTTPS to use this encrypted connection.');return}
  if(launch.returnPath?.startsWith('/')&&!launch.returnPath.startsWith('//'))q('[data-return]').href=launch.returnPath;
  try{
   const headers={Authorization:'Bearer '+token};const values=await Promise.all(['/api/v1/proxies','/api/v1/proxy-drivers'].map(async path=>{const r=await fetch(path,{headers,cache:'no-store'});if(!r.ok)throw Error('Sign in to the panel again.');return r.json()}));proxies=values[0]||[];const options=values[1].country_options||[];
   for(const code of ProxyCountry.normalize(launch.countries||['AUTO']))if(code!=='AUTO'&&!options.some(x=>x.code===code))options.push({code,label:'Previously selected country'});
   for(let i=0;i<3;i++){const label=document.createElement('label');label.textContent='Proxy '+(i+1)+(i?' (fallback)':'');const select=document.createElement('select');select.name='browser_proxy'+i;select.append(new Option(i?'None':'Select proxy',''));for(const p of proxies.filter(x=>x.status==='healthy'))select.append(new Option(p.name,p.id));select.value=launch.proxyIDs?.[i]||'';label.append(select);form.append(label)}
   const fields=document.createElement('div');fields.innerHTML=ProxyCountry.field(options,launch.countries||['AUTO']);form.append(fields);
   if(launch.authorizationID)form.querySelectorAll('select').forEach(x=>{x.inert=true;x.setAttribute('aria-disabled','true')});
   const button=document.createElement('button');button.textContent='Create browser connection';form.append(button);form.onsubmit=e=>{e.preventDefault();start()};
   let saved;try{saved=JSON.parse(sessionStorage.getItem('providerBrowser.native')||'null')}catch{}
   if(saved&&Date.parse(saved.expires_at)>Date.now()){show(saved);await poll();return}
   forget();message('Confirm the proxy and country, then create your connection.');
  }catch(e){message(e.message)}
 }
 init();
})();
