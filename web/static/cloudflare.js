// Cloudflare credentials are write-only; they never enter page caches or storage.
async function openCloudflareSettings(){
 try{
  const data=await api('/api/v1/configs/cloudflare');
  $('#modalTitle').textContent='Cloudflare domains';$('#modalForm').classList.remove('profileForm');$('#modalError').textContent='';$('#modalSubmit').hidden=true;
  $('#modalFields').innerHTML='<p>API tokens are recommended. Allow Zone Read, DNS Edit, SSL and Certificates Edit, and Zone Settings Read for the domains you select. Zone Settings Edit is needed only for automatic Full (strict).</p><div class="cardActions"><button type="button" data-cf-new>Add Cloudflare account</button><button type="button" data-cf-status>Refresh status</button><button type="button" data-cf-ranges>Refresh official IP ranges</button></div><label>Import operator probe results<input type="file" accept="application/json,.json" data-cf-import></label>'+
   (data.capacity_available===false?'<p class="error">Fleet capacity could not be checked. Refresh status before planning additional servers.</p>':'')+
   (data.accounts.length?data.accounts.map(a=>cloudflareAccountCard(a,data.capacity?.[a.id])).join(''):'<p>No Cloudflare account connected yet.</p>')+
   '<p>Official IP ranges: <a href="https://www.cloudflare.com/ips/" target="_blank" rel="noopener">cloudflare.com/ips</a>. Ranges identify Cloudflare ownership; operator speed requires measurements from that operator.</p>';
  $('#modalFields [data-cf-new]').onclick=()=>editCloudflareAccount();
  $('#modalFields [data-cf-status]').onclick=()=>openCloudflareSettings();
  $('#modalFields [data-cf-ranges]').onclick=async e=>{e.target.disabled=true;try{await api('/api/v1/configs/cloudflare/ips/refresh',{method:'POST'});toast('Official IP ranges refreshed')}catch(err){$('#modalError').textContent=configMessage(err.message)}finally{e.target.disabled=false}};
  data.accounts.forEach(a=>{const root=[...$('#modalFields').querySelectorAll('[data-cf-records]')].find(x=>x.dataset.cfRecords===a.id);if(root)bindCloudflareRecords(root,a.records,a.record_count)});

  $('#modalFields [data-cf-import]').onchange=async e=>{const file=e.target.files[0];if(!file)return;try{if(file.size>65536)throw Error('Probe file is too large');const report=JSON.parse(await file.text());if(!Array.isArray(report.results)||report.results.length>64)throw Error('Invalid probe results');let saved=0;for(const result of report.results){if(!result.protocol_verified)continue;await api('/api/v1/configs/cloudflare/measurements',{method:'POST',body:JSON.stringify(result)});saved++}toast(saved+' operator measurements saved')}catch(err){$('#modalError').textContent=configMessage(err.message)}finally{e.target.value=''}};

  $('#modalFields').querySelectorAll('[data-cf-edit]').forEach(b=>b.onclick=()=>editCloudflareAccount(data.accounts.find(a=>a.id===b.dataset.cfEdit)));
  $('#modalFields').querySelectorAll('[data-cf-refresh]').forEach(b=>b.onclick=async()=>{const label=b.textContent;b.disabled=true;b.textContent='Refreshing…';try{await api('/api/v1/configs/cloudflare/accounts/'+encodeURIComponent(b.dataset.cfRefresh)+'/refresh',{method:'POST'});await openCloudflareSettings()}catch(e){$('#modalError').textContent=configMessage(e.message)}finally{b.disabled=false;b.textContent=label}});
  $('#modalForm').onsubmit=e=>e.preventDefault();if(!$('#modal').open)$('#modal').showModal();
 }catch(e){toast(configMessage(e.message),'error')}
}
function editCloudflareAccount(existing){
 const a=existing||{id:profileRequestID(),name:'',revision:0,enabled:true,manage_ssl:false,zones:[]};
 $('#modalTitle').textContent=existing?'Cloudflare account':'Connect Cloudflare';$('#modalSubmit').hidden=false;$('#modalSubmit').disabled=false;$('#modalSubmit').textContent='Save';$('#modalError').textContent='';
 $('#modalFields').innerHTML='<label>Name<input name="cf_name" maxlength="100" required value="'+esc(a.name)+'"></label><label>Authentication<select name="cf_auth"><option value="token">API token (recommended)</option><option value="key">Global API key and email</option></select></label><label>Token / key'+(existing?' (leave empty to retain)':'')+'<input name="cf_secret" type="password" autocomplete="new-password" maxlength="4096"'+(existing?'':' required')+'></label><label data-cf-email hidden>Email<input name="cf_email" type="email" autocomplete="off"></label><label><input type="checkbox" name="cf_enabled"'+(a.enabled?' checked':'')+'> Enabled</label><label><input type="checkbox" name="cf_ssl"'+(a.manage_ssl?' checked':'')+'> Automatically set Full (strict) for selected zones</label><p>This SSL setting applies to the whole selected zone. Otherwise, Full (strict) must already be enabled. Domains must use a full Cloudflare DNS setup.</p><fieldset><legend>Active domains</legend>'+(a.zones.length?a.zones.map(z=>'<label><input type="checkbox" name="cf_zone" value="'+esc(z.id)+'"'+(z.enabled?' checked':'')+(!z.present||z.status!=='active'?' disabled':'')+'> '+esc(z.name)+' · '+esc(z.present?z.status:'Not accessible')+'</label>').join(''):'<p>Save the credential first to retrieve your domains, then select the domains to use.</p>')+'</fieldset><p>Disabling an account or domain withdraws its links and queues deletion of its managed DNS records.</p>';
 const f=n=>$('#modalFields [name="cf_'+n+'"]');f('auth').onchange=()=>{$('[data-cf-email]').hidden=f('auth').value!=='key';f('email').required=f('auth').value==='key'&&!!f('secret').value};
 $('#modalForm').onsubmit=async e=>{
  e.preventDefault();const body={id:a.id,name:f('name').value.trim(),revision:a.revision,enabled:f('enabled').checked,manage_ssl:f('ssl').checked,zone_ids:[...$('#modalFields').querySelectorAll('[name="cf_zone"]:checked:not(:disabled)')].map(x=>x.value)};
  if(f('secret').value)body.auth=f('auth').value==='token'?{token:f('secret').value}:{key:f('secret').value,email:f('email').value.trim()};
  $('#modalSubmit').disabled=true;
  try{await api('/api/v1/configs/cloudflare/accounts'+(existing?'/'+encodeURIComponent(a.id):''),{method:existing?'PUT':'POST',body:JSON.stringify(body)});f('secret').value='';await openCloudflareSettings()}
  catch(err){$('#modalError').textContent=configMessage(err.message)+' — reload status before retrying an uncertain save.';$('#modalSubmit').disabled=false}
 };
 if(!$('#modal').open)$('#modal').showModal();
}
function cloudflareProfileFields(s,accounts){
 const n=(name,label,value,min,max)=>'<label>'+label+'<input name="profile_cf_'+name+'" type="number" step="1" min="'+min+'" max="'+max+'" aria-describedby="profileCFScheduleHint" value="'+esc(value)+'"></label>';
 return '<fieldset id="profileCloudflareFields"><legend>Cloudflare CDN</legend><label><input type="checkbox" name="profile_cf_enabled"'+(s.cloudflare_account?' checked':'')+'> Automatically use active Cloudflare domains</label><p data-cf-compatibility>Available for WS + TLS. Certificates and transport host are managed automatically.</p><div data-cf-profile-options><label>Account<select name="profile_cf_account"><option value="">Select account</option>'+accounts.filter(a=>a.enabled).map(a=>'<option value="'+esc(a.id)+'"'+(a.id===s.cloudflare_account?' selected':'')+'>'+esc(a.name)+'</option>').join('')+'</select></label><div class="grid">'+n('count','Records per server per interval (1–2)',s.cloudflare_records||1,1,2)+n('interval','Creation interval (minutes)',(s.cloudflare_interval_seconds||1800)/60,5,1435)+n('min','Minimum lifetime (minutes)',(s.cloudflare_min_lifetime_seconds||2700)/60,15,1440)+n('max','Maximum lifetime (minutes)',(s.cloudflare_max_lifetime_seconds||3600)/60,15,1440)+n('active','Maximum active records per server (up to 8)',s.cloudflare_max_active||4,1,8)+'</div><p id="profileCFScheduleHint" aria-live="polite"></p><label>Operator IP preference<select name="profile_cf_operator">'+[['','Automatic DNS'],['mci','MCI'],['irancell','Irancell'],['rightel','Rightel'],['mokhaberat','Mokhaberat'],['shatel','Shatel'],['other','Other']].map(([v,l])=>'<option value="'+v+'"'+(s.cloudflare_operator===v?' selected':'')+'>'+l+'</option>').join('')+'</select></label><p>Uses fresh operator probe results, then verifies the actual CDN connection. Without a fresh result, the owned domain is used.</p><p>Each record receives its own fixed random expiry. Keep capacity for overlapping generations. Refresh subscriptions before old domains expire.</p></div></fieldset>';
}

