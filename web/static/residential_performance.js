/* Server-owned publication, future admission and rollback survive this page. */
let performanceStatus=null,performanceBusy=false;
async function performanceRequest(body){
 const signature=JSON.stringify(body);let saved;
 try{saved=JSON.parse(sessionStorage.getItem('residentialPerformanceRequest')||'null')}catch{}
 const request=saved?.signature===signature?saved.request:{...body,request_id:residentialUUIDValue()};
 sessionStorage.setItem('residentialPerformanceRequest',JSON.stringify({signature,request}));
 const result=await api('/api/v1/residential-performance',{method:'POST',body:JSON.stringify(request)});
 sessionStorage.removeItem('residentialPerformanceRequest');return result;
}
async function refreshResidentialPerformance(){
 const slot=document.querySelector('#residentialPerformance');if(!slot||performanceBusy)return;
 try{
  const d=await api('/api/v1/residential-performance');performanceStatus=d;const e=d.experiment,active=e&&['RUNNING','KEPT','ROLLING_BACK'].includes(e.state);
  const names=new Map(d.panels.map(p=>[p.id,p.label])),targets=e?.targets||[],assigned=new Set(targets.map(t=>t.panel_id));
  const waiting=e?.inherits_future?d.panels.filter(p=>p.experiment_id!==e.id).length:0;
  const verified=e?.counts?.verified??targets.filter(t=>t.state==='APPLIED'&&t.verified).length,total=e?.counts?.total??targets.length;
  const title=e?.state==='KEPT'?(e.inherits_future?'PUBLISHED · All current and future servers':'PERMANENT · Selected servers'):e?.state;
  slot.innerHTML='<h3>Performance & rollback</h3><p class="muted">Publish shared server settings for existing and future users. Choose selected servers or all current and future servers. Permanent publication has no automatic rollback timer.</p>'+
   (d.routing_enabled?'':'<p class="error">Routing is paused. Publication and rollback stay queued until routing execution resumes.</p>')+
   '<div class="cardActions"><button data-performance="start" '+(active?'disabled':'')+'>Publish settings</button>'+
   (e?.state==='RUNNING'?'<button data-performance="keep" '+(e.can_keep?'':'disabled')+'>Keep verified settings</button><button class="ghost" data-performance="promote" '+(e.can_keep?'':'disabled')+'>Add test servers</button>':'')+
   (e&&['RUNNING','KEPT'].includes(e.state)&&!e.inherits_future?'<button data-performance="publish">Publish to all current & future servers</button>':'')+
   (e&&['RUNNING','KEPT','ROLLING_BACK'].includes(e.state)?'<button class="danger" data-performance="rollback" '+(e.state==='ROLLING_BACK'?'disabled':'')+'>'+(e.state==='ROLLING_BACK'?'Rollback pending…':'Rollback all settings')+'</button>':'')+'</div>'+
   (e?'<p role="status"><strong>'+esc(title)+'</strong> · '+(e.state==='RUNNING'?'Auto rollback: '+esc(new Date(e.deadline).toLocaleString()):e.state==='KEPT'?'No expiry · Manual rollback available':esc(e.reason||''))+'</p>'+
    '<p class="muted">'+esc(e.config.fast_share)+'% fast group · '+esc(e.config.fast_count||'auto')+' paths · '+esc(e.config.dns_mode.toUpperCase())+' DNS'+(e.state==='KEPT'||e.state==='RUNNING'?' · '+verified+' freshly verified / '+total+' assigned'+(e?.counts?.retired?' · '+e.counts.retired+' retired':'')+(waiting?' · '+waiting+' awaiting assignment':''):'')+'</p>'+
    (e.inherits_future?'<p>New servers inherit this profile automatically. Rollback stops inheritance and restores every server assigned to this publication.</p>':'')+
    '<details><summary>Server results ('+targets.length+' shown / '+total+')</summary><div class="tableWrap"><table class="table"><thead><tr><th>Server</th><th>Result</th><th>Detail</th></tr></thead><tbody>'+targets.map(t=>'<tr><td>'+esc(names.get(t.panel_id)||t.panel_id)+'</td><td>'+esc(t.state==='APPLIED'&&!t.verified?'VERIFYING':t.state)+'</td><td>'+esc(t.error||'')+'</td></tr>').join('')+'</tbody></table></div></details>':'<p class="muted">No performance profile has been published.</p>')+
   '<p class="muted">Unavailable servers remain pending. Rollback preserves current proxies and client identities. Roll back the current profile before publishing different settings.</p>';
 }catch(e){slot.textContent='Performance status unavailable: '+e.message}
}
function performanceNumber(name,label,value,min,max,step=1){return '<label>'+label+'<input name="'+name+'" type="number" min="'+min+'" max="'+max+'" step="'+step+'" value="'+value+'" required></label>'}
function performanceCheck(name,label,value){return '<label class="remember"><input type="checkbox" name="'+name+'" '+(value?'checked':'')+'><span>'+label+'</span></label>'}
function performanceServerRows(panels,name='panel'){return panels.map(p=>'<label class="remember performanceServer"><input type="checkbox" name="'+name+'" value="'+esc(p.id)+'"><span>'+esc(p.label)+' <small class="muted">'+esc(p.state||'Waiting for setup')+'</small></span></label>').join('')}
async function openPerformanceTest(){
 await refreshResidentialPerformance();const d=performanceStatus;if(!d)return;
 const c=d.defaults,panels=d.panels;
 const form=residentialDialog('Publish performance settings',
 '<p class="muted">Applies to existing and future users on the selected servers. Existing Ads-only routing, direct users and UDP are preserved. Unavailable servers wait for application and verification.</p>'+
 '<label>Duration<select name="mode"><option value="permanent" selected>Permanent — no automatic rollback</option><option value="timed">Timed test — automatic rollback</option></select></label>'+
 '<div data-timer hidden>'+performanceNumber('minutes','Automatic rollback after (minutes)',15,5,60)+'</div>'+
 performanceCheck('future','All current and future servers',true)+
 '<div class="cardActions"><button type="button" class="ghost" data-select-all>Select all</button><button type="button" class="ghost" data-clear-selection>Clear selection</button></div>'+
 '<p data-selection-count role="status"></p><details class="performanceServerList"><summary>Choose servers</summary>'+performanceServerRows(panels)+'</details>'+
 '<div class="grid">'+performanceNumber('fast_share','Fast group share (%)',c.fast_share,50,90,10)+performanceNumber('fast_count','Fast group size (0 = automatic)',c.fast_count,0,16)+
 performanceNumber('probe_interval_seconds','Probe interval (seconds)',c.probe_interval_seconds,10,60)+performanceNumber('probe_timeout_ms','Probe timeout (ms)',c.probe_timeout_ms,1000,5000)+performanceNumber('max_rtt_ms','Maximum healthy RTT (ms)',c.max_rtt_ms,500,5000)+'</div>'+
 '<label>DNS transport<select name="dns_mode"><option value="tcp">TCP · 1.1.1.1 / 8.8.8.8</option><option value="doh">HTTPS · 1.1.1.1 / 8.8.8.8</option></select></label>'+
 '<details><summary>Advanced tuning</summary>'+
 performanceCheck('gateway_ipv4','IPv4 for proxy gateway connections',c.gateway_ipv4)+
 performanceCheck('tcp_options','Managed outbound TCP options',c.tcp_options)+performanceCheck('tcp_fast_open','TCP Fast Open (requires kernel support)',c.tcp_fast_open)+
 '<div class="grid">'+performanceNumber('keepalive_idle_seconds','Keepalive idle (seconds)',c.keepalive_idle_seconds,15,120)+performanceNumber('keepalive_interval_seconds','Keepalive interval (seconds)',c.keepalive_interval_seconds,10,60)+performanceNumber('tcp_user_timeout_ms','TCP user timeout (ms)',c.tcp_user_timeout_ms,10000,60000)+'</div>'+
 '<label>Routing DNS strategy<select name="routing_strategy"><option value="preserve">Preserve current strategy</option><option value="AsIs">AsIs</option></select></label>'+
 '<label>Buffer per connection (KiB)<select name="buffer_kb">'+[0,32,64,128,256,512].map(n=>'<option value="'+n+'" '+(n===c.buffer_kb?'selected':'')+'>'+(n||'Preserve')+'</option>').join('')+'</select></label>'+
 '<p class="muted">Relative proxy cost: 1 is neutral; a larger value makes a path less attractive. This is not a hard connection limit.</p>'+
 (cache.residential||[]).filter(p=>p.enabled).map(p=>'<label>'+esc(p.name)+'<input data-proxy-cost="'+esc(p.id)+'" type="number" min="0.25" max="4" step="0.05" value="1"></label>').join('')+
 '</details><div data-preview tabindex="-1"></div>');
 const submit=form.querySelector('[data-submit]'),err=form.querySelector('[data-error]'),mode=form.querySelector('[name=mode]'),future=form.querySelector('[name=future]');
 let draft=null,busy=false;const boxes=[...form.querySelectorAll('[name=panel]')];
 const invalidate=()=>{if(busy)return;draft=null;submit.textContent='Preview publication';form.querySelector('[data-preview]').replaceChildren()};
 const count=()=>{form.querySelector('[data-selection-count]').textContent=boxes.filter(x=>x.checked).length+' / '+panels.length+' current servers selected'+(future.checked?' · Future servers included':'')};
 const duration=()=>{const timed=mode.value==='timed';form.querySelector('[data-timer]').hidden=!timed;form.querySelector('[name=minutes]').disabled=!timed;future.disabled=timed;if(timed)future.checked=false;boxes.forEach((b,i)=>{b.disabled=timed&&!panels[i].eligible;if(b.disabled)b.checked=false});count();invalidate()};
 boxes.forEach(b=>b.checked=true);duration();
 form.querySelector('[data-select-all]').onclick=()=>{boxes.forEach(b=>b.checked=!b.disabled);count();invalidate()};
 form.querySelector('[data-clear-selection]').onclick=()=>{future.checked=false;boxes.forEach(b=>b.checked=false);count();invalidate()};
 future.onchange=()=>{if(future.checked)boxes.forEach(b=>b.checked=true);count();invalidate()};
 mode.onchange=duration;
 boxes.forEach(b=>b.onchange=()=>{if(!b.checked)future.checked=false;count();invalidate()});
 form.querySelector('form').onsubmit=e=>{e.preventDefault();submit.click()};
 form.addEventListener('input',invalidate);form.addEventListener('change',invalidate);
 submit.onclick=async()=>{
  if(busy)return;err.textContent='';
  try{
   if(!draft){
    if(!form.querySelector('form').reportValidity())return;
    const f=new FormData(form.querySelector('form')),ids=boxes.filter(x=>x.checked).map(x=>x.value),fleet=future.checked;
    if(!ids.length&&!fleet)throw Error('Select servers or choose all current and future servers.');
    const config={...c,costs:{}};
    for(const k of ['fast_share','fast_count','probe_interval_seconds','probe_timeout_ms','max_rtt_ms','keepalive_idle_seconds','keepalive_interval_seconds','tcp_user_timeout_ms','buffer_kb'])config[k]=Number(f.get(k));
    for(const k of ['gateway_ipv4','tcp_options','tcp_fast_open'])config[k]=f.get(k)==='on';
    for(const k of ['dns_mode','routing_strategy'])config[k]=f.get(k);
    for(const el of form.querySelectorAll('[data-proxy-cost]'))if(Number(el.value)!==1)config.costs[el.dataset.proxyCost]=Number(el.value);
    if(config.max_rtt_ms>config.probe_timeout_ms)throw Error('Maximum healthy RTT must not exceed probe timeout.');
    const reviewed=await api('/api/v1/residential-performance');
    draft={action:'start',mode:mode.value,scope:fleet?'fleet':'selected',panel_ids:fleet?[]:ids,config,minutes:mode.value==='timed'?Number(f.get('minutes')):0,base_revision:reviewed.revision,base_plan:ids.length===1?(reviewed.panels.find(p=>p.id===ids[0])?.plan_hash||''):''};
    form.querySelector('[data-preview]').innerHTML='<h4>Review publication</h4><p>'+esc(mode.value==='permanent'?'Permanent · No expiry · Manual rollback':'Timed test · '+draft.minutes+' minutes')+'</p><p>'+esc(fleet?'All '+panels.length+' current servers and every future server':ids.length+' selected servers')+'</p><details><summary>Current targets</summary><p>'+esc(panels.filter(p=>fleet||ids.includes(p.id)).map(p=>p.label).join(', '))+'</p></details><p>'+esc('Fast group '+config.fast_share+'% / '+(config.fast_count||'automatic')+' paths; probe '+config.probe_interval_seconds+'s / '+config.probe_timeout_ms+'ms; max RTT '+config.max_rtt_ms+'ms; '+config.dns_mode.toUpperCase()+' DNS.')+'</p><p class="muted">Previous settings are saved separately for every server. Publication is complete on a server only after runtime verification.</p>';
    submit.textContent=mode.value==='permanent'?'Publish permanently':'Start timed test';form.querySelector('[data-preview]').focus();return;
   }
   busy=true;submit.disabled=true;await performanceRequest(draft);form.close();toast(draft.mode==='permanent'?'Permanent publication queued':'Timed test queued');await refreshResidentialPerformance();
  }catch(e){err.textContent=e.message;if(e.message.includes('routing settings changed')){draft=null;submit.textContent='Preview publication'}}finally{busy=false;submit.disabled=false}
 };
}
async function openPerformancePromotion(){
 const d=performanceStatus,e=d?.experiment;if(!e?.can_keep)return;
 const used=new Set(e.targets.map(t=>t.panel_id)),panels=d.panels.filter(p=>p.eligible&&!used.has(p.id));
 const form=residentialDialog('Add test servers','<p>The same profile and deadline apply.</p><div class="cardActions"><button type="button" data-select-all>Select all</button><button type="button" class="ghost" data-clear-selection>Clear selection</button></div>'+performanceServerRows(panels));
 form.querySelector('[data-select-all]').onclick=()=>form.querySelectorAll('[name=panel]').forEach(x=>x.checked=true);
 form.querySelector('[data-clear-selection]').onclick=()=>form.querySelectorAll('[name=panel]').forEach(x=>x.checked=false);
 const submit=form.querySelector('[data-submit]');submit.textContent='Add selected servers';
 submit.onclick=async()=>{const ids=[...form.querySelectorAll('[name=panel]:checked')].map(x=>x.value);if(!ids.length)return;submit.disabled=true;try{await performanceRequest({action:'promote',experiment_id:e.id,expected_version:e.version,panel_ids:ids});form.close();await refreshResidentialPerformance()}catch(err){form.querySelector('[data-error]').textContent=err.message}finally{submit.disabled=false}};
}
function openPermanentFleetPublish(){
 const e=performanceStatus?.experiment;if(!e||!['RUNNING','KEPT'].includes(e.state))return;
 const form=residentialDialog('Publish to all current & future servers','<p>The current profile will apply permanently to all current servers and every new server. The timer stops. Manual rollback remains available for the complete publication.</p><p>'+esc(e.config.fast_share+'% fast group · '+e.config.dns_mode.toUpperCase()+' DNS')+'</p>');
 const submit=form.querySelector('[data-submit]');submit.textContent='Publish permanently';
 submit.onclick=async()=>{submit.disabled=true;try{await performanceRequest({action:'publish',scope:'fleet',experiment_id:e.id,expected_version:e.version});form.close();await refreshResidentialPerformance()}catch(err){form.querySelector('[data-error]').textContent=err.message}finally{submit.disabled=false}};
}
document.addEventListener('click',async event=>{
 const b=event.target.closest('[data-performance]');if(!b||b.disabled)return;
 const action=b.dataset.performance;
 if(action==='start'){await openPerformanceTest();return}
 if(action==='promote'){await openPerformancePromotion();return}
 if(action==='publish'){openPermanentFleetPublish();return}
 const e=performanceStatus?.experiment;if(!e)return;
 b.disabled=true;performanceBusy=true;
 try{await performanceRequest({action,experiment_id:e.id,expected_version:e.version});toast(action==='rollback'?'Inheritance stopped. Rollback queued; waiting for server verification':'Verified settings kept')}
 catch(err){toast(err.message,'error')}
 finally{performanceBusy=false;await refreshResidentialPerformance()}
});
setInterval(()=>{if(current==='residential'&&!document.hidden)refreshResidentialPerformance()},5000);
