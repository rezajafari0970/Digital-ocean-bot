/* A disposable remote browser. No provider document, URL or cookie is embedded. */
(() => {
 'use strict';
 let active=null;
 async function open({api,provider,accountID='',authorizationID='',proxyIDs=[],countries=['AUTO'],countryOptions,proxies}){
  if(active){active.focus();return}
  const dialog=document.createElement('dialog');dialog.className='providerBrowser';dialog.setAttribute('aria-label','Isolated provider browser');active=dialog;
  dialog.innerHTML='<header><h2>Provider browser</h2><button type="button" data-close aria-label="Close isolated browser">×</button></header><p data-status role="status">Loading available proxies…</p><form data-setup></form><section data-browser hidden><p data-route></p><div class="browserToolbar"><button type="button" data-nav="back" aria-label="Back">←</button><button type="button" data-nav="forward" aria-label="Forward">→</button><button type="button" data-nav="reload">Reload</button><button type="button" data-nav="newtab">New tab</button><select data-tabs aria-label="Browser tabs"></select></div><form data-address><input name="url" type="url" aria-label="Website address" required autocomplete="off" spellcheck="false"><button>Go</button></form><canvas tabindex="0" aria-label="Remote website; tap to interact"></canvas><form data-type><input name="text" type="password" aria-label="Text to type in the remote browser" autocomplete="off" placeholder="Type or paste text for the selected field"><button>Type text</button></form><div class="browserToolbar"><button type="button" data-key="Tab">Tab</button><button type="button" data-key="Enter">Enter</button><button type="button" data-key="Backspace">⌫</button><button type="button" data-key="SelectAll">Select all</button><button type="button" data-scroll="-400">Scroll up</button><button type="button" data-scroll="400">Scroll down</button></div></section><p class="muted">Cookies and site storage belong only to this session and are deleted when it closes. Session limit: 15 minutes; idle limit: 2 minutes. File uploads and downloads are unavailable.</p>';
  document.body.append(dialog);dialog.showModal();
  const q=s=>dialog.querySelector(s),status=q('[data-status]'),setup=q('[data-setup]'),canvas=q('canvas');
  let id='',closed=false,timer=0,queue=Promise.resolve(),pending=0,screen={width:0,height:0},imageURL='';
  const controller=new AbortController();
  async function close(){if(closed)return;closed=true;window.removeEventListener('pagehide',close);clearTimeout(timer);controller.abort();active=null;if(imageURL)URL.revokeObjectURL(imageURL);dialog.close();dialog.remove();if(id)try{await api('/api/v1/provider-browser/sessions/'+id,{method:'DELETE',keepalive:true})}catch{}}
  q('[data-close]').onclick=close;dialog.addEventListener('cancel',e=>{e.preventDefault();close()});
  window.addEventListener('pagehide',close,{once:true});
  function fail(error){if(!closed)status.textContent=error.message||'The isolated browser is unavailable.'}
  async function paint(frame){
   if(closed)return;if(frame.isolated!==true)throw Error('Browser isolation was not verified.');
   screen={width:frame.width,height:frame.height};canvas.width=screen.width;canvas.height=screen.height;
   const image=new Image();image.src='data:image/jpeg;base64,'+frame.image;await image.decode();if(closed)return;
   canvas.getContext('2d').drawImage(image,0,0);const url=q('[data-address] input');if(document.activeElement!==url)url.value=frame.url==='about:blank'?'':frame.url;
   const tabs=q('[data-tabs]');tabs.replaceChildren(...(frame.tabs||[]).map(t=>{const o=new Option(t.title||t.url||'New tab',t.id);o.selected=t.id===frame.target;return o}));
   status.textContent=frame.title||'Browser ready';
  }
  function action(payload){
   if(closed||!id)return Promise.resolve();pending++;
   const next=queue.then(async()=>{if(closed)return;const frame=await api('/api/v1/provider-browser/sessions/'+id+'/action',{method:'POST',signal:controller.signal,body:JSON.stringify(payload)});await paint(frame)});
   queue=next.catch(fail).finally(()=>pending--);return next;
  }
  function poll(){clearTimeout(timer);if(closed)return;timer=setTimeout(async()=>{if(!pending&&!document.hidden)try{await action({kind:'frame'})}catch(e){fail(e);const ended=id;id='';q('[data-browser]').hidden=true;setup.hidden=false;setup.querySelector('button').disabled=false;try{await api('/api/v1/provider-browser/sessions/'+ended,{method:'DELETE'})}catch{}return}poll()},1500)}
  q('[data-address]').onsubmit=e=>{e.preventDefault();action({kind:'navigate',url:new FormData(e.target).get('url')}).catch(fail)};
  q('[data-type]').onsubmit=async e=>{e.preventDefault();const field=e.target.elements.text,value=field.value;if(!value)return;field.value='';try{await action({kind:'text',text:value})}catch(e){fail(e)}};
  q('[data-tabs]').onchange=e=>action({kind:'tab',target:e.target.value}).catch(fail);
  dialog.querySelectorAll('[data-nav]').forEach(b=>b.onclick=()=>action({kind:b.dataset.nav}).catch(fail));
  dialog.querySelectorAll('[data-key]').forEach(b=>b.onclick=()=>action({kind:'key',key:b.dataset.key}).catch(fail));
  dialog.querySelectorAll('[data-scroll]').forEach(b=>b.onclick=()=>action({kind:'scroll',x:screen.width/2,y:screen.height/2,delta:Number(b.dataset.scroll)}).catch(fail));
  const pos=e=>{const r=canvas.getBoundingClientRect();return {x:Math.max(0,Math.min(screen.width-1,(e.clientX-r.left)*screen.width/r.width)),y:Math.max(0,Math.min(screen.height-1,(e.clientY-r.top)*screen.height/r.height))}};
  canvas.onclick=e=>{canvas.focus();action({kind:'tap',...pos(e)}).catch(fail)};
  canvas.addEventListener('wheel',e=>{e.preventDefault();if(pending<2)action({kind:'scroll',...pos(e),delta:Math.max(-1000,Math.min(1000,e.deltaY))}).catch(fail)},{passive:false});
  canvas.onkeydown=e=>{const supported=['Enter','Tab','Backspace','Delete','Escape','ArrowUp','ArrowDown','ArrowLeft','ArrowRight','Home','End'];if(supported.includes(e.key)){e.preventDefault();action({kind:'key',key:e.key}).catch(fail)}else if((e.ctrlKey||e.metaKey)&&e.key.toLowerCase()==='a'){e.preventDefault();action({kind:'key',key:'SelectAll'}).catch(fail)}else if(e.key.length===1&&!e.ctrlKey&&!e.metaKey&&!e.altKey){e.preventDefault();action({kind:'text',text:e.key}).catch(fail)}};
  try{
   if(!countryOptions||!proxies){const [p,c]=await Promise.all([api('/api/v1/proxies'),api('/api/v1/proxy-drivers')]);proxies=p;countryOptions=c.country_options||[]}
   if(closed)return;
   const providerLabel=document.createElement('p');providerLabel.textContent='Open '+provider+' through an isolated proxy session.';setup.append(providerLabel);
   for(let i=0;i<3;i++){
    const label=document.createElement('label');label.textContent='Proxy #'+(i+1)+(i?' (fallback)':' (required)');const select=document.createElement('select');select.name='browser_proxy'+i;select.append(new Option(i?'None':'Select proxy',''));
    for(const p of proxies.filter(x=>x.status==='healthy'))select.append(new Option(p.name,p.id));select.value=proxyIDs[i]||'';label.append(select);setup.append(label);
   }
   const fields=document.createElement('div');fields.innerHTML=ProxyCountry.field(countryOptions,countries);setup.append(fields);
   if(authorizationID)setup.querySelectorAll('select').forEach(x=>{x.inert=true;x.setAttribute('aria-disabled','true')});
   const start=document.createElement('button');start.textContent='Open provider website with proxy';setup.append(start);status.textContent='Choose the proxy and country priorities.';
   setup.onsubmit=async e=>{
    e.preventDefault();if(start.disabled)return;
    try{
     const ids=[0,1,2].map(i=>setup.elements['browser_proxy'+i].value),policy=ProxyCountry.read(setup);ProxyCountry.validate(policy,ids[0],ids.slice(1));if(!ids[0])throw Error('Select a proxy. This browser never uses a direct connection.');
     start.disabled=true;status.textContent='Verifying proxy country and starting an isolated browser…';
     const data=await api('/api/v1/provider-browser/sessions',{method:'POST',signal:controller.signal,body:JSON.stringify({provider,account_id:accountID,authorization_id:authorizationID,proxy_id:ids[0],proxy_ids:[...new Set(ids.filter(Boolean))],proxy_countries:policy,width:Math.max(360,Math.min(1200,window.innerWidth-44)),height:800})});
     id=data.id;if(closed){await api('/api/v1/provider-browser/sessions/'+id,{method:'DELETE'});return}
     q('[data-route]').textContent='Verified country: '+data.country+' · Exit IP: '+data.exit_ip+' · Isolated session';setup.hidden=true;q('[data-browser]').hidden=false;await paint(data.frame);poll();
    }catch(e){fail(e);start.disabled=false}
   };
  }catch(e){fail(e)}
 }
 window.ProviderBrowser={open};
})();