// Minutes in the form; the API and worker use the shared cloudflareschedule policy.
function cloudflareScheduleFeedback(s){
 const invalid=(field,detail,required=0)=>({field,detail,required});
 for(const field of ['count','interval','min','max','active']){
  if(!Number.isSafeInteger(s[field]))return invalid(field,'Enter a whole number for this schedule field.');
 }
 if(s.count<1||s.count>2)return invalid('count','Records per server per interval must be 1 or 2.');
 if(s.interval<5)return invalid('interval','Creation interval must be at least 5 minutes.');
 if(s.min<15||s.min>1440)return invalid('min','Minimum lifetime must be between 15 minutes and 24 hours.');
 if(s.max<s.min||s.max>1440)return invalid('max','Maximum lifetime must be at least the minimum lifetime and no more than 24 hours.');
 if(s.interval>s.min-5)return invalid('interval','Creation interval must be at least 5 minutes shorter than the minimum lifetime.');
 const required=(Math.ceil(s.max/s.interval)+1)*s.count;
 if(required>8)return invalid('interval','This schedule needs '+required+' active records per server; the limit is 8. Increase the creation interval, shorten the maximum lifetime, or reduce records per interval.',required);
 if(s.active<required)return invalid('active','Maximum active records per server must be at least '+required+' for this schedule (up to 8).',required);
 if(s.active>8)return invalid('active','Maximum active records per server cannot exceed 8.',required);
 return {required};
}
function bindCloudflareSchedule(f){
 const fields=['count','interval','min','max','active'],hint=document.getElementById('profileCFScheduleHint');
 let adjustment='';
 const sync=(adjust=false)=>{
  const enabled=f('cf_enabled').checked&&!f('cf_enabled').disabled;
  for(const name of ['account','operator',...fields]){
   const input=f('cf_'+name);input.disabled=!enabled;input.required=enabled&&name!=='operator';input.setCustomValidity('');
  }
  if(!enabled){hint.textContent='';hint.classList.remove('error');adjustment='';return null}
  const values=Object.fromEntries(fields.map(name=>[name,f('cf_'+name).value.trim()===''?NaN:Number(f('cf_'+name).value)]));
  let result=cloudflareScheduleFeedback(values);
  if(adjust&&result.field==='active'&&result.required<=8&&values.active<result.required){
   values.active=result.required;f('cf_active').value=String(result.required);
   adjustment='Active capacity was raised to '+result.required+'. ';
   result=cloudflareScheduleFeedback(values);
  }
  hint.classList.toggle('error',!!result.detail);
  hint.textContent=result.detail||adjustment+'This schedule needs at least '+result.required+' active records per server, including one spare generation during replacement.';
  if(result.detail){f('cf_'+result.field).setCustomValidity(result.detail);return result}
  return null;
 };
 for(const name of fields){
  const update=()=>{adjustment='';sync(name!=='active')};
  f('cf_'+name).addEventListener('input',update);f('cf_'+name).addEventListener('change',update);
 }
 return sync;
}

