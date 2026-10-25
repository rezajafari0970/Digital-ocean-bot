// One profile lifecycle for every registered public capability.
let configProfileCatalog=null,configProfilesRead=null,configProfilesReadSlot=null,configProfilesAbort=null,configProfilesEpoch=0;
function profileRandomHex(){return Array.from(crypto.getRandomValues(new Uint8Array(16)),b=>b.toString(16).padStart(2,'0')).join('')}
function profileRequestID(){const h=profileRandomHex();return h.slice(0,8)+'-'+h.slice(8,12)+'-4'+h.slice(13,16)+'-a'+h.slice(17,20)+'-'+h.slice(20)}
const profileStateLabels={ready:'Ready',syncing:'Applying automatically',verifying:'Checking connections',retrying:'Retrying automatically',paused:'Paused',deleting:'Deleting',waiting_for_servers:'Waiting for servers'};
const profileErrorLabels={profile_server_ineligible:'This server is outside the current publication scope',profile_output_disabled:'Publication is disabled in this profile’s settings',profile_routes_refresh_pending:'Refreshing verified routes; output resumes automatically',profile_class_verification_pending:'Checking each routing class before publication',profile_output_refresh_pending:'Refreshing the native output snapshot',profile_output_verification_pending:'Output is waiting for its publication checks',runtime_retry:'Connection verification will retry automatically',profile_verification_timeout:'Connection verification reached its deadline; an independent retry is scheduled',profile_verification_retry:'Connection verification will retry independently',runtime_prepare:'Checking and updating this server’s verified runtime',profile_runtime_prepare_failed:'Runtime preparation failed; an independent retry is scheduled',client_policy:'Applying and verifying client limits',routes_pending:'Saved; waiting for routing and connection verification',runtime:'Testing the running service and connection',executor_wait:'Queued for this server',credentials:'Preparing this profile’s clients',native:'Applying and observing this server',sni:'Checking Reality SNI candidates',profile_panel_busy:'Queued; another operation on this server is completing',profile_global_policy_inventory_unavailable:'Waiting for this panel’s client inventory',profile_global_policy_unverified:'Client limits need another verification',profile_global_policy_identity_conflict:'Client identity differs from this profile’s ownership',profile_global_policy_read_failed:'Retrying the client policy read',profile_global_policy_ambiguous:'Duplicate client identity needs review',profile_runtime_batch_unavailable:'Connection check timed out; retry scheduled independently',profile_sni_scan_failed:'SNI scan failed; this server will retry independently.',profile_sni_no_verified_target:'No candidate passed TLS 1.3, HTTP/2 and certificate validation.',profile_sni_scan_unavailable:'SNI scanner is waiting for this panel.',profile_class_traffic_retrying:'One routing class needs another connection test; verified classes remain published.',trojan_plain_tcp_private_network_only:'Trojan without TLS is supported only on private networks by the official core.',profile_final_mask_invalid:'FinalMask must contain a valid TCP fragment configuration.',profile_port_conflict:'This port belongs to another configuration.',profile_panel_port_conflict:'This port belongs to the panel.',profile_port_reserved:'This port is reserved for a system service.',profile_path_invalid:'HTTP paths must start with /.',profile_revision_changed:'This profile changed. Reopen it to edit the current version.',profile_is_deleting:'This profile is being deleted.',profile_runtime_binary_unverified:'Checking the running Xray service',profile_runtime_socket_unverified:'Waiting for the listener to start',profile_runtime_users_unverified:'Checking applied users',profile_runtime_traffic_unverified:'Retrying the connection test',profile_certificate_provision_failed:'Retrying certificate issuance',profile_public_port_unreachable:'Checking public port reachability',profile_write_outcome_unconfirmed:'Observing the previous change before any retry',profile_existing_state_conflict:'The server configuration differs from its saved ownership',profile_delete_unverified:'Verifying removal',profile_delete_runtime_unverified:'Waiting for the listener to stop'};
Object.assign(profileErrorLabels,{
 profile_http_host_required:'Enter the HTTP Host for this TCP header.',
 profile_tcp_header_invalid:'Choose None or HTTP for the TCP header.',
 profile_tcp_header_security_unsupported:'HTTP headers cannot be combined with Reality or Vision.',
 profile_delete_panel_unavailable:'Server unavailable; removal will retry automatically.',
 profile_delete_inventory_unavailable:'Waiting for a fresh server inventory to confirm removal.',
 profile_delete_identity_changed:'Removal paused: the server identity differs from this profile.',
 cloudflare_dns_read_permission_denied:'Cloudflare denied DNS access. Grant Zone / DNS / Edit to this token for the selected domain, then refresh domains and access.',
 cloudflare_dns_write_permission_denied:'Cloudflare denied DNS changes. Grant Zone / DNS / Edit for the selected domain, then refresh domains and access.',
 cloudflare_certificate_permission_denied:'Cloudflare denied certificate access. Grant Zone / SSL and Certificates / Edit for the selected domain.',
 cloudflare_ssl_permission_denied:'Cloudflare denied SSL settings access. Grant Zone Settings / Read; automatic Full (strict) also needs Zone Settings / Edit.',
 cloudflare_permission_denied:'Cloudflare rejected this credential or its permissions. Review the token and its selected domains.',
 cloudflare_probe_target_not_ready:'No verified published config is available for this record yet. Resolve its DNS or certificate status first.',
 cloudflare_operation_retrying:'Cloudflare access could not be verified; a bounded retry is scheduled.',
 cloudflare_dns_quota_unavailable:'DNS capacity could not be refreshed. Existing record checks and cleanup continue.',
 cloudflare_busy:'A Cloudflare update is already running. Wait a moment, then refresh status.',
 cloudflare_certificate_issue_unconfirmed:'Recovering the previously requested certificate before considering another issuance.',
 cloudflare_certificate_list_incomplete:'The complete certificate list could not be read. Recovery will retry without issuing a duplicate.',
 cloudflare_response_invalid:'Cloudflare returned an unexpected response. The existing operation is being checked before retrying.',
 cloudflare_response_unconfirmed:'The Cloudflare response was incomplete. Checking the existing operation before retrying.',
 cloudflare_transport_unconfirmed:'Cloudflare did not confirm the request. Checking its outcome before retrying.'
});
const configMessage=code=>{const base=String(code||'').replace(/^profile_/,'');return profileErrorLabels[code]||profileErrorLabels[base]||base.replaceAll('_',' ')};
function profileServerStatus(x){
 if(x.state==='RETIRED')return 'Removed';
 if(x.removing){
  if(x.code?.startsWith('profile_delete_'))return configMessage(x.code);
  return x.stage==='deleting'?'Removing clients and listener':'Queued for removal';
 }
 if(x.code)return configMessage(x.code);
 if(x.applied_revision!==x.desired_revision)return 'Applying saved changes';
 if(x.publication_code)return configMessage(x.publication_code);
 if(x.state==='VERIFIED')return Date.now()-Date.parse(x.verified_at)<300000?'Verified':'Awaiting fresh verification';
 return configMessage(x.stage);
}
function profileRequest(p){return {id:p.id,name:p.name,spec:p.spec,enabled:p.enabled,fleet:p.fleet,panel_ids:p.panel_ids,classes:p.classes,clients_per_class:p.clients_per_class,class_settings:p.class_settings,revision:p.revision,legacy_key:p.legacy_key||'',request_id:p.request_id||''}}
function setProfileBusy(id,busy){document.querySelectorAll('[data-profile-id]').forEach(card=>{if(card.dataset.profileId===id)card.querySelectorAll('button').forEach(b=>b.disabled=busy)})}
async function refreshConfigProfiles(){
 if(!token||current!=='configs'||document.querySelector('#app')?.hidden||document.querySelector('#modal')?.open)return;
 const slot=document.querySelector('#configProfilesSlot');if(!slot)return;
 if(configProfilesRead&&configProfilesReadSlot===slot)return configProfilesRead;
 configProfilesAbort?.abort();configProfilesAbort=new AbortController();const controller=configProfilesAbort,epoch=++configProfilesEpoch;
 configProfilesReadSlot=slot;const view=activePageView;
 configProfilesRead=(async()=>{
  try{
   const data=await viewRead('/api/v1/configs/profiles',view,controller.signal);if(!currentPageView(view)||!slot.isConnected||epoch!==configProfilesEpoch)return;
   if(!Array.isArray(data.profiles)||!Array.isArray(data.drivers)||!Array.isArray(data.servers))throw Error('Configuration status unavailable');
   configProfileCatalog=data;slot.querySelector('[data-profile-read-error]')?.remove();
   const retained=new Map([...slot.querySelectorAll('details[open][data-profile-servers]')].map(x=>[x.dataset.profileServers,x]));
   const open=new Set(retained.keys());
   const stamp=JSON.stringify([data.profiles,data.reset,data.unified]);
   if(slot.dataset.snapshot===stamp){const at=slot.querySelector('[data-observed-at]');if(at)at.textContent=formatDate(data.observed_at);slot.querySelectorAll('details[open][data-profile-servers]').forEach(refreshProfileServers);return}
   slot.dataset.snapshot=stamp;
   const reset=data.reset&&data.reset.done<data.reset.total?'<div class="card section"><h3>Removing previous configurations</h3><p>'+esc(data.reset.done)+' / '+esc(data.reset.total)+' servers completed. Remaining servers are checked automatically.</p></div>':'';
   slot.innerHTML='<div class="card section"><h3>Configurations</h3><p>Create a profile, choose its port and count, and save. Current and future selected servers are kept in sync automatically.</p><p class="muted">Status checked: <span data-observed-at>'+esc(formatDate(data.observed_at))+'</span></p>'+(data.profiles.some(x=>!x.profile.delete_requested_at)?'<button class="small ghost" data-delete-all-profiles>Delete all configurations</button>':'')+'</div>'+reset+
    data.profiles.map(item=>{
     const p=item.profile,s=p.spec,deleting=!!p.delete_requested_at;
     return '<article class="card section" data-profile-id="'+esc(p.id)+'" data-profile-revision="'+esc(p.revision)+'"><div class="cardHeader"><h3>'+esc(p.name)+'</h3><span class="badge '+(item.state==='ready'?'ok':'')+'">'+esc(profileStateLabels[item.state]||item.state)+'</span></div>'+
      '<p>'+esc([s.protocol.toUpperCase(),s.network.toUpperCase(),s.security.toUpperCase()].join(' · '))+' · Port '+esc(s.port)+'</p>'+
      '<p>'+p.classes.map(c=>esc((c==='DIRECT'?'Direct':'Residential')+': '+(p.class_settings?.[c]?.count??p.clients_per_class)+' per server')).join(' · ')+'</p>'+
      (deleting?'<p>'+esc(item.deleting)+' listeners awaiting confirmed removal.</p>':!p.enabled?'<p>Creation and output are paused.'+(item.deleting?' Removing '+esc(item.deleting)+' listeners.':'')+'</p>':
      (item.class_stats?'<p>'+Object.entries(item.class_stats).map(([c,v])=>esc((c==='DIRECT'?'Direct':'Residential')+': '+v.active+' / '+v.target+' created · '+v.published+' / '+v.publication_target+' published')).join('<br>')+'</p>':'')+
      '<div class="grid"><div><span class="muted">Ready servers</span><h3>'+esc(item.ready)+' / '+esc(item.eligible)+'</h3></div><div><span class="muted">Published configs</span><h3>'+esc(item.published)+' / '+esc(item.target)+'</h3></div></div>'+
      (item.pending||item.blocked?'<p>'+esc(item.pending)+' applying · '+esc(item.blocked)+' retrying</p>':''))+
      (!deleting?'<div class="cardActions"><button class="small" data-edit-profile="'+esc(p.id)+'">Edit</button><button class="small ghost" data-toggle-profile="'+esc(p.id)+'">'+(p.enabled?'Pause':'Resume')+'</button><button class="small ghost" data-delete-profile="'+esc(p.id)+'">Delete</button></div>':'')+
      '<details data-profile-servers="'+esc(p.id)+'"'+(open.has(p.id)?' open':'')+'><summary>Server details</summary><div data-server-list><p class="muted">Loading…</p></div></details></article>';
    }).join('')+(!data.profiles.length?'<div class="card section"><h3>No configurations</h3><p>Tap New to create your first profile. No protocol is created automatically.</p></div>':'');
   slot.querySelectorAll('details[data-profile-servers]').forEach(d=>{const old=retained.get(d.dataset.profileServers);if(old)d.replaceWith(old)});
   const deleteAll=slot.querySelector('[data-delete-all-profiles]');if(deleteAll)deleteAll.onclick=()=>deleteAllConfigProfilesDialog(data.profiles.map(x=>x.profile).filter(p=>!p.delete_requested_at));
   const byID=id=>data.profiles.find(x=>x.profile.id===id)?.profile;
   slot.querySelectorAll('[data-edit-profile]').forEach(b=>b.onclick=()=>openConfigProfile(byID(b.dataset.editProfile)));
   slot.querySelectorAll('[data-toggle-profile]').forEach(b=>b.onclick=async()=>{const p=byID(b.dataset.toggleProfile);setProfileBusy(p.id,true);try{await api('/api/v1/configs/profiles/'+encodeURIComponent(p.id),{method:'PUT',body:JSON.stringify({...profileRequest(p),enabled:!p.enabled})});slot.dataset.snapshot='';await refreshConfigProfilesAfterRead()}catch(e){toast(configMessage(e.message),'error');setProfileBusy(p.id,false)}});
   slot.querySelectorAll('[data-delete-profile]').forEach(b=>b.onclick=()=>deleteConfigProfileDialog(byID(b.dataset.deleteProfile)));
   slot.querySelectorAll('[data-profile-servers]').forEach(d=>{d.ontoggle=()=>{if(d.open)refreshProfileServers(d)};if(d.open)refreshProfileServers(d)});
  }catch(e){if(currentPageView(view)&&slot.isConnected&&epoch===configProfilesEpoch){
   slot.querySelector('[data-profile-read-error]')?.remove();
   if(!slot.querySelector('[data-profile-id]')){slot.innerHTML='';delete slot.dataset.snapshot}
   slot.insertAdjacentHTML('afterbegin','<div class="card section" data-profile-read-error><p>Status is unavailable. Any displayed counts are from the last successful check.</p><button data-retry-profiles>Retry status</button></div>');
   slot.querySelector('[data-retry-profiles]').onclick=()=>refreshConfigProfiles();
  }}
 })().finally(()=>{controller.abort();if(epoch===configProfilesEpoch){configProfilesRead=null;configProfilesReadSlot=null}});
 return configProfilesRead;
}
async function refreshConfigProfilesAfterRead(){if(configProfilesRead)await configProfilesRead;return refreshConfigProfiles()}
const profileServerReads=new WeakMap();
async function refreshProfileServers(details){
 if(!details.open||!details.isConnected)return;
 if(profileServerReads.has(details))return profileServerReads.get(details);
 const view=activePageView;
 const read=(async()=>{try{const data=await viewRead('/api/v1/configs/profiles/'+encodeURIComponent(details.dataset.profileServers)+'/servers',view);if(!currentPageView(view)||!details.isConnected||!details.open)return;
  const stamp=JSON.stringify(data.servers);if(details.dataset.serverSnapshot===stamp)return;details.dataset.serverSnapshot=stamp;
  details.querySelector('[data-server-list]').innerHTML=data.servers.length?'<div class="tableWrap"><table class="table"><thead><tr><th>Server</th><th>Status</th><th>Output</th><th>Last verified</th><th>Retry / SNI</th></tr></thead><tbody>'+data.servers.map(x=>'<tr><td>'+esc(x.host)+'</td><td>'+esc(profileServerStatus(x))+'</td><td>'+esc(Number.isInteger(x.published)?x.published+' / '+x.publication_target:'—')+'</td><td>'+esc(x.verified_at&&!x.verified_at.startsWith('0001')?formatDate(x.verified_at):'Pending')+'</td><td>'+esc(x.next_attempt_at&&!x.next_attempt_at.startsWith('0001')?formatDate(x.next_attempt_at):'Queued')+(x.failures?'<br>'+esc(x.failures)+' failed attempts':'')+(x.sni?'<br>'+esc(x.sni):'')+'</td></tr>').join('')+'</tbody></table></div>':'<p>No servers in this profile yet.</p>';
 }catch(e){if(currentPageView(view)&&details.isConnected){delete details.dataset.serverSnapshot;details.querySelector('[data-server-list]').innerHTML='<p>Server status unavailable. Retrying automatically.</p>'}}
 })().finally(()=>profileServerReads.delete(details));profileServerReads.set(details,read);return read;
}
setInterval(()=>{if(current==='configs'&&!document.hidden&&!document.querySelector('#modal')?.open)document.querySelectorAll('details[open][data-profile-servers]').forEach(refreshProfileServers)},2000);

