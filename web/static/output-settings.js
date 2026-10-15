async function refreshOutputSettings(){
 const slot=document.querySelector('#outputSettingsSlot');if(!slot)return;
 try{
  const policy=await api('/api/v1/configs/output-settings');if(!slot.isConnected)return;
  slot.innerHTML='<div class="card section"><div class="cardHeader"><h3>Client output fields</h3><button class="small ghost" id="editOutputSettings">Edit output fields</button></div><div class="kv"><span>XHTTP Extra</span><span>'+ (policy.include_xhttp_extra?'Included':'Omitted')+'</span><span>FinalMask</span><span>'+(policy.include_finalmask?'Included when configured for clients':'Omitted')+'</span><span>Browser Dialer</span><span>Not included · unavailable with REALITY</span></div><p class="muted">Applies to all output links and subscriptions. Listener and core settings stay saved. Refresh the subscription in your client after saving.</p></div>';
  document.querySelector('#editOutputSettings').onclick=()=>openOutputSettings(policy);
 }catch(err){
  if(slot.isConnected){slot.innerHTML='<div class="card section"><p>Output settings unavailable.</p><button class="small ghost" id="retryOutputSettings">Retry</button></div>';slot.querySelector('button').onclick=refreshOutputSettings}
 }
}
function openOutputSettings(policy){
 $('#modalTitle').textContent='Client output fields';$('#modalForm').noValidate=false;$('#modalSubmit').hidden=false;
 $('#modalFields').innerHTML='<p>Select the optional fields to include in client links. These choices apply immediately to all output and subscription URLs.</p><label class="remember"><input type="checkbox" name="include_xhttp_extra"> Include XHTTP Extra (JSON)</label><label class="remember"><input type="checkbox" name="include_finalmask"> Include FinalMask when configured for clients</label><p class="muted">The complete Extra and FinalMask settings remain saved in Listener transport settings. Server settings are preserved.</p><p class="muted">Browser Dialer needs a browser on the device making the connection. It cannot be activated for a REALITY client by a server-only setting and is not included in these links.</p><p class="muted">Server address, port, credentials, security, transport, path, Host and Flow remain in the link so it matches the listener.</p>';
 const field=n=>$('#modalFields [name="'+n+'"]');
 field('include_xhttp_extra').checked=policy.include_xhttp_extra;field('include_finalmask').checked=policy.include_finalmask;
 $('#modalError').textContent='';$('#modal').showModal();
 $('#modalForm').onsubmit=async event=>{
  event.preventDefault();$('#modalSubmit').disabled=true;
  try{
   await api('/api/v1/configs/output-settings',{method:'PUT',body:JSON.stringify({revision:policy.revision,include_xhttp_extra:field('include_xhttp_extra').checked,include_finalmask:field('include_finalmask').checked})});
   $('#modal').close();await refreshOutputSettings();
  }catch(err){$('#modalError').textContent=err.message}
  finally{$('#modalSubmit').disabled=false}
 };
}