function cloudflareAccountCard(a,capacity){
 return '<fieldset class="cloudflareAccount"><legend>'+esc(a.name)+'</legend><p>'+esc(a.enabled?'Enabled':'Paused')+'</p><p>'+esc(a.dns_record_count??0)+' confirmed DNS records · '+esc(a.pending_record_count??0)+' pending requests · '+esc(a.probe_ready_count??0)+' probe-ready records</p>'+
 cloudflareCapacityCard(capacity)+
 (a.error_code?'<p class="error">'+esc(configMessage(a.error_code))+'</p>':'')+
 '<p>'+esc(a.zones.filter(z=>z.enabled).map(z=>z.name).join(', ')||'No domains selected')+'</p>'+
 a.zones.filter(z=>z.enabled).map(z=>'<p>'+esc(z.name)+': '+esc(z.dns_error_code?configMessage(z.dns_error_code):z.dns_checked_at?'DNS read access verified; writes are verified during creation.':'DNS access check pending.')+'</p>'+(z.dns_quota_checked_at?'<p'+(z.dns_record_quota!=null&&z.dns_record_usage>=z.dns_record_quota?' class="error"':'')+'>DNS capacity: '+esc(z.dns_record_usage)+' / '+esc(z.dns_record_quota??'account quota')+(z.dns_record_quota!=null&&z.dns_record_usage>=z.dns_record_quota?'. Full: new records wait for capacity. Existing records and expiry cleanup continue. Select another active domain or increase the Cloudflare quota.':'')+'</p>':'')+(z.dns_quota_error_code?'<p>'+esc(configMessage(z.dns_quota_error_code))+'</p>':'')).join('')+
 '<div class="cardActions"><button type="button" data-cf-edit="'+esc(a.id)+'">Domains and settings</button><button type="button" data-cf-refresh="'+esc(a.id)+'">Refresh domains and access</button></div>'+
 '<details><summary>Record status ('+esc(a.record_count)+' tracked requests)</summary><p>Pending requests are local work, not confirmed DNS records. Up to 500 tracked requests are available here. Old static links expire; refresh your subscription.</p><div data-cf-records="'+esc(a.id)+'"></div></details></fieldset>';
}
function bindCloudflareRecords(root,records,total){
 let page=0;const size=25,pages=Math.max(1,Math.ceil(records.length/size));
 const render=()=>{
  root.innerHTML=records.slice(page*size,(page+1)*size).map(x=>{
   const state={PREPARED:'Waiting to create DNS',SENT:'Creation outcome is being checked',DNS_READY:'DNS exists',DELETING:'Verifying DNS removal',CONFLICT:'Record ownership conflict'}[x.state]||x.state;
   return '<article class="cloudflareRecord"><div class="cloudflareHostname">'+esc(x.hostname)+'</div><p>'+esc(state)+(x.error_code?' · '+esc(configMessage(x.error_code)):'')+'</p><p>'+esc(x.state==='PREPARED'||x.state==='SENT'?'Request deadline: ':'DNS expiry: ')+esc(formatDate(x.expires_at))+'</p><p>Last verified: '+esc(x.verified_at?formatDate(x.verified_at):'Pending')+'</p><button type="button" data-cf-probe="'+esc(x.id)+'"'+(x.probe_ready?'':' disabled')+'>'+(x.probe_ready?'Download probe target':'Probe unavailable until a config is published')+'</button></article>';
  }).join('')||'<p>No tracked requests.</p>';
  if(records.length)root.insertAdjacentHTML('beforeend','<div class="cloudflarePager"><button type="button" data-cf-prev'+(page===0?' disabled':'')+'>Previous</button><span>'+esc(page*size+1)+'–'+esc(Math.min((page+1)*size,records.length))+' / '+esc(total)+'</span><button type="button" data-cf-next'+(page+1===pages?' disabled':'')+'>Next</button></div>');
  root.querySelector('[data-cf-prev]')?.addEventListener('click',()=>{page--;render();root.scrollIntoView({block:'start'})});
  root.querySelector('[data-cf-next]')?.addEventListener('click',()=>{page++;render();root.scrollIntoView({block:'start'})});
  root.querySelectorAll('[data-cf-probe]:not(:disabled)').forEach(b=>b.onclick=()=>downloadCloudflareProbe(b));
 };
 render();
}
async function downloadCloudflareProbe(b){
 b.disabled=true;
 try{
  const target=await api('/api/v1/configs/cloudflare/records/'+encodeURIComponent(b.dataset.cfProbe)+'/probe'),url=URL.createObjectURL(new Blob([JSON.stringify(target,null,2)],{type:'application/json'})),a=document.createElement('a');
  a.href=url;a.download='cloudflare-probe-'+b.dataset.cfProbe+'.json';a.click();setTimeout(()=>URL.revokeObjectURL(url),1000);
 }catch(err){$('#modalError').textContent=configMessage(err.message)}
 finally{b.disabled=false}
}

