document.getElementById("modal").addEventListener("close",()=>{document.getElementById("modalSubmit").disabled=false});
let configProfileCatalog=null;
async function refreshConfigProfiles(){
 const slot=document.querySelector('#configProfilesSlot');if(!slot)return;
 try{
  const data=await api('/api/v1/configs/profiles');
  if(!slot.isConnected)return;
  if(!Array.isArray(data.profiles)||!Array.isArray(data.drivers))throw Error('Invalid profile status');
  configProfileCatalog=data;
  const cards=data.profiles.map(item=>{
   const p=item.profile,s=p.spec;
   return '<div class="card section"><div class="cardHeader"><h3>'+esc(p.name)+'</h3><button class="small ghost" data-edit-profile="'+esc(p.id)+'">Edit profile</button></div>'+
    '<p>'+esc(s.protocol.toUpperCase()+' · '+s.network.toUpperCase()+' · '+s.security.toUpperCase())+' · Port '+esc(s.port)+'</p>'+
    '<p>'+esc(p.classes.join(' + '))+' · '+esc(p.clients_per_class)+' config per class per server</p>'+
    '<p>'+(p.enabled?'Automatic · '+item.verified+' / '+item.eligible+' eligible servers verified':'Disabled · excluded from output')+
    (item.pending?' · '+item.pending+' pending':'')+(item.blocked?' · '+item.blocked+' need attention':'')+'</p>'+
    '<p class="muted">Saved changes are applied and checked automatically. Update the subscription after changing a port or transport.</p></div>';
  }).join('');
  const issues=(data.issues||[]).filter(x=>data.profiles.some(v=>v.profile.id===x.profile_id&&v.profile.enabled));
  slot.innerHTML='<div class="card section"><h3>Config profiles</h3><p>Each profile has its own protocol, network, security, port and server scope. New creates an independent profile. Eligible new servers are enrolled automatically.</p>'+
   '<p class="muted">Output contains only profiles that pass current server, configuration and routing checks.</p></div>'+cards+
   (!cards?'<p class="muted">Use New to add a profile. Existing Reality policies are shown below.</p>':'')+
   (issues.length?'<details class="card section"><summary>Profile attention · '+issues.length+'</summary>'+issues.map(x=>'<p>'+esc(x.panel_id.slice(0,8))+' · '+esc(x.stage)+' · '+esc(x.code)+'</p>').join('')+'</details>':'');
  slot.querySelectorAll('[data-edit-profile]').forEach(b=>b.onclick=()=>openConfigProfile(data.profiles.find(x=>x.profile.id===b.dataset.editProfile).profile));
 }catch(e){if(slot.isConnected)slot.innerHTML='<div class="card section"><p>Profile status unavailable.</p><button id="retryProfiles">Retry</button></div>';slot.querySelector('#retryProfiles')?.addEventListener('click',refreshConfigProfiles)}
}
async function openConfigProfile(existing=null){
 try{
  const data=await api('/api/v1/configs/profiles');
  if(!Array.isArray(data.drivers)||!Array.isArray(data.profiles))throw Error('Profile catalog unavailable');
  configProfileCatalog=data;
  const p=existing||{name:'',enabled:true,fleet:true,panel_ids:[],classes:['DIRECT','RESIDENTIAL'],clients_per_class:1,revision:0,spec:{protocol:'vless',network:'tcp',security:'reality',port:0,sni:'www.microsoft.com',flow:'xtls-rprx-vision'}};
  const s=p.spec,catalog=data.drivers;
  $('#modalForm').classList.remove('profileForm');$('#modalForm').noValidate=false;
  $('#modalTitle').textContent=existing?'Edit config profile':'New config profile';
  $('#modalSubmit').hidden=false;$('#modalSubmit').disabled=false;
  const opts=(values)=>values.map(x=>'<option value="'+esc(x)+'">'+esc(x.toUpperCase())+'</option>').join('');
  $('#modalFields').innerHTML='<label>Name<input name="profile_name" maxlength="100" required value="'+esc(p.name)+'" placeholder="My config profile"></label>'+
   '<div class="grid"><label>Protocol<select name="profile_protocol">'+opts([...new Set(catalog.map(x=>x.protocol))])+'</select></label>'+
   '<label>Network<select name="profile_network"></select></label><label>Security<select name="profile_security"></select></label></div>'+
   '<p id="profileCompatibility" class="muted"></p>'+
   '<label>Port<input name="profile_port" type="number" min="1" max="65535" value="'+(s.port||'')+'" placeholder="Automatic free port"></label>'+
   '<p class="muted">Leave empty for an automatically reserved port. Ports used by another profile, Reality or the panel are rejected.</p>'+
   '<div id="profilePathFields"><label>Path / gRPC service name<input name="profile_path" value="'+esc(s.path||'')+'" maxlength="4096"></label><label>Transport host (optional)<input name="profile_host" value="'+esc(s.host||'')+'" maxlength="253"></label></div>'+
   '<div id="profileRealityFields"><label>Reality SNI<input name="profile_sni" value="'+esc(s.sni||'www.microsoft.com')+'" maxlength="253"></label><label><input name="profile_vision" type="checkbox"> Vision flow (TCP only)</label></div>'+
   '<label><input name="profile_direct" type="checkbox"> Direct configs</label><label><input name="profile_residential" type="checkbox"> Residential configs</label>'+
   '<label>Configs per class per server<input name="profile_count" type="number" min="1" max="100" required value="'+esc(p.clients_per_class)+'"></label>'+
   '<p class="muted">These profile credentials have no client quota or expiry; they follow server lifetime. Existing Reality policies retain their configured quotas, expiry and automatic replacement.</p>'+
   '<label><input name="profile_fleet" type="checkbox"> All eligible servers, including future servers</label>'+
   '<label id="profileScopeLabel">Selected panel IDs (one per line)<textarea name="profile_panels" rows="3">'+esc((p.panel_ids||[]).join('\n'))+'</textarea></label>'+
   '<label><input name="profile_enabled" type="checkbox"> Enable automatic creation and output</label>'+
   (existing?'<p class="muted">Port and transport changes preserve credentials. For another protocol or client membership, create an independent profile. Disabling hides this profile from output; existing imported connections remain available.</p>':'');
  const f=n=>$('#modalFields [name="profile_'+n+'"]');
  f('protocol').value=s.protocol;
  f('direct').checked=p.classes.includes('DIRECT');f('residential').checked=p.classes.includes('RESIDENTIAL');
  f('enabled').checked=p.enabled;f('fleet').checked=p.fleet;f('vision').checked=!!s.flow;
  const syncSecurity=()=>{
   const protocol=f('protocol').value,network=f('network').value,security=f('security').value;
   const cap=catalog.find(x=>x.protocol===protocol&&x.network===network&&x.security===security);
   $('#profileCompatibility').textContent=cap&&!cap.public?'Unavailable for public servers: the installed Xray client rejects Trojan TCP without TLS. Plain TCP is limited to trusted private networks.':security==='tls'?'TLS uses an automatically renewed, trusted certificate for the server IP.':security==='none'?'No transport TLS. This protocol uses its own payload encryption.':'REALITY uses independently generated and persisted keys for each listener.';
   $('#modalSubmit').disabled=!cap?.public;
   $('#profilePathFields').hidden=network==='tcp';f('path').required=network!=='tcp';f('path').disabled=network==='tcp';f('host').disabled=network==='tcp';
   if(network!=='tcp'&&!f('path').value)f('path').value=network==='grpc'?'dob':'/dob';
   $('#profileRealityFields').hidden=security!=='reality';f('sni').required=security==='reality';f('sni').disabled=security!=='reality';
   f('vision').disabled=!(protocol==='vless'&&network==='tcp'&&security==='reality');
  };
  const syncNetwork=()=>{
   const prev=f('security').value,choices=catalog.filter(x=>x.protocol===f('protocol').value&&x.network===f('network').value);
   f('security').innerHTML=opts(choices.map(x=>x.security));
   f('security').value=choices.some(x=>x.security===prev)?prev:choices.find(x=>x.public)?.security||choices[0]?.security;
   syncSecurity();
  };
  const syncProtocol=()=>{
   const prev=f('network').value,networks=[...new Set(catalog.filter(x=>x.protocol===f('protocol').value).map(x=>x.network))];
   f('network').innerHTML=opts(networks);f('network').value=networks.includes(prev)?prev:networks.includes('tcp')?'tcp':networks[0];syncNetwork();
  };
  syncProtocol();f('network').value=s.network;syncNetwork();f('security').value=s.security;syncSecurity();
  f('protocol').onchange=syncProtocol;f('network').onchange=syncNetwork;f('security').onchange=syncSecurity;
  const scope=()=>{$('#profileScopeLabel').hidden=f('fleet').checked;f('panels').disabled=f('fleet').checked};f('fleet').onchange=scope;scope();
  if(existing){for(const n of ['protocol','direct','residential','count'])f(n).disabled=true}
  $('#modalError').textContent='';$('#modal').showModal();
  $('#modalForm').onsubmit=async e=>{
   e.preventDefault();
   const spec={protocol:f('protocol').value,network:f('network').value,security:f('security').value,port:Number(f('port').value||0)};
   if(spec.network!=='tcp'){spec.path=f('path').value.trim();spec.host=f('host').value.trim()}
   if(spec.security==='reality'){spec.sni=f('sni').value.trim();if(spec.network==='tcp'&&f('vision').checked)spec.flow='xtls-rprx-vision'}
   const body={...p,name:f('name').value.trim(),spec,enabled:f('enabled').checked,fleet:f('fleet').checked,panel_ids:f('fleet').checked?[]:f('panels').value.split(/\s+/).filter(Boolean),classes:[...(f('direct').checked?['DIRECT']:[]),...(f('residential').checked?['RESIDENTIAL']:[])],clients_per_class:Number(f('count').value)};
   $('#modalSubmit').disabled=true;
   try{await api('/api/v1/configs/profiles'+(existing?'/'+encodeURIComponent(p.id):''),{method:existing?'PUT':'POST',body:JSON.stringify(body)});$('#modal').close();await load('configs')}
   catch(err){$('#modalError').textContent=err.message;syncSecurity()}
  };
 }catch(e){toast(e.message,'error')}
}
setInterval(()=>{if(current==='configs'&&!document.hidden&&!document.querySelector('#modal')?.open)refreshConfigProfiles()},10000);
