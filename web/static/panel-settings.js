/* Panel settings and durable project updates. No credentials are returned by GET. */
(function(){
 let starting=false,lastState=null;
 const stages=['Check prerequisites','Fetch release','Install dependencies','Build release','Back up and preserve data','Verify services and panel','Complete'];
 const labels={idle:'Ready to update',queued:'Update queued',running:'Updating project',succeeded:'Last update completed',failed:'Update failed',interrupted:'Update interrupted',unavailable:'Update service unavailable'};
 function settingsFields(d){
  return '<label>Listen port<input name="listen_port" type="number" min="1024" max="65535" required value="'+esc(d.listen_port)+'"></label>'+
   '<label>Web path<input name="web_path" required value="'+esc(d.web_path)+'"></label>'+
   '<label>Username<input name="username" autocomplete="username" required value="'+esc(d.username)+'"></label>'+
   '<label>New password<input name="password" type="password" autocomplete="new-password" minlength="12" maxlength="72" placeholder="Leave blank to keep current password"></label>'+
   '<label id="currentCredentialLabel" hidden>Current password<input name="current_password" type="password" autocomplete="current-password"></label>';
 }
 window.renderPanelSettings=function(d){
  return '<div class="card section"><p>Listen port: '+esc(d.listen_port)+'</p><p>Web path: '+esc(d.web_path)+'</p><p>Username: '+esc(d.username)+'</p><button data-action="edit-settings">Edit settings</button><p class="muted">Port and path changes restart the panel automatically. Leave the password blank to keep it.</p><div id="settingsApplyNotice"></div></div>'+
   '<div class="card section"><h3>Project update</h3><p class="muted">Update from the published release. Existing accounts, proxies, servers and settings are preserved.</p><div id="projectUpdateStatus" aria-live="polite"><p>Checking update service…</p></div><button data-action="project-update" id="projectUpdateButton" disabled>Update project</button></div>';
 };
 window.openPanelSettingsForm=function(){
  const d=cache.settings;if(!d)return;
  $('#modalTitle').textContent='Panel Settings';$('#modalError').textContent='';
  $('#modalFields').innerHTML=settingsFields(d);$('#modalSubmit').hidden=false;$('#modalSubmit').textContent='Save and apply';
  const form=$('#modalForm');
  const changed=()=>{const needed=form.elements.username.value!==d.username||form.elements.password.value!=='';
   $('#currentCredentialLabel').hidden=!needed;form.elements.current_password.required=needed;};
  form.elements.username.oninput=changed;form.elements.password.oninput=changed;
  $('#modal').showModal();
 };
 window.savePanelSettingsForm=async function(o){
  const button=$('#modalSubmit');button.disabled=true;
  try{
   o.listen_port=Number(o.listen_port);
   const result=await api('/api/v1/panel-settings',{method:'PUT',body:JSON.stringify(o)});
   $('#modalForm').reset();$('#modal').close();
   cache.settings=result.settings;
   if(result.credentials_changed){logout();$('#username').value=result.settings.username;$('#loginError').textContent='Login details saved. Sign in again.';}
   else await load('settings');
   if(result.restart_required){
    const target=new URL(location.href);target.port=String(result.settings.listen_port);target.pathname=result.settings.web_path;target.hash='settings';target.search='';
    if(result.restart_queued){setTimeout(()=>location.assign(target.href),3500);}
    else{
     const slot=$('#settingsApplyNotice');
     if(slot)slot.innerHTML='<p class="error">Settings saved. The restart could not be queued.</p><button data-action="apply-panel-settings">Apply saved settings</button><p><a href="'+esc(target.href)+'">Open saved panel address</a></p>';
     else $('#loginError').textContent='Login details saved. Sign in again to apply the saved panel address.';
    }
   }
  }finally{button.disabled=false;}
 };
 function showState(s){
  lastState=s;
  const slot=$('#projectUpdateStatus'),button=$('#projectUpdateButton');if(!slot||!button)return;
  const label=labels[s.status]||'Checking update';
  let body='<p><b>'+esc(label)+'</b></p>';
  if(s.current_revision)body+='<p>Currently running revision: '+esc(s.current_revision.slice(0,12))+'</p>';
  if(s.installed_revision)body+='<p class="muted">Last update installed revision: '+esc(s.installed_revision.slice(0,12))+'</p>';
  if(s.ended_at)body+='<p class="muted">Last update finished: '+esc(new Date(s.ended_at).toLocaleString())+'</p>';
  if(s.job_id){
   body+='<ol>'+stages.map((label,index)=>{
    const n=index+1,done=s.status==='succeeded'||n<s.stage,failed=(s.status==='failed'||s.status==='interrupted')&&n===s.stage;
    return '<li class="'+(done?'good':failed?'error':'')+'">'+esc(label)+' — '+(done?'Done':failed?'Failed':n===s.stage?'Current':'Waiting')+'</li>';
   }).join('')+'</ol>';
  }
  if(s.recovery_required)body+='<p class="error">Recovery is required before another update. The saved backup has been retained.</p>';
  else if(s.status==='failed'||s.status==='interrupted')body+='<p class="error">Update did not complete. Check the result before trying again.</p>';
  slot.innerHTML=body;button.disabled=starting||!s.available||s.busy||s.recovery_required;
 }
 window.refreshProjectUpdate=function(){
  if(current!=='settings'||!token||!$('#projectUpdateStatus'))return;
  return pollPageView('project-update','settings',['#projectUpdateStatus'],async read=>read('/api/v1/project-update'),s=>showState(s));
 };
 document.addEventListener('click',async e=>{
  const button=e.target.closest('[data-action="project-update"],[data-action="apply-panel-settings"]');if(!button)return;
  e.preventDefault();e.stopPropagation();
  if(button.dataset.action==='apply-panel-settings'){
   button.disabled=true;
   try{await api('/api/v1/panel-settings/apply',{method:'POST',body:'{}'});const target=new URL(location.href);target.port=String(cache.settings.listen_port);target.pathname=cache.settings.web_path;setTimeout(()=>location.assign(target.href),3500);}
   catch(error){toast(error.message,'error');button.disabled=false;}
   return;
  }
  if(starting||button.disabled)return;
  starting=true;button.disabled=true;
  try{showState(await api('/api/v1/project-update',{method:'POST',body:'{}'}));}
  catch(error){const slot=$('#projectUpdateStatus');if(slot)slot.innerHTML+='<p class="error">The request result is not confirmed: '+esc(error.message)+'. Checking update status…</p>';}
  finally{starting=false;refreshProjectUpdate();}
 },true);
 setInterval(refreshProjectUpdate,2000);
})();