// The estimate reserves replacement overlap; it is not a promise of verified
// links or permission to consume domains outside the operator's selection.
function cloudflareCapacityCard(c){
 if(!c)return '';
 const incomplete=!c.complete;
 const summary='<p>Planned DNS capacity: '+esc(c.required_records)+' records for '+esc(c.planned_servers)+' server slots across '+esc(c.profiles)+' profiles. Each server counts once per profile. Includes replacement overlap.</p>'+
  '<p>Current eligible servers need '+esc(c.current_required_records)+' records. Known usable capacity: '+esc(c.known_record_capacity)+'; free now: '+esc(c.available_now)+'.</p>';
 let detail='';
 if(incomplete)detail='<p class="error">Capacity is not fully known: '+esc(c.unknown_zones)+' domains need a fresh quota check; '+esc(c.invalid_profiles)+' profiles need a valid schedule. Refresh status before planning more servers.</p>';
 else if(c.shortfall>0)detail='<p class="error">Capacity shortage: '+esc(c.shortfall)+' additional DNS record slots are needed for the planned servers and overlap. Select another dedicated domain, increase the provider quota, or review the rotation schedule.</p>';
 else detail='<p>Observed capacity covers this estimate. DNS creation, TLS and connection verification still determine published links.</p>';
 if(c.blocked_zones)detail+='<p>'+esc(c.blocked_zones)+' selected domains are excluded until access and Full (strict) are verified.</p>';
 return '<div data-cf-capacity>'+summary+detail+'<p>This estimate includes enabled accounts’ server targets and existing servers, subtracts unrelated DNS usage, and does not change your settings.</p></div>';
}
