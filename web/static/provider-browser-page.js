/* Standalone, same-origin controls for the isolated remote browser. */
(() => {
 'use strict';
 const q=s=>document.querySelector(s),canvas=q('canvas'),viewport=q('[data-viewport]'),status=q('[data-status]'),setup=q('[data-setup]'),ime=q('[data-ime]');
 const storage={read:k=>{try{return JSON.parse(sessionStorage.getItem(k)||'null')}catch{return null}},write:(k,v)=>sessionStorage.setItem(k,JSON.stringify(v))};
 const launch=storage.read('providerBrowser.launch'),credential=sessionStorage.getItem('providerBrowser.token');
 let generation=0,startAbort=null,frameID='',lastDisplay='';
 let displayMode=sessionStorage.getItem('providerBrowser.display')==='desktop'?'desktop':'auto';
 let id='',closed=false,busy=false,timer=0,resizeTimer=0,queue=Promise.resolve(),pending=0,screen={width:0,height:0},editable=[],proxies=[],options=[],pollFailures=0,gesture=null,composing=false;
 const life=new AbortController();
 window.opener=null;
 function message(text){status.textContent=text||''}
 function welcome(title,text){q('[data-welcome]').hidden=false;q('[data-heading]').textContent=title;q('[data-message]').textContent=text;canvas.hidden=true;controls(false)}
 function saveSession(data){storage.write('providerBrowser.session',data)}
 function forgetSession(){sessionStorage.removeItem('providerBrowser.session')}
 async function api(path,opt={}){
  const abort=new AbortController(),stop=()=>abort.abort();life.signal.addEventListener('abort',stop,{once:true});opt.signal?.addEventListener('abort',stop,{once:true});
  const timeout=setTimeout(stop,opt.method==='POST'&&path.endsWith('/sessions')?110000:25000);
  try{
   const r=await fetch(path,{...opt,headers:{Authorization:'Bearer '+credential,'Content-Type':'application/json'},signal:abort.signal,cache:'no-store'});
   const data=r.status===204?null:await r.json();
   if(!r.ok){const error=new Error(data?.detail||data?.error||'The browser request failed.');error.status=r.status;error.code=data?.error;throw error}
   return data;
  }catch(e){if(e.name==='AbortError'&&!closed)throw Error('The connection check timed out. Retry or choose another proxy.');throw e}
  finally{clearTimeout(timeout);life.signal.removeEventListener('abort',stop);opt.signal?.removeEventListener('abort',stop)}
 }
 function dimensions(){
  const r=viewport.getBoundingClientRect(),desktop=displayMode==='desktop';
  const width=desktop?1280:Math.max(320,Math.min(1440,Math.round(r.width)));
  const height=Math.max(240,Math.min(3200,Math.round(r.height*(desktop?width/Math.max(1,r.width):1))));
  return {width,height,scale_factor:Math.max(1,Math.min(3,window.devicePixelRatio||1)),mobile:!desktop&&r.width<800,touch:matchMedia('(pointer:coarse)').matches||navigator.maxTouchPoints>0};
 }
 function controls(ready){
  document.querySelectorAll('[data-nav],[data-key],[data-keyboard],[data-address] input,[data-address] button,[data-tabs]').forEach(e=>e.disabled=!ready);
 }
 function displayLabel(){const desktop=displayMode==='desktop';q('[data-desktop]').textContent=desktop?'Mobile':'Desktop';q('[data-desktop]').setAttribute('aria-label',desktop?'Use mobile site':'Use desktop site');q('[data-desktop]').setAttribute('aria-pressed',String(desktop))}
 q('[data-desktop]').onclick=()=>{displayMode=displayMode==='desktop'?'auto':'desktop';sessionStorage.setItem('providerBrowser.display',displayMode);displayLabel();fit()};
 controls(false);displayLabel();
 async function paint(frame,source){
  if(closed)return;
  if(frame.isolated!==true||!frame.width||!frame.height)throw Error('The proxy browser could not verify its isolation.');
  if(frame.image){
   const image=new Image();image.src='data:image/'+(frame.format==='png'?'png':'jpeg')+';base64,'+frame.image;await image.decode();if(closed)return;
   canvas.width=image.naturalWidth;canvas.height=image.naturalHeight;canvas.getContext('2d').drawImage(image,0,0);
  }else if(!frameID)throw Error('The browser did not return a screen image.');
  frameID=frame.frame_id||'';screen={width:frame.width,height:frame.height};
  canvas.dataset.viewportWidth=String(screen.width);canvas.dataset.viewportHeight=String(screen.height);
  canvas.dataset.format=frame.format||'jpeg';canvas.dataset.scale=String(frame.scale_factor||1);
  editable=Array.isArray(frame.editable)?frame.editable:[];
  const address=q('[data-address] input');if(document.activeElement!==address)address.value=frame.url==='about:blank'?'':frame.url;
  q('[data-tabs]').replaceChildren(...(frame.tabs||[]).map(t=>{const o=new Option(t.title||t.url||'New tab',t.id);o.selected=t.id===frame.target;return o}));
  q('[data-tabbar]').hidden=(frame.tabs||[]).length<=1;
  document.title=(frame.title||'Website')+' · Proxy';
  q('[data-welcome]').hidden=true;canvas.hidden=false;controls(true);message('');input.update(frame,source);
 }
 function unavailable(e){
  if(closed)return;message(e.message||'The website connection was interrupted.');
  if([401,403,404,410].includes(e.status)){
   id='';forgetSession();clearTimeout(timer);setup.hidden=false;
   welcome('Session ended',e.status===401?'Sign in to the panel and open the website again.':'The isolated session ended. Start again to verify the proxy route.');
   setup.querySelector('button')?.removeAttribute('disabled');
  }
 }
 function action(payload){
  if(closed||!id)return Promise.resolve();
  const session=id;pending++;
  const next=queue.then(async()=>{if(closed||id!==session)return;const data=typeof payload==='function'?payload():payload;const frame=await api('/api/v1/provider-browser/sessions/'+session+'/action',{method:'POST',body:JSON.stringify(data.kind==='frame'?{...data,frame_id:frameID}:data)});if(closed||id!==session)return;await paint(frame,data);return frame});
  queue=next.catch(unavailable).finally(()=>pending--);return next;
 }
 function poll(){
  clearTimeout(timer);if(closed||!id)return;
  timer=setTimeout(async()=>{if(!pending&&!document.hidden)try{await action({kind:'frame'});pollFailures=0}catch(e){pollFailures++;unavailable(e)}poll()},pollFailures?2500:300);
 }
 function fit(){clearTimeout(resizeTimer);resizeTimer=setTimeout(()=>{
  if(input.isGesturing()){fit();return}
  const size=dimensions(),key=JSON.stringify(size);
  if(id&&!closed&&key!==lastDisplay){lastDisplay=key;action({kind:'resize',...size}).catch(e=>{lastDisplay='';unavailable(e)})}
 },180)}
 async function showSessions(){
  const box=q('[data-sessions]');box.replaceChildren();box.hidden=false;
  try{
   const rows=await api('/api/v1/provider-browser/sessions');
   const intro=document.createElement('p');intro.textContent=rows.length?'Your open websites are using the available browser slots. Close one to open this website.':'Browser slots are busy. Wait briefly, then try again.';box.append(intro);
   for(const row of rows){
    const line=document.createElement('div');line.className='sessionRow';
    const label=document.createElement('span');label.textContent=row.provider+' · '+(row.country||'Verified proxy');
    const close=document.createElement('button');close.type='button';close.textContent='Close and open here';
    close.onclick=async()=>{close.disabled=true;try{await api('/api/v1/provider-browser/sessions/'+row.id,{method:'DELETE'});box.hidden=true;await start()}catch(e){message(e.message);close.disabled=false}};
    line.append(label,close);box.append(line);
   }
  }catch(e){message(e.message)}
 }
 function route(data){
  const name=proxies.find(x=>x.id===data.proxy_id)?.name||'Verified proxy';
  q('[data-route]').textContent=data.country+' · '+name;
  q('[data-connection]').textContent='Country: '+data.country+' · Proxy: '+name+' · Exit IP: '+data.exit_ip;
 }
 async function start(){
  if(busy||closed||id)return;
  const attempt=++generation;startAbort=new AbortController();
  busy=true;q('[data-retry]').hidden=true;q('[data-sessions]').hidden=true;const button=setup.querySelector('button');if(button)button.disabled=true;
  try{
   const ids=[0,1,2].map(i=>setup.elements['browser_proxy'+i].value),countries=ProxyCountry.read(setup);
   ProxyCountry.validate(countries,ids[0],ids.slice(1));if(!ids[0])throw Error('Select a proxy to open this website.');
   launch.proxyIDs=ids;launch.countries=countries;storage.write('providerBrowser.launch',launch);
   setup.hidden=true;welcome('Opening '+launch.provider,'Verifying the selected proxy and country…');message('Connecting through your proxy…');
   const display=dimensions();lastDisplay=JSON.stringify(display);
   const data=await api('/api/v1/provider-browser/sessions',{method:'POST',signal:startAbort.signal,body:JSON.stringify({provider:launch.provider,account_id:launch.accountID||'',authorization_id:launch.authorizationID||'',proxy_id:ids[0],proxy_ids:[...new Set(ids.filter(Boolean))],proxy_countries:countries,...display})});
   if(closed||attempt!==generation){try{await api('/api/v1/provider-browser/sessions/'+data.id,{method:'DELETE',keepalive:true})}catch{}return}
   id=data.id;saveSession({id,proxy_id:data.proxy_id,country:data.country,exit_ip:data.exit_ip,expires_at:data.expires_at});route(data);await paint(data.frame);fit();poll();
  }catch(e){if(!closed&&attempt===generation){welcome('Could not open the website',e.message);setup.hidden=false;q('[data-route]').textContent='Not connected';message(e.message);if(e.code==='browser_capacity_full')await showSessions()}}
  finally{busy=false;if(button)button.disabled=false}
 }
 async function end(){
  if(closed)return;
  generation++;startAbort?.abort();
  const session=id;clearTimeout(timer);
  if(session)try{await api('/api/v1/provider-browser/sessions/'+session,{method:'DELETE',keepalive:true})}catch(e){message('Could not close this session: '+e.message);poll();return}
  id='';frameID='';lastDisplay='';forgetSession();input.reset();canvas.getContext('2d').clearRect(0,0,canvas.width,canvas.height);q('[data-options]').hidden=true;
  welcome('Session closed','Cookies and website data for this session are removed.');setup.hidden=false;q('[data-route]').textContent='Disconnected';
 }
 q('[data-close]').onclick=end;
 q('[data-menu]').onclick=()=>{const open=q('[data-options]').hidden;q('[data-options]').hidden=!open;q('[data-menu]').setAttribute('aria-expanded',String(open))};
 q('[data-fullscreen]').onclick=async()=>{try{if(document.fullscreenElement)await document.exitFullscreen();else await document.documentElement.requestFullscreen();q('[data-options]').hidden=true;fit()}catch{message('Full screen is unavailable in this browser.')}};
 q('[data-address]').onsubmit=e=>{e.preventDefault();let url=e.target.elements.url.value.trim();if(!/^[a-z][a-z0-9+.-]*:/i.test(url))url='https://'+url;try{const parsed=new URL(url);if(parsed.protocol!=='https:'||parsed.username||parsed.password||(parsed.port&&parsed.port!=='443'))throw Error();action({kind:'navigate',url:parsed.href}).catch(unavailable);e.target.elements.url.blur()}catch{message('Enter a valid HTTPS website address.')}};
 q('[data-tabs]').onchange=e=>action({kind:'tab',target:e.target.value}).catch(unavailable);
 document.querySelectorAll('[data-nav]').forEach(b=>b.onclick=()=>{q('[data-options]').hidden=true;action({kind:b.dataset.nav}).then(fit).catch(unavailable)});

 q('[data-retry]').onclick=start;
 const input=ProviderInput.create({canvas,viewport,action,getScreen:()=>screen,connected:()=>Boolean(id&&!closed),message,fit});
 window.addEventListener('resize',fit);window.visualViewport?.addEventListener('resize',fit);
 document.addEventListener('visibilitychange',()=>{if(document.hidden)clearTimeout(timer);else if(id){action({kind:'frame'}).then(()=>{fit();poll()}).catch(unavailable)}});
 window.addEventListener('pagehide',()=>{closed=true;clearTimeout(timer);clearTimeout(resizeTimer);life.abort();input.destroy()});
 window.addEventListener('pageshow',e=>{if(e.persisted)location.reload()});
 async function init(){
  if(!launch?.provider||!credential){welcome('Open this page from the panel','Choose “Open provider with proxy” from an account or its setup form.');message('Panel sign-in is required.');return}
  try{
   const [p,c]=await Promise.all([api('/api/v1/proxies'),api('/api/v1/proxy-drivers')]);proxies=p;options=c.country_options||[];
   for(const code of ProxyCountry.normalize(launch.countries||['AUTO']))if(code!=='AUTO'&&!options.some(x=>x.code===code))options.push({code,label:'Previously selected country'});
   for(let i=0;i<3;i++){const label=document.createElement('label');label.textContent='Proxy '+(i+1)+(i?' (fallback)':'');const select=document.createElement('select');select.name='browser_proxy'+i;select.append(new Option(i?'None':'Select proxy',''));for(const p of proxies.filter(x=>x.status==='healthy'))select.append(new Option(p.name,p.id));select.value=launch.proxyIDs?.[i]||'';label.append(select);setup.append(label)}
   const fields=document.createElement('div');fields.innerHTML=ProxyCountry.field(options,launch.countries||['AUTO']);setup.append(fields);
   if(launch.authorizationID)setup.querySelectorAll('select').forEach(x=>{x.inert=true;x.setAttribute('aria-disabled','true')});
   const button=document.createElement('button');button.textContent='Open website';setup.append(button);setup.onsubmit=e=>{e.preventDefault();start()};
   const saved=storage.read('providerBrowser.session');
   if(saved?.id&&(!saved.expires_at||Date.parse(saved.expires_at)>Date.now())){
    id=saved.id;route(saved);try{await action({kind:'frame'});fit();poll();return}catch(e){id='';forgetSession();welcome('Session ended','Start again to verify the proxy route.');setup.hidden=false;return}
   }
   forgetSession();
   if(launch.proxyIDs?.[0]&&setup.elements.browser_proxy0.value)await start();
   else{welcome('Choose a proxy','Select the proxy and country for this website.');setup.hidden=false;message('A proxy is required.')}
  }catch(e){welcome('Browser unavailable',e.message);message(e.message);q('[data-retry]').textContent='Reload page';q('[data-retry]').onclick=()=>location.reload();q('[data-retry]').hidden=false}
 }
 init();
})();
