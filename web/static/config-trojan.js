async function refreshTrojanConfig(){
 const slot=document.querySelector('#configTrojanSlot');if(!slot)return;
 try{
  const data=await api('/api/v1/configs/trojan');if(!slot.isConnected)return;
  if(data.managed_as_profile){slot.innerHTML='';return}
  const p=data.policy,list=data.listeners||[],verified=list.filter(x=>x.state==='VERIFIED').length;
  slot.innerHTML='<div class="card section"><div class="cardHeader"><h3>Trojan TCP · TLS</h3><button class="small ghost" id="editTrojanConfig">Settings</button></div>'+
   '<p>'+(p.enabled?'Enabled':'Disabled')+' · Port '+esc(p.port)+' · '+verified+' / '+list.length+' verified</p>'+
   '<p class="muted">One config per selected class on each server. Uses an automatically renewed, trusted IP certificate and a separate port. Names follow the server country.</p>'+
   '<button class="small ghost" id="refreshTrojanStatus">Refresh status</button>'+
   list.filter(x=>x.state!=='VERIFIED').map(x=>'<div class="kv"><span>'+esc(x.panel_id.slice(0,8))+'</span><span>'+esc(x.stage)+' · '+esc(x.error||x.state)+'</span></div>').join('')+'</div>';
  slot.querySelector('#editTrojanConfig').onclick=()=>openTrojanConfig(data);
  slot.querySelector('#refreshTrojanStatus').onclick=refreshTrojanConfig;
 }catch(e){if(slot.isConnected){slot.innerHTML='<div class="card section"><h3>Trojan TCP</h3><p>Settings unavailable.</p><button class="small ghost">Retry</button></div>';slot.querySelector('button').onclick=refreshTrojanConfig}}
}
function openTrojanConfig(data){
 const p=data.policy;
 $('#modalTitle').textContent='Trojan TCP';
 $('#modalForm').noValidate=false;$('#modalSubmit').hidden=false;
 $('#modalFields').innerHTML='<label><input name="trojan_enabled" type="checkbox"> Enable Trojan generation and output</label>'+
  '<label>Trojan TCP port<input name="trojan_port" type="number" min="1024" max="65535" value="'+esc(p.port)+'" required></label>'+
  '<label><input name="trojan_direct" type="checkbox"> Direct config</label><label><input name="trojan_residential" type="checkbox"> Residential config</label>'+
  '<label><input name="trojan_fleet" type="checkbox"> All eligible servers, including new servers</label>'+
  '<p class="muted">TLS certificates are issued for each server IP and renewed automatically. Port 80 must be free for certificate validation. Conflicting ports are rejected. Existing Reality settings are preserved.</p>'+
  (data.listeners.length?'<p class="muted">Ports and client classes are fixed while managed Trojan listeners exist. Disable stops generation and removes Trojan from shared output; previously imported credentials remain on the servers.</p>':'');
 const f=n=>$('#modalFields [name="'+n+'"]');
 f('trojan_enabled').checked=p.enabled;f('trojan_direct').checked=p.direct_enabled;f('trojan_residential').checked=p.residential_enabled;f('trojan_fleet').checked=p.fleet;
 if(data.listeners.length){f('trojan_port').disabled=true;f('trojan_direct').disabled=true;f('trojan_residential').disabled=true}
 $('#modalError').textContent='';$('#modal').showModal();
 $('#modalForm').onsubmit=async e=>{
  e.preventDefault();$('#modalSubmit').disabled=true;
  try{await api('/api/v1/configs/trojan',{method:'PUT',body:JSON.stringify({...p,enabled:f('trojan_enabled').checked,port:Number(f('trojan_port').value),direct_enabled:f('trojan_direct').checked,residential_enabled:f('trojan_residential').checked,fleet:f('trojan_fleet').checked})});$('#modal').close();await refreshTrojanConfig()}
  catch(err){$('#modalError').textContent=err.message}finally{$('#modalSubmit').disabled=false}
 };
}
