/* Residential selection and credential exports live only in memory. */
const residentialSelection = new Set();
function residentialRows(text, prefix, existing=[]) {
  const names=new Set(existing.map(x=>x.toLowerCase()));let number=1;
  if(!prefix||prefix.trim()!==prefix||prefix.length>70||/[\r\n\x00-\x1f]/.test(prefix))throw Error('Enter a name prefix (1–70 characters, exact case is preserved).');
  const lines=text.split(/\r?\n/).filter(x=>x.trim()!=='');
  if(!lines.length||lines.length>100)throw Error('Paste 1 to 100 SOCKS proxies per batch.');
  return lines.map((line,i)=>{
    const match=line.match(/^(\[[^\]]+\]|[^:]+):(\d+):([^:]*):(.*)$/);
    if(!match||Number(match[2])<1||Number(match[2])>65535||/[\r\n\x00]/.test(line))throw Error(`Line ${i+1}: expected host:port:user:password.`);
    while(names.has((prefix+number).toLowerCase()))number++;
    const name=prefix+number++;names.add(name.toLowerCase());return {name,line,endpoint:match[1]+':'+match[2]};
  });
}
function residentialUUIDValue(){
  if(crypto.randomUUID)return crypto.randomUUID();
  const b=crypto.getRandomValues(new Uint8Array(16));b[6]=(b[6]&15)|64;b[8]=(b[8]&63)|128;
  const h=[...b].map(x=>x.toString(16).padStart(2,'0')).join('');return `${h.slice(0,8)}-${h.slice(8,12)}-${h.slice(12,16)}-${h.slice(16,20)}-${h.slice(20)}`;
}
function renderResidential(rows){
  const ids=new Set(rows.map(x=>x.id));for(const id of residentialSelection)if(!ids.has(id))residentialSelection.delete(id);
  return `<div class="card section"><h3>Residential Proxies</h3><div id="residentialRoutingStatus" class="muted">Checking server routing…</div>
  <div class="residentialToolbar"><button type="button" data-residential="import">Bulk add SOCKS</button><button type="button" class="ghost" data-residential="copy-all" ${rows.length?'':'disabled'}>Copy all</button><button type="button" class="ghost" data-residential="copy-selected" ${residentialSelection.size?'':'disabled'}>Copy selected</button><button type="button" class="danger" data-residential="delete-selected" ${residentialSelection.size?'':'disabled'}>Delete selected</button><button type="button" class="danger" data-residential="delete-all" ${rows.length?'':'disabled'}>Delete all</button></div>
  <label class="remember"><input type="checkbox" data-residential="select-all" ${rows.length&&residentialSelection.size===rows.length?'checked':''}><span>Select all · <span id="residentialSelectionCount">${residentialSelection.size}</span> / ${rows.length}</span></label>
  <details class="residentialPolicy"><summary>Health &amp; routing details</summary>  <p class="muted">Only Google Ads and the temporary BrowserLeaks test domain use residential routing. DNS and other traffic remain direct. Each server checks its own proxy paths: about 80% of new connections use the fastest healthy group and 20% explore all healthy proxies. Unavailable or untested paths are blocked until a successful check; TCP checks do not certify provider UDP support.</p>
  <p class="muted">Health below is the control server's latest TCP observation. Each serving server independently excludes failed paths without restarting Xray. Response time is not a bandwidth measurement.</p></details></div>
  <div id="residentialCards">${rows.length?rows.map(x=>`<div class="residentialItem"><label class="remember residentialSelect"><input type="checkbox" data-residential-id="${esc(x.id)}" ${residentialSelection.has(x.id)?'checked':''}><span>Select ${esc(x.name)}</span></label>${residentialCard(x)}<p class="muted residentialObservation">Checked: ${x.last_checked_at?esc(new Date(x.last_checked_at).toLocaleString()):'pending'} · Success rate: ${x.check_count?Math.round(x.success_ewma*100)+'%':'pending'} · Average response: ${x.latency_ewma_ms?Math.round(x.latency_ewma_ms)+' ms':'pending'}${x.last_error?' · '+esc(x.last_error):''}</p></div>`).join(''):'<div class="card empty">No residential proxies. Use Bulk add SOCKS or New.</div>'}</div>`;
}
function refreshResidentialSelection(){
  const n=residentialSelection.size;const label=document.querySelector('#residentialSelectionCount');if(label)label.textContent=n;
  for(const action of ['copy-selected','delete-selected']){const b=document.querySelector(`[data-residential="${action}"]`);if(b)b.disabled=!n;}
  const all=document.querySelector('[data-residential="select-all"]');if(all){all.checked=n>0&&n===(cache.residential||[]).length;all.indeterminate=n>0&&!all.checked;}
}
function residentialDialog(title,html){
  document.querySelector('#residentialDialog')?.remove();
  const d=document.createElement('dialog');d.id='residentialDialog';d.innerHTML=`<form method="dialog" class="modal residentialModal"><div class="modalHead"><h3>${esc(title)}</h3><button class="ghost" type="button" data-close>×</button></div><div data-fields>${html}</div><p class="error" role="alert" data-error></p><div class="actions"><button type="button" class="ghost" data-close>Cancel</button><button type="button" data-submit>Preview names</button></div></form>`;
  document.body.appendChild(d);d.querySelectorAll('[data-close]').forEach(b=>b.onclick=()=>d.close());d.addEventListener('close',()=>d.remove(),{once:true});d.showModal();return d;
}
function openResidentialBulk(){
  const d=residentialDialog('Bulk add SOCKS',`<label>Name prefix<input name="prefix" value="ads" maxlength="70" autocomplete="off"></label><p class="muted">Exact spelling is preserved. Unused numbers are assigned automatically; edit each name in the preview.</p><label>SOCKS proxies<textarea name="proxies" rows="9" spellcheck="false" autocomplete="off" autocapitalize="off" placeholder="107.151.196.26:3297:user:pass&#10;107.151.196.26:3206:user:pass"></textarea></label><p class="muted">One host:port:user:password per line. Empty lines are ignored. Passwords may contain colons. No proxies are saved until you confirm the preview.</p><div data-preview></div>`);
  let rows=null,requestID=null,submitted=null,busy=false;const submit=d.querySelector('[data-submit]'),error=d.querySelector('[data-error]');
  const invalidate=()=>{if(busy)return;rows=null;requestID=null;submitted=null;d.querySelector('[data-preview]').replaceChildren();submit.textContent='Preview names';};
  d.querySelector('[name=prefix]').addEventListener('input',invalidate);d.querySelector('[name=proxies]').addEventListener('input',invalidate);
  d.querySelector('form').onsubmit=e=>{e.preventDefault();submit.click();};
  submit.onclick=async()=>{
    if(busy)return;error.textContent='';
    try{
      if(!rows){rows=residentialRows(d.querySelector('[name=proxies]').value,d.querySelector('[name=prefix]').value,(cache.residential||[]).map(x=>x.name));d.querySelector('[data-preview]').innerHTML=`<h4>Confirm ${rows.length} exact names</h4>`+rows.map((x,i)=>`<label class="residentialPreview"><span>${i+1}. ${esc(x.endpoint)}</span><input data-row="${i}" value="${esc(x.name)}" maxlength="80" aria-label="Name for proxy ${i+1}"></label>`).join('');submit.textContent=`Create ${rows.length} proxies`;return;}
      const used=new Set((cache.residential||[]).map(x=>x.name.toLowerCase()));
      const proxies=rows.map((x,i)=>{const name=d.querySelector(`[data-row="${i}"]`).value;if(!name||name!==name.trim()||name.length>80||/[\x00-\x1f\x7f]/.test(name))throw Error(`Row ${i+1}: enter a valid exact name.`);if(used.has(name.toLowerCase()))throw Error(`Row ${i+1}: name already exists or repeats in this batch.`);used.add(name.toLowerCase());return {name,line:x.line};});
      const value=JSON.stringify(proxies);if(submitted!==value){requestID=residentialUUIDValue();submitted=value;}
      busy=true;submit.disabled=true;d.querySelectorAll('input,textarea').forEach(x=>x.disabled=true);
      const result=await api('/api/v1/residential-proxies/import',{method:'POST',body:JSON.stringify({request_id:requestID,proxies})});
      rows=null;submitted=null;d.close();toast(`${result.created} residential proxies saved`);await load('residential');refreshResidentialSelection();
    }catch(e){error.textContent=e.message;}finally{busy=false;submit.disabled=false;d.querySelectorAll('input,textarea').forEach(x=>x.disabled=false);}
  };
}
async function copyResidential(ids){
  const x=await api('/api/v1/residential-proxies/export',{method:'POST',body:JSON.stringify({ids})});const text=x.proxies.map(p=>p.line).join('\n');
  try{await navigator.clipboard.writeText(text);toast(`${x.proxies.length} proxies copied`);}catch{
    const d=residentialDialog('Copy proxies',`<p class="muted">Clipboard access is unavailable. Select and copy this text. It contains proxy passwords.</p><textarea rows="12" readonly spellcheck="false"></textarea>`);d.querySelector('textarea').value=text;d.querySelector('[data-submit]').textContent='Select all';d.querySelector('[data-submit]').onclick=()=>{d.querySelector('textarea').select();};d.querySelector('textarea').select();
  }
}
document.addEventListener('change',e=>{
  const id=e.target.dataset.residentialId;if(id){e.target.checked?residentialSelection.add(id):residentialSelection.delete(id);refreshResidentialSelection();}
  if(e.target.dataset.residential==='select-all'){residentialSelection.clear();if(e.target.checked)(cache.residential||[]).forEach(x=>residentialSelection.add(x.id));document.querySelectorAll('[data-residential-id]').forEach(x=>x.checked=e.target.checked);refreshResidentialSelection();}
});
document.addEventListener('click',async e=>{
  const b=e.target.closest('[data-residential]');if(!b||b.tagName!=='BUTTON'||b.disabled)return;const action=b.dataset.residential;
  if(action==='import'){openResidentialBulk();return;}
  const rows=(cache.residential||[]).filter(x=>action.endsWith('-all')||residentialSelection.has(x.id));if(!rows.length)return;
  b.disabled=true;
  try{
    if(action.startsWith('copy-'))await copyResidential(rows.map(x=>x.id));
    else if(action.startsWith('delete-')){const names=rows.slice(0,12).map(x=>x.name).join(', ')+(rows.length>12?'…':'');if(!confirm(`Delete ${rows.length} residential proxies?\n${names}\n\nResidential routes will be removed from all panels. Shared account API proxies are retained.`))return;const x=await api('/api/v1/residential-proxies/delete',{method:'POST',body:JSON.stringify({ids:rows.map(x=>x.id)})});rows.forEach(x=>residentialSelection.delete(x.id));toast(`${x.deleted} residential proxies deleted`);await load('residential');}
  }catch(e){toast(e.message,'error');}finally{b.disabled=false;refreshResidentialSelection();}
});
setInterval(async()=>{
  if(current!=='residential'||document.hidden||document.querySelector('dialog[open]'))return;
  try{const rows=await api('/api/v1/residential-proxies');if(current!=='residential'||document.querySelector('dialog[open]'))return;cache.residential=rows;const holder=document.createElement('div');holder.innerHTML=renderResidential(rows);const target=document.querySelector('#residentialCards');if(target)target.replaceWith(holder.querySelector('#residentialCards'));refreshResidentialSelection();}catch{}
},10000);
