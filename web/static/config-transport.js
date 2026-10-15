async function refreshConfigTransport(){
 const slot=document.querySelector('#configTransportSlot');if(!slot)return;
 try{
  const data=await api('/api/v1/configs/transport');if(!slot.isConnected)return;
  const c=data.policy.config,network=c.network||'tcp';
  slot.innerHTML='<div class="card section"><div class="cardHeader"><h3>Transport for new listeners</h3><button class="small ghost" id="editTransport">Edit transport</button></div><div class="kv"><span>Network</span><span>'+esc(network==='xhttp'?'XHTTP Reality · stream-up':'TCP Reality')+'</span>'+(network==='xhttp'?'<span>XHTTP Host</span><span>'+esc(c.host)+'</span><span>Path length</span><span>'+esc(c.path_bytes)+' bytes · unique per listener</span><span>Flow</span><span>Empty</span><span>Browser Dialer</span><span>Disabled</span><span>FinalMask</span><span>Saved preset · not active</span>':'')+'</div><p class="muted">'+esc(data.scope)+'</p></div>';
  document.querySelector('#editTransport').onclick=()=>openConfigTransport(data);
 }catch(e){if(slot.isConnected)slot.innerHTML='<div class="card section">Transport settings unavailable. <button class="small ghost" id="retryTransport">Retry</button></div>';document.querySelector('#retryTransport')?.addEventListener('click',refreshConfigTransport)}
}
function openConfigTransport(data){
 const policy=data.policy,preset=data.preset,current=policy.config;
 $('#modalForm').noValidate=false;$('#modalSubmit').hidden=false;
 $('#modalTitle').textContent='Transport for new listeners';
 $('#modalFields').innerHTML='<p>'+esc(data.scope)+'</p><label>Transport<select name="transport_network"><option value="tcp">TCP Reality</option><option value="xhttp">XHTTP Reality — stream-up</option></select></label><div id="xhttpFields"><label>XHTTP Host<input name="xhttp_host" readonly value="'+esc(current.host||preset.host)+'"></label><label>Path length in bytes<input type="number" name="path_bytes" min="256" max="'+data.max_path_bytes+'" value="'+esc(current.path_bytes||preset.path_bytes)+'"></label><p class="muted">City, country and service segments are used without repetition. Every new listener receives a saved unique path. '+data.max_path_bytes+' bytes is the tested application limit, not a universal protocol maximum.</p><label>XHTTP Extra<textarea readonly rows="9">'+esc(JSON.stringify(preset.extra,null,2))+'</textarea></label><label>Requested FinalMask preset<textarea readonly rows="10">'+esc(JSON.stringify(preset.finalmask,null,2))+'</textarea></label><p class="muted">'+esc(data.finalmask_status)+'</p><label>Browser Dialer<select disabled><option>Disabled — WebView cannot use stream-up Reality</option></select></label><label>Flow<select disabled><option>Empty — Vision cannot use this preset</option></select></label><p class="muted">SNI continues to use the listener’s existing SNI selection policy. XHTTP Host does not change SNI.</p></div>';
 const select=$('#modalFields [name=transport_network]');
 select.value=current.network||'tcp';const sync=()=>{$('#xhttpFields').hidden=select.value!=='xhttp'};select.onchange=sync;sync();
 $('#modalError').textContent='';$('#modal').showModal();
 $('#modalForm').onsubmit=async e=>{
  e.preventDefault();
  let config={network:'tcp',finalmask_enabled:false};
  if(select.value==='xhttp'){
   const bytes=Number($('#modalFields [name=path_bytes]').value);
   if(!Number.isInteger(bytes)||bytes<256||bytes>data.max_path_bytes){$('#modalError').textContent='Path length must be a whole number from 256 to '+data.max_path_bytes+' bytes.';return}
   config={...preset,host:$('#modalFields [name=xhttp_host]').value.trim(),path_bytes:bytes};
  }
  try{await api('/api/v1/configs/transport',{method:'PUT',body:JSON.stringify({revision:policy.revision,config})});$('#modal').close();await refreshConfigTransport()}
  catch(err){$('#modalError').textContent=err.message}
 };
}
