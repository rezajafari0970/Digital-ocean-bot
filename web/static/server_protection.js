/* Server protection: desired state is never presented as verified deployment. */
let protectionData=null,protectionBusy=false,protectionReading=false,protectionFormRevision=0,protectionDirty=false;
function protectionRequestID(){const b=new Uint8Array(16);crypto.getRandomValues(b);b[6]=(b[6]&15)|64;b[8]=(b[8]&63)|128;const s=Array.from(b,x=>x.toString(16).padStart(2,'0')).join('');return s.slice(0,8)+'-'+s.slice(8,12)+'-'+s.slice(12,16)+'-'+s.slice(16,20)+'-'+s.slice(20)}
function protectionPending(){try{return JSON.parse(sessionStorage.getItem('dob.protection.pending')||'null')}catch{return null}}
function protectionShell(){
 return '<div class="card section"><h3>Server protection</h3><p class="muted">Monitor resource pressure locally, pause new VLESS connections during critical pressure, and recover a stopped x-ui with bounded retries.</p>'+
 '<label>Coverage<select id="protectionScope"><option value="fleet">All current and future servers</option><option value="selected">Selected servers</option></select></label>'+
 '<div id="protectionSelection" hidden></div><div class="actions" style="flex-wrap:wrap"><button type="button" id="protectionEnable">Enable protection</button><button type="button" id="protectionDisable" class="danger">Disable protection</button><button type="button" id="protectionRetry" class="ghost" hidden>Retry pending request</button></div>'+
 '<div id="protectionStatus" role="status" aria-live="polite">Loading protection status…</div>'+
 '<p class="muted">Local resource sampling: 250 ms. Fleet verification is slower. Existing connections remain open; changing an active connection to another server requires app support. Automatic app failover and per-user admission are not included.</p>'+
 '<details id="protectionDetails"><summary>Server results</summary><div id="protectionResults"></div></details></div>';
}
function renderProtection(d){
 const c=d.control,panels=d.panels||[],assigned=panels.filter(p=>p.assigned);
 const on=assigned.filter(p=>p.desired_enabled),verified=on.filter(p=>p.verified);
 const cleanup=assigned.filter(p=>(c.enabled?!p.desired_enabled&&!p.verified:p.desired_enabled||!p.verified));
 const bad=assigned.filter(p=>['ERROR','UNSUPPORTED'].includes(p.state));
 const status=document.getElementById('protectionStatus');
 if(!status)return;
 status.innerHTML='<p><strong>'+(c.enabled?'ON requested':'OFF requested')+'</strong> · Revision '+esc(c.revision)+'</p><p>'+
 (c.enabled?verified.length+' / '+on.length+' enabled servers freshly verified':(cleanup.length?'Disabling: '+cleanup.length+' server(s) awaiting verified cleanup.':'Protection disabled; assigned server cleanup verified.'))+
 (bad.length?' · '+bad.length+' need attention':'')+'</p><p class="muted">'+(d.inherits_future?'New eligible servers inherit this policy automatically.':'Future servers do not inherit protection.')+'</p>';
 document.getElementById('protectionResults').innerHTML=assigned.length?assigned.map(p=>{
 const st=p.status||{},m=st.metrics||{};
 return '<div class="card section"><strong>'+esc(p.label)+'</strong><p>'+esc(p.state)+' · '+(p.verified?'verified':'pending / unverified')+' · '+esc(st.state||'unknown')+'</p>'+
 '<p>'+esc(p.error||st.reason||'Waiting for a server receipt')+'</p>'+
 '<p class="muted">RAM available: '+esc(Math.round(m.mem_available_mb||0))+' MiB · CPU: '+esc(Math.round(m.cpu_percent||0))+'% · New connections: '+(!p.verified?'unknown (unverified)':(st.admission_blocked?'paused':'not paused'))+'</p>'+
 '<p class="muted">Last receipt: '+esc(p.checked_at||'never')+'</p></div>';
 }).join(''):'<p class="muted">No servers assigned yet.</p>';
 document.getElementById('protectionEnable').disabled=protectionBusy;document.getElementById('protectionEnable').textContent=c.enabled?'Apply coverage':'Enable protection';
 document.getElementById('protectionDisable').disabled=protectionBusy||(!c.enabled&&!cleanup.length&&!protectionPending());
 document.getElementById('protectionRetry').hidden=!protectionPending();
 document.getElementById('protectionRetry').disabled=protectionBusy;
}
async function refreshServerProtection(){
 const slot=document.getElementById('serverProtectionSlot');
 if(!slot||protectionReading||document.hidden)return;
 protectionReading=true;
 try{
  const d=await api('/api/v1/server-protection');if(!slot.isConnected)return;
  const first=!document.getElementById('protectionStatus');
  protectionData=d;
  if(first){
   slot.innerHTML=protectionShell();
   slot.addEventListener('change',e=>{if(e.target.id==='protectionScope'||e.target.name==='protectionPanel'){protectionDirty=true;document.getElementById('protectionSelection').hidden=document.getElementById('protectionScope').value!=='selected'}});
   document.getElementById('protectionEnable').onclick=()=>changeServerProtection(true);
   document.getElementById('protectionDisable').onclick=()=>changeServerProtection(false);
   document.getElementById('protectionRetry').onclick=()=>changeServerProtection(null);
  }
  if(!protectionDirty){
   protectionFormRevision=d.control.revision;
   document.getElementById('protectionScope').value=d.control.scope;
   document.getElementById('protectionSelection').hidden=d.control.scope!=='selected';
   document.getElementById('protectionSelection').innerHTML=(d.panels||[]).filter(p=>p.eligible).map(p=>'<label><input type="checkbox" name="protectionPanel" value="'+esc(p.id)+'" '+(d.control.panel_ids.includes(p.id)?'checked':'')+'> '+esc(p.label)+'</label>').join('')||'<p>No eligible servers.</p>';
  }
  renderProtection(d);
 }catch(e){if(slot.isConnected){const s=document.getElementById('protectionStatus');if(s)s.textContent='Protection status unavailable: '+e.message;else slot.innerHTML='<div class="card section error">Protection status unavailable: '+esc(e.message)+'</div>'}}
 finally{protectionReading=false}
}
async function changeServerProtection(enabled){
 if(protectionBusy)return;protectionBusy=true;
 try{
  let q;
  if(enabled===null){q=protectionPending();if(!q)return}
  else{
   const latest=protectionData;
   const scope=enabled?document.getElementById('protectionScope').value:latest.control.scope;
   const panels=!enabled?latest.control.panel_ids:(scope==='selected'?Array.from(document.querySelectorAll('input[name="protectionPanel"]:checked'),x=>x.value):[]);
   if(enabled&&scope==='selected'&&!panels.length)throw Error('Select at least one server.');
   q={request_id:protectionRequestID(),expected_revision:enabled?protectionFormRevision:latest.control.revision,enabled,scope,panel_ids:panels};
   sessionStorage.setItem('dob.protection.pending',JSON.stringify(q));
  }
  const result=await api('/api/v1/server-protection',{method:'POST',body:JSON.stringify(q)});
  sessionStorage.removeItem('dob.protection.pending');protectionDirty=false;
  toast('Protection '+(result.enabled?'enable':'disable')+' requested. Waiting for server verification.');
 }catch(e){if(/protection_conflict|policy changed/.test(e.message)){sessionStorage.removeItem('dob.protection.pending');protectionDirty=false}toast('Protection request: '+e.message,'error')}
 finally{protectionBusy=false;await refreshServerProtection()}
}
setInterval(()=>{if(typeof current!=='undefined'&&current==='configs')refreshServerProtection()},5000);