function deleteConfigProfileDialog(p){
 $('#modalTitle').textContent='Delete '+p.name+'?';$('#modalForm').noValidate=false;$('#modalForm').classList.remove('profileForm');
 $('#modalFields').innerHTML='<p>This removes this profile’s users and listeners from its servers and removes its links from output. Deletion continues automatically after you close this window.</p>';
 $('#modalError').textContent='';$('#modalSubmit').hidden=false;$('#modalSubmit').disabled=false;$('#modalSubmit').textContent='Delete profile';$('#modal').showModal();
 $('#modalForm').onsubmit=async e=>{e.preventDefault();$('#modalSubmit').disabled=true;setProfileBusy(p.id,true);try{await api('/api/v1/configs/profiles/'+encodeURIComponent(p.id),{method:'DELETE',body:JSON.stringify({revision:p.revision})});$('#modal').close();await refreshConfigProfilesAfterRead()}catch(err){setProfileBusy(p.id,false);$('#modalError').textContent=configMessage(err.message);$('#modalSubmit').disabled=false}};
}
function deleteAllConfigProfilesDialog(profiles){
 const snapshot=profiles.map(p=>({id:p.id,revision:p.revision}));
 $('#modalTitle').textContent='Delete all '+snapshot.length+' configurations?';$('#modalForm').classList.remove('profileForm');$('#modalForm').noValidate=false;
 $('#modalFields').innerHTML='<p>This removes these profiles and their clients from every assigned server. Removal runs independently on each server and continues automatically. Profiles created after this selection are excluded.</p>';
 $('#modalError').textContent='';$('#modalSubmit').hidden=false;$('#modalSubmit').disabled=false;$('#modalSubmit').textContent='Delete all configurations';$('#modal').showModal();
 $('#modalForm').onsubmit=async e=>{e.preventDefault();$('#modalSubmit').disabled=true;
  try{const result=await api('/api/v1/configs/profiles/delete-batch',{method:'POST',body:JSON.stringify({profiles:snapshot})});$('#modal').close();if(result.conflicts)toast(result.conflicts+' changed profiles were kept. Review them before deleting.','error');await refreshConfigProfilesAfterRead()}
  catch(err){$('#modalError').textContent=configMessage(err.message);$('#modalSubmit').disabled=false}
 };
}
async function openConfigProfile(existing=null){
 try{
  const view=activePageView,data=await viewRead('/api/v1/configs/profiles',view);if(!currentPageView(view))return;if(!Array.isArray(data.drivers))throw Error('Configuration catalog unavailable');
  const catalog=data.drivers.filter(x=>x.public),p=existing||{name:'',enabled:true,fleet:true,panel_ids:[],classes:['DIRECT','RESIDENTIAL'],clients_per_class:1,revision:0,request_id:profileRequestID(),spec:{protocol:'',network:'',security:'',port:0}};
  const cfData=await viewRead('/api/v1/configs/cloudflare',view);if(!currentPageView(view))return;
  const s=p.spec,pathValues={};if(s.network)pathValues[s.network]=s.path||'';
  $('#modalForm').classList.remove('profileForm');$('#modalForm').noValidate=false;$('#modalTitle').textContent=existing?'Edit configuration':'New configuration';$('#modalSubmit').hidden=false;$('#modalSubmit').disabled=false;$('#modalSubmit').textContent='Save';
  const options=(values,placeholder='')=>(placeholder?'<option value="">'+placeholder+'</option>':'')+values.map(x=>'<option value="'+esc(x)+'">'+esc(x.toUpperCase())+'</option>').join('');
  $('#modalFields').innerHTML='<label>Name (optional)<input name="profile_name" maxlength="100" value="'+esc(p.name)+'" placeholder="A name for this profile"></label>'+
   '<div class="grid"><label>Protocol<select name="profile_protocol" required>'+options([...new Set(catalog.map(x=>x.protocol))],'Select protocol')+'</select></label><label>Network<select name="profile_network" required></select></label><label>Security<select name="profile_security" required></select></label></div>'+
   '<p id="profileCompatibility" class="muted"></p><label>Port<input name="profile_port" type="number" step="any" min="1" max="65535" value="'+(s.port||'')+'" placeholder="Automatic free port"></label><p class="muted">Leave empty to reserve a free port. Each profile owns its port on its selected servers.</p>'+
   cloudflareProfileFields(s,cfData.accounts)+
   ['DIRECT','RESIDENTIAL'].map(cls=>{
    const key=cls.toLowerCase(),v=p.class_settings?.[cls]||{count:p.clients_per_class,lifetime_seconds:0,device_limit:0,quota_bytes:0,users_per_second:0,creation_interval_seconds:0,flow:s.flow||'',publish:true,allow_insecure:false,export_final_mask:false};
    const n=(field,label,value,min=0,max=31536000)=>'<label>'+label+'<input name="profile_'+key+'_'+field+'" type="number" step="any" min="'+min+'" max="'+max+'" value="'+esc(value||0)+'" required></label>';
    const unit=v.lifetime_seconds&&v.lifetime_seconds%3600===0?'hours':'minutes';
    return '<fieldset><legend><label><input name="profile_'+key+'" type="checkbox"> '+(cls==='DIRECT'?'Direct':'Residential')+'</label></legend><div data-class-fields="'+key+'">'+
     n('count','Configs per server',v.count,1,100)+'<div class="grid">'+n('lifetime','Lifetime (0 = server lifetime)',v.lifetime_seconds/(unit==='hours'?3600:60),0,525600)+'<label>Unit<select name="profile_'+key+'_lifetime_unit"><option value="minutes"'+(unit==='minutes'?' selected':'')+'>Minutes</option><option value="hours"'+(unit==='hours'?' selected':'')+'>Hours</option></select></label></div>'+
     n('devices','Device limit (0 = unlimited)',v.device_limit,0,1000)+
     '<details><summary>Creation and output settings</summary>'+n('quota','Quota in bytes (0 = unlimited)',v.quota_bytes,0,9007199254740991)+n('rate','Maximum users per second (0 = automatic)',v.users_per_second,0,100)+n('interval','Creation interval in seconds (0 = automatic)',v.creation_interval_seconds,0,86400)+
     '<label>Flow<select name="profile_'+key+'_flow"><option value="">None</option><option value="xtls-rprx-vision"'+(v.flow==='xtls-rprx-vision'?' selected':'')+'>Vision · UDP 443 allowed</option></select></label>'+
     '<label><input name="profile_'+key+'_publish" type="checkbox"'+(v.publish?' checked':'')+'> Publish in subscriptions</label><label><input name="profile_'+key+'_insecure" type="checkbox"'+(v.allow_insecure?' checked':'')+'> Skip certificate verification (TLS only)</label>'+
     '<label>FinalMask (client JSON)<textarea name="profile_'+key+'_mask" rows="4" maxlength="16384">'+esc(v.final_mask||'')+'</textarea></label><label><input name="profile_'+key+'_export_mask" type="checkbox"'+(v.export_final_mask?' checked':'')+'> Include FinalMask in exported configs</label><p class="muted">Leave export disabled for the plain output format. The saved mask remains available.</p></details></div></fieldset>';
   }).join('')+
   '<details><summary>Connection settings</summary><label id="profileTCPHeaderFields">TCP header<select name="profile_header_type"><option value="">None</option><option value="http">HTTP</option></select></label><div id="profilePathFields"><label id="profilePathLabel">Path / service name<input name="profile_path" maxlength="4096" value=""></label><label>Transport host (optional)<input name="profile_host" maxlength="253" value="'+esc(s.host||'')+'"></label></div><div id="profileRealityFields"><label>SNI selection<select name="profile_sni_mode"><option value="manual">Specified SNI, verified on each server</option><option value="auto">Automatic scanner on each server</option></select></label><label>Reality SNI<input name="profile_sni" maxlength="253" value="'+esc(s.sni||'www.microsoft.com')+'"></label><label>Scanner candidates (optional, comma separated)<input name="profile_sni_candidates" maxlength="5000" value="'+esc(s.sni_candidates||'')+'"></label><p class="muted">Automatic selection requires a valid certificate, TLS 1.3 and HTTP/2. The selected SNI is shown per server.</p></div></details>'+
   '<label><input name="profile_fleet" type="checkbox"> All current and future eligible servers</label><div id="profileScopeLabel">'+data.servers.map(x=>'<label><input type="checkbox" name="profile_selected" value="'+esc(x.id)+'"'+(p.panel_ids.includes(x.id)?' checked':'')+'> '+esc(x.host)+' · '+esc(x.region)+'</label>').join('')+'</div><label><input name="profile_enabled" type="checkbox"> Enabled</label>';
  const f=n=>$('#modalFields [name="profile_'+n+'"]');f('protocol').value=s.protocol;f('direct').checked=p.classes.includes('DIRECT');f('residential').checked=p.classes.includes('RESIDENTIAL');f('fleet').checked=p.fleet;f('enabled').checked=p.enabled;f('sni_mode').value=s.sni_mode||'manual';f('header_type').value=s.header_type==='http'?'http':'';
  const syncCFSchedule=bindCloudflareSchedule(f);
  let previousNetwork='';
  const syncSecurity=()=>{const network=f('network').value,security=f('security').value;
   const cdnSupported=network==='ws'&&security==='tls';f('cf_enabled').disabled=!cdnSupported;if(!cdnSupported)f('cf_enabled').checked=false;
   const cdn=f('cf_enabled').checked&&!f('cf_enabled').disabled;$('#profileCloudflareFields [data-cf-profile-options]').hidden=!cdn;
   $('#profileCloudflareFields [data-cf-compatibility]').textContent=cdnSupported?'Cloudflare manages domain, SNI and strict TLS certificates.':'Select WS + TLS to use Cloudflare CDN.';
   syncCFSchedule();
   $('#profileCompatibility').textContent=security==='tls'?'TLS certificates are issued and renewed automatically.':security==='reality'?'Reality keys are generated and retained independently for this profile.':'';
   const tcpHeader=network==='tcp'&&security!=='reality';$('#profileTCPHeaderFields').hidden=!tcpHeader;f('header_type').disabled=!tcpHeader;
    if(!tcpHeader)f('header_type').value='';
    const httpHeader=tcpHeader&&f('header_type').value==='http';
    if(httpHeader){if(!f('host').value)f('host').value='play.googleapis.com';if(!f('path').value)f('path').value='/'}
    $('#profilePathFields').hidden=!network||(network==='tcp'&&!httpHeader);f('path').required=!!network&&(network!=='tcp'||httpHeader);f('path').disabled=$('#profilePathFields').hidden;f('host').disabled=f('path').disabled||cdn;f('host').required=httpHeader;
   $('#profileRealityFields').hidden=security!=='reality';f('sni').required=security==='reality';f('sni').disabled=security!=='reality';['direct','residential'].forEach(cls=>{
    const vision=f('protocol').value==='vless'&&network==='tcp'&&security==='reality';f(cls+'_flow').disabled=!vision;if(!vision)f(cls+'_flow').value='';
    f(cls+'_insecure').disabled=security!=='tls'||cdn;if(security!=='tls'||cdn)f(cls+'_insecure').checked=false;
    f(cls+'_mask').disabled=security==='none';f(cls+'_export_mask').disabled=security==='none';if(security==='none')f(cls+'_export_mask').checked=false;
   });
   f('sni').required=security==='reality'&&f('sni_mode').value!=='auto';f('sni').disabled=security!=='reality'||f('sni_mode').value==='auto';
   f('sni_candidates').disabled=security!=='reality'||f('sni_mode').value!=='auto';
   const capability=catalog.find(x=>x.protocol===f('protocol').value&&x.network===network&&x.security===security);
   if(capability?.client_requirement==='plain_tcp_compatible_client')$('#profileCompatibility').textContent='Plain TCP: no TLS or Reality. Requires a compatible client; recent official Xray clients reject unencrypted VLESS and Trojan on public IPs.';
  };
  const syncNetwork=()=>{
   if(previousNetwork)pathValues[previousNetwork]=f('path').value;
   const network=f('network').value;f('path').value=pathValues[network]||(network==='grpc'?'dob_'+profileRandomHex():network==='tcp'?'':'/'+profileRandomHex());previousNetwork=network;
   const previous=f('security').value,choices=catalog.filter(x=>x.protocol===f('protocol').value&&x.network===network).map(x=>x.security);f('security').innerHTML=options(choices);f('security').value=choices.includes(previous)?previous:choices[0]||'';syncSecurity();
  };
  const syncProtocol=()=>{const previous=f('network').value,networks=[...new Set(catalog.filter(x=>x.protocol===f('protocol').value).map(x=>x.network))];f('network').innerHTML=options(networks);f('network').value=networks.includes(previous)?previous:networks.includes('tcp')?'tcp':networks[0]||'';syncNetwork()};
  syncProtocol();if(existing){f('network').value=s.network;syncNetwork();f('security').value=s.security;f('cf_enabled').checked=!!s.cloudflare_account;syncSecurity()}
  f('protocol').onchange=syncProtocol;f('network').onchange=syncNetwork;f('security').onchange=syncSecurity;f('header_type').onchange=syncSecurity;f('sni_mode').onchange=syncSecurity;f('cf_enabled').onchange=syncSecurity;
  const syncClasses=()=>{['direct','residential'].forEach(cls=>{$('[data-class-fields="'+cls+'"]').hidden=!f(cls).checked;document.querySelectorAll('[data-class-fields="'+cls+'"] input[type=number]').forEach(x=>x.required=f(cls).checked)});};f('direct').onchange=syncClasses;f('residential').onchange=syncClasses;syncClasses();
  const scope=()=>{$('#profileScopeLabel').hidden=f('fleet').checked};f('fleet').onchange=scope;scope();$('#modalError').textContent='';$('#modal').showModal();
  $('#modalForm').onsubmit=async e=>{
   e.preventDefault();const cfIssue=syncCFSchedule();if(cfIssue){$('#modalError').textContent=cfIssue.detail;f('cf_'+cfIssue.field).reportValidity();return}const classes=[...(f('direct').checked?['DIRECT']:[]),...(f('residential').checked?['RESIDENTIAL']:[])];if(!classes.length){$('#modalError').textContent='Select Direct, Residential, or both.';return}
   const spec={protocol:f('protocol').value,network:f('network').value,security:f('security').value,port:Number(f('port').value||0)};
   if(spec.network==='tcp'&&!f('header_type').disabled&&f('header_type').value==='http')spec.header_type='http';
    if(spec.network!=='tcp'||spec.header_type==='http'){spec.path=f('path').value.trim();spec.host=f('host').value.trim();if((spec.network==='ws'||spec.network==='xhttp'||spec.header_type==='http')&&!spec.path.startsWith('/')){$('#modalError').textContent='The HTTP path must start with /.';return}}
   if(spec.security==='reality'){spec.sni_mode=f('sni_mode').value;spec.sni=spec.sni_mode==='auto'?'':f('sni').value.trim();if(spec.sni_mode==='auto')spec.sni_candidates=f('sni_candidates').value.split(',').map(x=>x.trim()).filter(Boolean).join(',')}
   if(f('cf_enabled').checked&&!f('cf_enabled').disabled){
    spec.cloudflare_operator=f('cf_operator').value;spec.cloudflare_account=f('cf_account').value;spec.cloudflare_records=Number(f('cf_count').value);spec.cloudflare_interval_seconds=Number(f('cf_interval').value)*60;spec.cloudflare_min_lifetime_seconds=Number(f('cf_min').value)*60;spec.cloudflare_max_lifetime_seconds=Number(f('cf_max').value)*60;spec.cloudflare_max_active=Number(f('cf_active').value);spec.host='';
   }
   const class_settings={};
   for(const cls of ['DIRECT','RESIDENTIAL']){const k=cls.toLowerCase(),v={count:Number(f(k+'_count').value),lifetime_seconds:Math.round(Number(f(k+'_lifetime').value)*(f(k+'_lifetime_unit').value==='hours'?3600:60)),device_limit:Number(f(k+'_devices').value),quota_bytes:Number(f(k+'_quota').value),users_per_second:Number(f(k+'_rate').value),creation_interval_seconds:Number(f(k+'_interval').value),flow:f(k+'_flow').disabled?'':f(k+'_flow').value,publish:f(k+'_publish').checked,allow_insecure:!f(k+'_insecure').disabled&&f(k+'_insecure').checked,final_mask:f(k+'_mask').disabled?'':f(k+'_mask').value.trim(),export_final_mask:!f(k+'_export_mask').disabled&&f(k+'_export_mask').checked};
    if(v.final_mask){try{const mask=JSON.parse(v.final_mask);if(!mask||Array.isArray(mask)||typeof mask!=='object')throw Error();v.final_mask=JSON.stringify(mask)}catch{$('#modalError').textContent=cls+': FinalMask must be a JSON object.';return}}
    class_settings[cls]=v;
   }
   const panels=[...document.querySelectorAll('[name="profile_selected"]:checked')].map(x=>x.value);if(!f('fleet').checked&&!panels.length){$('#modalError').textContent='Select at least one server.';return}
   const body={...(existing?profileRequest(p):{request_id:p.request_id}),name:f('name').value.trim()||[spec.protocol.toUpperCase(),spec.network.toUpperCase(),spec.security.toUpperCase()].join(' · '),spec,enabled:f('enabled').checked,fleet:f('fleet').checked,panel_ids:f('fleet').checked?[]:panels,classes,class_settings,clients_per_class:class_settings[classes[0]].count};
   $('#modalSubmit').disabled=true;if(existing)setProfileBusy(p.id,true);
   try{await api('/api/v1/configs/profiles'+(existing?'/'+encodeURIComponent(p.id):''),{method:existing?'PUT':'POST',body:JSON.stringify(body)});$('#modal').close();await refreshConfigProfilesAfterRead()}catch(err){if(existing)setProfileBusy(p.id,false);$('#modalError').textContent=configMessage(err.message);$('#modalSubmit').disabled=false}
  };
 }catch(e){toast(configMessage(e.message),'error')}
}
document.getElementById('modal').addEventListener('close',()=>{document.getElementById('modalSubmit').disabled=false;document.getElementById('modalSubmit').textContent='Save'});
setInterval(()=>{if(current==='configs'&&!document.hidden&&!document.querySelector('#modal')?.open)refreshConfigProfiles()},5000);