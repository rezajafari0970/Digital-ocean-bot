// One profile lifecycle for every registered public capability.
let configProfileCatalog=null,configProfilesRead=null,configProfilesReadSlot=null,configProfilesAbort=null,configProfilesEpoch=0;
function profileRandomHex(){return Array.from(crypto.getRandomValues(new Uint8Array(16)),b=>b.toString(16).padStart(2,'0')).join('')}
function profileRequestID(){const h=profileRandomHex();return h.slice(0,8)+'-'+h.slice(8,12)+'-4'+h.slice(13,16)+'-a'+h.slice(17,20)+'-'+h.slice(20)}
const profileStateLabels={ready:'Ready',syncing:'Applying automatically',retrying:'Retrying automatically',paused:'Paused',deleting:'Deleting',waiting_for_servers:'Waiting for servers'};
const profileErrorLabels={profile_port_conflict:'This port belongs to another configuration.',profile_panel_port_conflict:'This port belongs to the panel.',profile_port_reserved:'This port is reserved for a system service.',profile_path_invalid:'HTTP paths must start with /.',profile_revision_changed:'This profile changed. Reopen it to edit the current version.',profile_is_deleting:'This profile is being deleted.',profile_runtime_binary_unverified:'Checking the running Xray service',profile_runtime_socket_unverified:'Waiting for the listener to start',profile_runtime_users_unverified:'Checking applied users',profile_runtime_traffic_unverified:'Retrying the connection test',profile_certificate_provision_failed:'Retrying certificate issuance',profile_public_port_unreachable:'Checking public port reachability',profile_write_outcome_unconfirmed:'Observing the previous change before any retry',profile_existing_state_conflict:'The server configuration differs from its saved ownership',profile_delete_unverified:'Verifying removal',profile_delete_runtime_unverified:'Waiting for the listener to stop'};
const configMessage=code=>profileErrorLabels[code]||String(code||'').replace(/^profile_/,'').replaceAll('_',' ');
function profileRequest(p){return {id:p.id,name:p.name,spec:p.spec,enabled:p.enabled,fleet:p.fleet,panel_ids:p.panel_ids,classes:p.classes,clients_per_class:p.clients_per_class,revision:p.revision,legacy_key:p.legacy_key||'',request_id:p.request_id||''}}
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
   const open=new Set([...slot.querySelectorAll('details[open][data-profile-servers]')].map(x=>x.dataset.profileServers));
   const stamp=JSON.stringify([data.profiles,data.reset,data.unified]);
   if(slot.dataset.snapshot===stamp){const at=slot.querySelector('[data-observed-at]');if(at)at.textContent=formatDate(data.observed_at);return}
   slot.dataset.snapshot=stamp;
   const reset=data.reset&&data.reset.done<data.reset.total?'<div class="card section"><h3>Removing previous configurations</h3><p>'+esc(data.reset.done)+' / '+esc(data.reset.total)+' servers completed. Remaining servers are checked automatically.</p></div>':'';
   slot.innerHTML='<div class="card section"><h3>Configurations</h3><p>Create a profile, choose its port and count, and save. Current and future selected servers are kept in sync automatically.</p><p class="muted">Status checked: <span data-observed-at>'+esc(formatDate(data.observed_at))+'</span></p></div>'+reset+
    data.profiles.map(item=>{
     const p=item.profile,s=p.spec,deleting=!!p.delete_requested_at;
     return '<article class="card section" data-profile-id="'+esc(p.id)+'" data-profile-revision="'+esc(p.revision)+'"><div class="cardHeader"><h3>'+esc(p.name)+'</h3><span class="badge '+(item.state==='ready'?'ok':'')+'">'+esc(profileStateLabels[item.state]||item.state)+'</span></div>'+
      '<p>'+esc([s.protocol.toUpperCase(),s.network.toUpperCase(),s.security.toUpperCase()].join(' · '))+' · Port '+esc(s.port)+'</p>'+
      '<p>'+esc(p.clients_per_class)+' per server for '+esc(p.classes.map(c=>c==='DIRECT'?'Direct':'Residential').join(' and '))+'</p>'+
      (deleting?'<p>'+esc(item.deleting)+' listeners awaiting confirmed removal.</p>':!p.enabled?'<p>Creation and output are paused.'+(item.deleting?' Removing '+esc(item.deleting)+' listeners.':'')+'</p>':
      '<div class="grid"><div><span class="muted">Ready servers</span><h3>'+esc(item.ready)+' / '+esc(item.eligible)+'</h3></div><div><span class="muted">Published configs</span><h3>'+esc(item.published)+' / '+esc(item.target)+'</h3></div></div>'+
      (item.pending||item.blocked?'<p>'+esc(item.pending)+' applying · '+esc(item.blocked)+' retrying</p>':''))+
      (!deleting?'<div class="cardActions"><button class="small" data-edit-profile="'+esc(p.id)+'">Edit</button><button class="small ghost" data-toggle-profile="'+esc(p.id)+'">'+(p.enabled?'Pause':'Resume')+'</button><button class="small ghost" data-delete-profile="'+esc(p.id)+'">Delete</button></div>':'')+
      '<details data-profile-servers="'+esc(p.id)+'"'+(open.has(p.id)?' open':'')+'><summary>Server details</summary><div data-server-list><p class="muted">Loading…</p></div></details></article>';
    }).join('')+(!data.profiles.length?'<div class="card section"><h3>No configurations</h3><p>Tap New to create your first profile. No protocol is created automatically.</p></div>':'');
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
async function refreshProfileServers(details){
 try{const data=await viewRead('/api/v1/configs/profiles/'+encodeURIComponent(details.dataset.profileServers)+'/servers',activePageView);if(!details.isConnected||!details.open)return;
  details.querySelector('[data-server-list]').innerHTML=data.servers.length?'<div class="tableWrap"><table class="table"><thead><tr><th>Server</th><th>Status</th><th>Last verified</th></tr></thead><tbody>'+data.servers.map(x=>'<tr><td>'+esc(x.host)+'</td><td>'+esc(x.code?configMessage(x.code):x.state==='VERIFIED'&&x.applied_revision===x.desired_revision?'Verified':x.state==='RETIRED'?'Removed':x.applied_revision!==x.desired_revision?'Applying saved changes':configMessage(x.stage))+'</td><td>'+esc(x.verified_at&&!x.verified_at.startsWith('0001')?formatDate(x.verified_at):'Pending')+'</td></tr>').join('')+'</tbody></table></div>':'<p>No servers in this profile yet.</p>';
 }catch(e){if(details.isConnected)details.querySelector('[data-server-list]').innerHTML='<p>Server status unavailable. Close and reopen to retry.</p>'}
}
function deleteConfigProfileDialog(p){
 $('#modalTitle').textContent='Delete '+p.name+'?';$('#modalForm').noValidate=false;$('#modalForm').classList.remove('profileForm');
 $('#modalFields').innerHTML='<p>This removes this profile’s users and listeners from its servers and removes its links from output. Deletion continues automatically after you close this window.</p>';
 $('#modalError').textContent='';$('#modalSubmit').hidden=false;$('#modalSubmit').disabled=false;$('#modalSubmit').textContent='Delete profile';$('#modal').showModal();
 $('#modalForm').onsubmit=async e=>{e.preventDefault();$('#modalSubmit').disabled=true;setProfileBusy(p.id,true);try{await api('/api/v1/configs/profiles/'+encodeURIComponent(p.id),{method:'DELETE',body:JSON.stringify({revision:p.revision})});$('#modal').close();await refreshConfigProfilesAfterRead()}catch(err){setProfileBusy(p.id,false);$('#modalError').textContent=configMessage(err.message);$('#modalSubmit').disabled=false}};
}
async function openConfigProfile(existing=null){
 try{
  const view=activePageView,data=await viewRead('/api/v1/configs/profiles',view);if(!currentPageView(view))return;if(!Array.isArray(data.drivers))throw Error('Configuration catalog unavailable');
  const catalog=data.drivers.filter(x=>x.public),p=existing||{name:'',enabled:true,fleet:true,panel_ids:[],classes:['DIRECT','RESIDENTIAL'],clients_per_class:1,revision:0,request_id:profileRequestID(),spec:{protocol:'',network:'',security:'',port:0}};
  const s=p.spec,pathValues={};if(s.network)pathValues[s.network]=s.path||'';
  $('#modalForm').classList.remove('profileForm');$('#modalForm').noValidate=false;$('#modalTitle').textContent=existing?'Edit configuration':'New configuration';$('#modalSubmit').hidden=false;$('#modalSubmit').disabled=false;$('#modalSubmit').textContent='Save';
  const options=(values,placeholder='')=>(placeholder?'<option value="">'+placeholder+'</option>':'')+values.map(x=>'<option value="'+esc(x)+'">'+esc(x.toUpperCase())+'</option>').join('');
  $('#modalFields').innerHTML='<label>Name (optional)<input name="profile_name" maxlength="100" value="'+esc(p.name)+'" placeholder="A name for this profile"></label>'+
   '<div class="grid"><label>Protocol<select name="profile_protocol" required>'+options([...new Set(catalog.map(x=>x.protocol))],'Select protocol')+'</select></label><label>Network<select name="profile_network" required></select></label><label>Security<select name="profile_security" required></select></label></div>'+
   '<p id="profileCompatibility" class="muted"></p><label>Port<input name="profile_port" type="number" min="1" max="65535" value="'+(s.port||'')+'" placeholder="Automatic free port"></label><p class="muted">Leave empty to reserve a free port. Each profile owns its port on its selected servers.</p>'+
   '<label>Configs per class per server<input name="profile_count" type="number" min="1" max="100" required value="'+esc(p.clients_per_class)+'"></label><label><input name="profile_direct" type="checkbox"> Direct</label><label><input name="profile_residential" type="checkbox"> Residential</label><p class="muted">Changing the count keeps existing users where possible. These configs last for the server lifetime.</p>'+
   '<details><summary>Connection settings</summary><div id="profilePathFields"><label id="profilePathLabel">Path / service name<input name="profile_path" maxlength="4096" value=""></label><label>Transport host (optional)<input name="profile_host" maxlength="253" value="'+esc(s.host||'')+'"></label></div><div id="profileRealityFields"><label>Reality SNI<input name="profile_sni" maxlength="253" value="'+esc(s.sni||'www.microsoft.com')+'"></label><label><input name="profile_vision" type="checkbox"> Vision flow (TCP)</label></div></details>'+
   '<label><input name="profile_fleet" type="checkbox"> All current and future eligible servers</label><div id="profileScopeLabel">'+data.servers.map(x=>'<label><input type="checkbox" name="profile_selected" value="'+esc(x.id)+'"'+(p.panel_ids.includes(x.id)?' checked':'')+'> '+esc(x.host)+' · '+esc(x.region)+'</label>').join('')+'</div><label><input name="profile_enabled" type="checkbox"> Enabled</label>';
  const f=n=>$('#modalFields [name="profile_'+n+'"]');f('protocol').value=s.protocol;f('direct').checked=p.classes.includes('DIRECT');f('residential').checked=p.classes.includes('RESIDENTIAL');f('fleet').checked=p.fleet;f('enabled').checked=p.enabled;f('vision').checked=s.flow!==''&&(!!s.flow||!existing);
  let previousNetwork='';
  const syncSecurity=()=>{const network=f('network').value,security=f('security').value;
   $('#profileCompatibility').textContent=security==='tls'?'TLS certificates are issued and renewed automatically.':security==='reality'?'Reality keys are generated and retained independently for this profile.':'';
   $('#profilePathFields').hidden=!network||network==='tcp';f('path').required=!!network&&network!=='tcp';f('path').disabled=!network||network==='tcp';f('host').disabled=f('path').disabled;
   $('#profileRealityFields').hidden=security!=='reality';f('sni').required=security==='reality';f('sni').disabled=security!=='reality';f('vision').disabled=!(f('protocol').value==='vless'&&network==='tcp'&&security==='reality');
  };
  const syncNetwork=()=>{
   if(previousNetwork)pathValues[previousNetwork]=f('path').value;
   const network=f('network').value;f('path').value=pathValues[network]||(network==='grpc'?'dob_'+profileRandomHex():network==='tcp'?'':'/'+profileRandomHex());previousNetwork=network;
   const previous=f('security').value,choices=catalog.filter(x=>x.protocol===f('protocol').value&&x.network===network).map(x=>x.security);f('security').innerHTML=options(choices);f('security').value=choices.includes(previous)?previous:choices[0]||'';syncSecurity();
  };
  const syncProtocol=()=>{const previous=f('network').value,networks=[...new Set(catalog.filter(x=>x.protocol===f('protocol').value).map(x=>x.network))];f('network').innerHTML=options(networks);f('network').value=networks.includes(previous)?previous:networks.includes('tcp')?'tcp':networks[0]||'';syncNetwork()};
  syncProtocol();if(existing){f('network').value=s.network;syncNetwork();f('security').value=s.security;syncSecurity()}
  f('protocol').onchange=syncProtocol;f('network').onchange=syncNetwork;f('security').onchange=syncSecurity;
  const scope=()=>{$('#profileScopeLabel').hidden=f('fleet').checked};f('fleet').onchange=scope;scope();$('#modalError').textContent='';$('#modal').showModal();
  $('#modalForm').onsubmit=async e=>{
   e.preventDefault();const classes=[...(f('direct').checked?['DIRECT']:[]),...(f('residential').checked?['RESIDENTIAL']:[])];if(!classes.length){$('#modalError').textContent='Select Direct, Residential, or both.';return}
   const spec={protocol:f('protocol').value,network:f('network').value,security:f('security').value,port:Number(f('port').value||0)};
   if(spec.network!=='tcp'){spec.path=f('path').value.trim();spec.host=f('host').value.trim();if((spec.network==='ws'||spec.network==='xhttp')&&!spec.path.startsWith('/')){$('#modalError').textContent='The HTTP path must start with /.';return}}
   if(spec.security==='reality'){spec.sni=f('sni').value.trim();if(spec.network==='tcp'&&f('vision').checked)spec.flow='xtls-rprx-vision'}
   const panels=[...document.querySelectorAll('[name="profile_selected"]:checked')].map(x=>x.value);if(!f('fleet').checked&&!panels.length){$('#modalError').textContent='Select at least one server.';return}
   const body={...(existing?profileRequest(p):{request_id:p.request_id}),name:f('name').value.trim()||[spec.protocol.toUpperCase(),spec.network.toUpperCase(),spec.security.toUpperCase()].join(' · '),spec,enabled:f('enabled').checked,fleet:f('fleet').checked,panel_ids:f('fleet').checked?[]:panels,classes,clients_per_class:Number(f('count').value)};
   $('#modalSubmit').disabled=true;if(existing)setProfileBusy(p.id,true);
   try{await api('/api/v1/configs/profiles'+(existing?'/'+encodeURIComponent(p.id):''),{method:existing?'PUT':'POST',body:JSON.stringify(body)});$('#modal').close();await refreshConfigProfilesAfterRead()}catch(err){if(existing)setProfileBusy(p.id,false);$('#modalError').textContent=configMessage(err.message);$('#modalSubmit').disabled=false}
  };
 }catch(e){toast(configMessage(e.message),'error')}
}
document.getElementById('modal').addEventListener('close',()=>{document.getElementById('modalSubmit').disabled=false;document.getElementById('modalSubmit').textContent='Save'});
setInterval(()=>{if(current==='configs'&&!document.hidden&&!document.querySelector('#modal')?.open)refreshConfigProfiles()},5000);
