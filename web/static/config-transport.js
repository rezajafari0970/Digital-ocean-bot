function transportDialerStatus(c){
 return 'Unavailable with REALITY · not enabled in exported links';
}
function transportMaskStatus(c){
 return c.finalmask_enabled ? 'TCP Fragment · '+(c.finalmask_scope||'both') : 'Disabled';
}
async function refreshConfigTransport(){
 const slot=document.querySelector('#configTransportSlot');if(!slot)return;
 try{
  const data=await api('/api/v1/configs/transport');if(!slot.isConnected)return;
  const c=data.policy.config,network=c.network||'tcp';
  const row=(name,value)=>'<span>'+esc(name)+'</span><span>'+esc(value)+'</span>';
  let rows=row('Network',network==='xhttp'?'XHTTP Reality · '+c.mode:'TCP Reality');
  if(network==='xhttp')rows+=row('XHTTP Host',c.host)+row('Path length',c.path_bytes+' bytes · unique per listener')+row('Flow',c.flow||'Empty')+row('VLESS Encryption',c.flow?'Enabled for Vision':'None')+row('Browser Dialer',transportDialerStatus(c))+row('FinalMask',transportMaskStatus(c));
  slot.innerHTML='<div class="card section"><div class="cardHeader"><h3>Listener transport settings</h3><button class="small ghost" id="editTransport">Edit transport</button></div><div class="kv">'+rows+'</div><p class="muted">'+esc(data.scope)+'</p></div>';
  document.querySelector('#editTransport').onclick=()=>openConfigTransport(data);
  const outputArea=document.createElement('div');outputArea.id='outputSettingsSlot';slot.appendChild(outputArea);
  if(typeof refreshOutputSettings==='function')await refreshOutputSettings();
  const jobs=data.applications||[],counts={};
  jobs.forEach(j=>counts[j.state]=(counts[j.state]||0)+1);
  const stateNames={PENDING:'Queued',RUNNING:'Applying / verifying',SUCCEEDED:'Verified',BLOCKED:'Needs review',FAILED:'Failed',OBSOLETE:'Server retired'};
  const status=Object.entries(counts).map(([k,v])=>esc(stateNames[k]||k)+': '+v).join(' · ')||'No existing-listener application requested';
  const area=document.createElement('div');area.className='card section';
  area.innerHTML='<h3>Reality transport change history</h3><p>'+status+'</p><p class="muted">Saving Reality transport queues its application automatically. This history includes completed changes and retired servers; it is not the current fleet health. UUIDs, quotas and expiry are preserved.</p><button class="small" id="applyExistingTransport">Reconcile saved transport now</button><button class="small ghost" id="refreshTransportStatus">Refresh status</button><div id="transportApplyError"></div>'+
   jobs.slice(0,80).map(j=>'<div class="kv"><span>'+esc(j.panel_id.slice(0,8))+' · '+esc(j.managed_key)+'</span><span>'+esc(stateNames[j.state]||j.state)+' '+esc(j.error||'')+((j.state==='SUCCEEDED'||j.state==='BLOCKED')?'<button type="button" class="small ghost" data-transport-rollback="'+esc(j.id)+'">Restore previous transport</button>':'')+(j.state==='BLOCKED'?'<button type="button" class="small ghost" data-transport-retry="'+esc(j.id)+'">Recheck and retry</button>':'')+'</span></div>').join('');
  slot.appendChild(area);
  document.querySelector('#refreshTransportStatus').onclick=refreshConfigTransport;
  document.querySelector('#applyExistingTransport').onclick=async ev=>{
   ev.currentTarget.disabled=true;
   try{await api('/api/v1/configs/transport/apply',{method:'POST',body:JSON.stringify({revision:data.policy.revision})});await refreshConfigTransport()}
   catch(err){document.querySelector('#transportApplyError').textContent=err.message;ev.target.disabled=false}
  };
  area.querySelectorAll('[data-transport-retry]').forEach(btn=>btn.onclick=async()=>{
   btn.disabled=true;
   try{await api('/api/v1/configs/transport/retry/'+btn.dataset.transportRetry,{method:'POST',body:'{}'});await refreshConfigTransport()}
   catch(err){document.querySelector('#transportApplyError').textContent=err.message;btn.disabled=false}
  });
  area.querySelectorAll('[data-transport-rollback]').forEach(btn=>btn.onclick=async()=>{
   btn.disabled=true;
   try{await api('/api/v1/configs/transport/rollback/'+btn.dataset.transportRollback,{method:'POST',body:'{}'});await refreshConfigTransport()}
   catch(err){document.querySelector('#transportApplyError').textContent=err.message;btn.disabled=false}
  });
 }catch(e){
  if(slot.isConnected)slot.innerHTML='<div class="card section">Transport settings unavailable. <button class="small ghost" id="retryTransport">Retry</button></div>';
  document.querySelector('#retryTransport')?.addEventListener('click',refreshConfigTransport);
 }
}
function openConfigTransport(data){
 const policy=data.policy,preset=data.preset,current=policy.config;
 const c=current.network==='xhttp'?structuredClone(current):structuredClone(preset);
 $('#modalForm').noValidate=false;$('#modalSubmit').hidden=false;
 $('#modalTitle').textContent='Listener transport settings';
 $('#modalFields').innerHTML='<p>'+esc(data.scope)+'</p>'+
 '<input type="checkbox" name="apply_existing" checked hidden><p class="muted">Changes apply automatically to current and future managed Reality listeners.</p>'+
 '<label>Transport<select name="transport_network"><option value="tcp">TCP Reality</option><option value="xhttp">XHTTP Reality</option></select></label>'+
 '<div id="xhttpFields">'+
 '<label>XHTTP Mode<select name="xhttp_mode"><option value="stream-up">stream-up</option><option value="packet-up">packet-up</option><option value="stream-one">stream-one</option><option value="auto">auto</option></select></label>'+
 '<label>XHTTP Host<input name="xhttp_host" autocomplete="off" maxlength="253" value="'+esc(c.host)+'"></label>'+
 '<label>Path length in bytes<input type="number" name="path_bytes" min="256" max="'+data.max_path_bytes+'" value="'+esc(c.path_bytes)+'"></label>'+
 '<p class="muted">Each new listener gets a saved unique path. Direct and Residential clients on that listener share it.</p>'+
 '<label>Flow<select name="transport_flow"><option value="xtls-rprx-vision-udp443">xtls-rprx-vision-udp443</option><option value="xtls-rprx-vision">xtls-rprx-vision</option><option value="">Empty</option></select></label>'+
 '<p class="muted">Vision on XHTTP enables VLESS Encryption with a separate per-listener authentication key. The udp443 option permits UDP port 443 in the client.</p>'+
 '<label>XHTTP Extra (JSON)<textarea name="xhttp_extra" rows="9" spellcheck="false">'+esc(JSON.stringify(c.extra||{},null,2))+'</textarea></label>'+
 '<label>FinalMask<select name="finalmask_enabled"><option value="true">Enabled</option><option value="false">Disabled</option></select></label>'+
 '<label>Apply TCP mask to<select name="finalmask_scope"><option value="both">Server and client</option><option value="client">Client only</option><option value="server">Server only</option></select></label>'+
 '<label>FinalMask (JSON)<textarea name="finalmask_json" rows="12" spellcheck="false">'+esc(JSON.stringify(c.finalmask||preset.finalmask,null,2))+'</textarea></label>'+
 '<button type="button" class="small ghost" id="useFragmentPreset">Use tested TCP Fragment preset</button>'+
 '<p class="muted">'+esc(data.finalmask_status)+'</p>'+
 '<label>Browser Dialer<select name="browser_dialer" disabled><option value="">Unavailable with REALITY</option><option value="OkHttp">Saved OkHttp preference — inactive</option><option value="WebView">Saved WebView preference — inactive</option></select></label>'+
 '<p class="muted" id="browserDialerNotice"></p>'+
 '<p class="muted">XHTTP Host does not change SNI. SNI follows the panel’s existing SNI selection policy. Applying to existing listeners changes their saved settings after native verification.</p></div>';
 const field=name=>$('#modalFields [name="'+name+'"]');
 field('transport_network').value=current.network||'tcp';field('xhttp_mode').value=c.mode||'stream-up';
 field('transport_flow').value=c.flow||'';field('finalmask_enabled').value=String(!!c.finalmask_enabled);
 field('finalmask_scope').value=c.finalmask_scope||'both';field('browser_dialer').value=c.browser_dialer||'';
 const sync=()=>{
  const visible=field('transport_network').value==='xhttp';$('#xhttpFields').hidden=!visible;
  $('#xhttpFields').querySelectorAll('input,select,textarea,button').forEach(x=>{x.disabled=!visible||x.name==='browser_dialer'});
  $('#browserDialerNotice').textContent='Browser Dialer is not active or exported for REALITY. Enabling it requires a compatible client transport; changing only this server setting cannot enable it.';
 };
 field('transport_network').onchange=sync;field('browser_dialer').onchange=sync;sync();
 $('#useFragmentPreset').onclick=()=>{field('finalmask_json').value=JSON.stringify(preset.finalmask,null,2)};
 $('#modalError').textContent='';$('#modal').showModal();
 $('#modalForm').onsubmit=async e=>{
  e.preventDefault();let config={network:'tcp',finalmask_enabled:false};
  if(field('transport_network').value==='xhttp'){
   const bytes=Number(field('path_bytes').value);
   if(!Number.isInteger(bytes)||bytes<256||bytes>data.max_path_bytes){$('#modalError').textContent='Path length must be a whole number from 256 to '+data.max_path_bytes+' bytes.';return}
   let extra,mask;
   try{extra=JSON.parse(field('xhttp_extra').value);mask=JSON.parse(field('finalmask_json').value)}catch(err){$('#modalError').textContent='Extra and FinalMask must contain valid JSON: '+err.message;return}
   if(!extra||typeof extra!=='object'||Array.isArray(extra)||!mask||typeof mask!=='object'||Array.isArray(mask)){$('#modalError').textContent='Extra and FinalMask must be JSON objects.';return}
   config={...c,network:'xhttp',mode:field('xhttp_mode').value,host:field('xhttp_host').value.trim(),path_bytes:bytes,extra,finalmask:mask,finalmask_enabled:field('finalmask_enabled').value==='true',finalmask_scope:field('finalmask_scope').value,flow:field('transport_flow').value,browser_dialer:field('browser_dialer').value};
   delete config.path;
  }
  $('#modalSubmit').disabled=true;
  try{await api('/api/v1/configs/transport',{method:'PUT',body:JSON.stringify({revision:policy.revision,config,apply_existing:field('apply_existing').checked})});$('#modal').close();await refreshConfigTransport()}
  catch(err){$('#modalError').textContent=err.message}
  finally{$('#modalSubmit').disabled=false}
 };
}
