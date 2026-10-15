function transportDialerStatus(c){
 return c.browser_dialer ? 'Requested '+c.browser_dialer+' · inactive with REALITY' : 'Disabled';
}
function transportMaskStatus(c){
 return c.finalmask_enabled ? 'TCP Fragment · '+(c.finalmask_scope||'both')+' · new listeners' : 'Disabled';
}
async function refreshConfigTransport(){
 const slot=document.querySelector('#configTransportSlot');if(!slot)return;
 try{
  const data=await api('/api/v1/configs/transport');if(!slot.isConnected)return;
  const c=data.policy.config,network=c.network||'tcp';
  const row=(name,value)=>'<span>'+esc(name)+'</span><span>'+esc(value)+'</span>';
  let rows=row('Network',network==='xhttp'?'XHTTP Reality · '+c.mode:'TCP Reality');
  if(network==='xhttp')rows+=row('XHTTP Host',c.host)+row('Path length',c.path_bytes+' bytes · unique per listener')+row('Flow',c.flow||'Empty')+row('VLESS Encryption',c.flow?'Enabled for Vision':'None')+row('Browser Dialer',transportDialerStatus(c))+row('FinalMask',transportMaskStatus(c));
  slot.innerHTML='<div class="card section"><div class="cardHeader"><h3>Transport for new listeners</h3><button class="small ghost" id="editTransport">Edit transport</button></div><div class="kv">'+rows+'</div><p class="muted">'+esc(data.scope)+'</p></div>';
  document.querySelector('#editTransport').onclick=()=>openConfigTransport(data);
 }catch(e){
  if(slot.isConnected)slot.innerHTML='<div class="card section">Transport settings unavailable. <button class="small ghost" id="retryTransport">Retry</button></div>';
  document.querySelector('#retryTransport')?.addEventListener('click',refreshConfigTransport);
 }
}
function openConfigTransport(data){
 const policy=data.policy,preset=data.preset,current=policy.config;
 const c=current.network==='xhttp'?structuredClone(current):structuredClone(preset);
 $('#modalForm').noValidate=false;$('#modalSubmit').hidden=false;
 $('#modalTitle').textContent='Transport for new listeners';
 $('#modalFields').innerHTML='<p>'+esc(data.scope)+'</p>'+
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
 '<label>Browser Dialer preference<select name="browser_dialer"><option value="">Disabled</option><option value="OkHttp">OkHttp</option><option value="WebView">WebView</option></select></label>'+
 '<p class="muted" id="browserDialerNotice"></p>'+
 '<p class="muted">XHTTP Host does not change SNI. SNI follows the panel’s existing SNI selection policy. Changing defaults applies to new listeners; existing listeners keep their saved settings.</p></div>';
 const field=name=>$('#modalFields [name="'+name+'"]');
 field('transport_network').value=current.network||'tcp';field('xhttp_mode').value=c.mode||'stream-up';
 field('transport_flow').value=c.flow||'';field('finalmask_enabled').value=String(!!c.finalmask_enabled);
 field('finalmask_scope').value=c.finalmask_scope||'both';field('browser_dialer').value=c.browser_dialer||'';
 const sync=()=>{
  const visible=field('transport_network').value==='xhttp';$('#xhttpFields').hidden=!visible;
  $('#xhttpFields').querySelectorAll('input,select,textarea,button').forEach(x=>{x.disabled=!visible});
  $('#browserDialerNotice').textContent=field('browser_dialer').value ? 'This saves your preference. It is not active with REALITY: Browser Dialer needs a TLS endpoint and must be enabled in the client app. A VLESS link cannot turn on this app setting.' : 'Native dialing. Browser Dialer can be selected as a preference, but REALITY cannot use the browser connection.';
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
  try{await api('/api/v1/configs/transport',{method:'PUT',body:JSON.stringify({revision:policy.revision,config})});$('#modal').close();await refreshConfigTransport()}
  catch(err){$('#modalError').textContent=err.message}
  finally{$('#modalSubmit').disabled=false}
 };
}
