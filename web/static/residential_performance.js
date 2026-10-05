/* Performance intent is durable on the server; this UI never owns the rollback timer. */
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
  const names=new Map(d.panels.map(p=>[p.id,p.label]));
  slot.innerHTML='<h3>Performance & rollback</h3><p class="muted">Start on one verified server. An unconfirmed test returns to its previous profile at the deadline. A disconnected server stays pending until restoration is verified.</p>'+
   (d.routing_enabled?'':'<p class="error">Routing is paused. Queued changes and rollback require routing execution to resume.</p>')+'<div class="cardActions"><button data-performance="start" '+(active?'disabled':'')+'>Test performance profile</button>'+
   (e?.state==='RUNNING'?'<button data-performance="keep" '+(e.can_keep?'':'disabled')+'>Keep verified settings</button><button class="ghost" data-performance="promote" '+(e.can_keep?'':'disabled')+'>Add test servers</button>':'')+
   (e&&['RUNNING','KEPT','ROLLING_BACK'].includes(e.state)?'<button class="danger" data-performance="rollback" '+(e.state==='ROLLING_BACK'?'disabled':'')+'>'+(e.state==='ROLLING_BACK'?'Rollback pending…':'Rollback')+'</button>':'')+'</div>'+
   (e?'<p role="status"><strong>'+esc(e.state)+'</strong> · '+(e.state==='RUNNING'?'Auto rollback: '+esc(new Date(e.deadline).toLocaleString()):esc(e.reason||''))+'</p>'+
    '<p class="muted">'+esc(e.config.fast_share)+'% fast group · '+esc(e.config.fast_count||'auto')+' paths · '+esc(e.config.dns_mode.toUpperCase())+' DNS</p>'+
    '<div class="tableWrap"><table class="table"><thead><tr><th>Server</th><th>Result</th><th>Detail</th></tr></thead><tbody>'+e.targets.map(t=>'<tr><td>'+esc(names.get(t.panel_id)||t.panel_id)+'</td><td>'+esc(t.state)+'</td><td>'+esc(t.error||'')+'</td></tr>').join('')+'</tbody></table></div>':'<p class="muted">No performance test has been started.</p>')+
   '<p class="muted">Rollback preserves current proxies and clients. Three failed runtime verifications also trigger rollback during a test. Closing this page does not cancel the deadline. Roll back the current profile before testing a different one.</p>';
 }catch(e){slot.textContent='Performance status unavailable: '+e.message}
}
function performanceNumber(name,label,value,min,max,step=1){return '<label>'+label+'<input name="'+name+'" type="number" min="'+min+'" max="'+max+'" step="'+step+'" value="'+value+'" required></label>'}
function performanceCheck(name,label,value){return '<label class="remember"><input type="checkbox" name="'+name+'" '+(value?'checked':'')+'><span>'+label+'</span></label>'}
async function openPerformanceTest(){
 await refreshResidentialPerformance();const d=performanceStatus;if(!d)return;
 const c=d.defaults,panels=d.panels.filter(p=>p.eligible);
 const form=residentialDialog('Test performance profile',
 '<p class="muted">Existing Ads-only routing and UDP remain enabled. DNS uses the server direct path. Health probes measure response time, not bandwidth or ad loading.</p>'+
 '<label>Test server<select name="panel_id" required>'+panels.map(p=>'<option value="'+esc(p.id)+'">'+esc(p.label)+'</option>').join('')+'</select></label>'+
 performanceNumber('minutes','Automatic rollback after (minutes)',15,5,60)+
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
 '</details><div data-preview></div>');
 const submit=form.querySelector('[data-submit]'),err=form.querySelector('[data-error]');submit.textContent='Preview test';let draft=null,busy=false;
 form.querySelector('form').onsubmit=e=>{e.preventDefault();submit.click()};
 form.addEventListener('input',()=>{if(!busy){draft=null;submit.textContent='Preview test';form.querySelector('[data-preview]').replaceChildren()}});
 submit.onclick=async()=>{
  if(busy)return;err.textContent='';
  try{
   if(!draft){
    if(!form.querySelector('form').reportValidity())return;
    const f=new FormData(form.querySelector('form')),p=panels.find(p=>p.id===f.get('panel_id'));if(!p)throw Error('Wait for a fresh verified server before starting.');
    const config={...c,costs:{}};
    for(const k of ['fast_share','fast_count','probe_interval_seconds','probe_timeout_ms','max_rtt_ms','keepalive_idle_seconds','keepalive_interval_seconds','tcp_user_timeout_ms','buffer_kb'])config[k]=Number(f.get(k));
    for(const k of ['gateway_ipv4','tcp_options','tcp_fast_open'])config[k]=f.get(k)==='on';
    for(const k of ['dns_mode','routing_strategy'])config[k]=f.get(k);
    for(const el of form.querySelectorAll('[data-proxy-cost]'))if(Number(el.value)!==1)config.costs[el.dataset.proxyCost]=Number(el.value);
    if(config.max_rtt_ms>config.probe_timeout_ms)throw Error('Maximum healthy RTT must not exceed probe timeout.');
    draft={action:'start',panel_ids:[p.id],config,minutes:Number(f.get('minutes')),base_revision:d.revision,base_plan:p.plan_hash};
    form.querySelector('[data-preview]').textContent='Ready to test '+p.label+' for '+draft.minutes+' minutes. Fast share '+config.fast_share+'%, group '+(config.fast_count||'automatic')+', '+config.dns_mode.toUpperCase()+' DNS. Previous settings will be saved before the server changes.';
    submit.textContent='Start timed test';return;
   }
   busy=true;submit.disabled=true;await performanceRequest(draft);form.close();toast('Timed test queued');await refreshResidentialPerformance();
  }catch(e){err.textContent=e.message}finally{busy=false;submit.disabled=false}
 };
}
async function openPerformancePromotion(){
 const d=performanceStatus,e=d?.experiment;if(!e?.can_keep)return;
 const used=new Set(e.targets.map(t=>t.panel_id)),panels=d.panels.filter(p=>p.eligible&&!used.has(p.id));
 const form=residentialDialog('Add test servers','<p>The same profile and deadline apply. Current targets must remain verified.</p>'+panels.map(p=>'<label class="remember"><input type="checkbox" name="panel" value="'+esc(p.id)+'"><span>'+esc(p.label)+'</span></label>').join(''));
 const submit=form.querySelector('[data-submit]');submit.textContent='Add selected servers';
 submit.onclick=async()=>{const ids=[...form.querySelectorAll('[name=panel]:checked')].map(x=>x.value);if(!ids.length)return;submit.disabled=true;try{await performanceRequest({action:'promote',experiment_id:e.id,expected_version:e.version,panel_ids:ids});form.close();await refreshResidentialPerformance()}catch(err){form.querySelector('[data-error]').textContent=err.message}finally{submit.disabled=false}};
}
document.addEventListener('click',async event=>{
 const b=event.target.closest('[data-performance]');if(!b||b.disabled)return;
 const action=b.dataset.performance;
 if(action==='start'){await openPerformanceTest();return}
 if(action==='promote'){await openPerformancePromotion();return}
 const e=performanceStatus?.experiment;if(!e)return;
 b.disabled=true;performanceBusy=true;
 try{await performanceRequest({action,experiment_id:e.id,expected_version:e.version});toast(action==='rollback'?'Rollback queued; waiting for server verification':'Verified settings kept')}
 catch(err){toast(err.message,'error')}
 finally{performanceBusy=false;await refreshResidentialPerformance()}
});
setInterval(()=>{if(current==='residential'&&!document.hidden)refreshResidentialPerformance()},5000);
