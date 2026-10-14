/* Status reads use the common view lifecycle. Provider refresh is explicit per account. */
function renderIPInventory(d){
 if(d.stage!=='READ_ONLY_FOUNDATION'||!Array.isArray(d.accounts))throw Error('Invalid IP inventory response');
 const val=x=>x==null?'Unknown':esc(x), date=x=>x?esc(formatDate(x)):'Never';
 return '<div class="card section"><h3>Additional IPs</h3><p><strong>Discovery available · Allocation and rotation disabled</strong></p><p>Lifetime, desired count, interval, budget, canary servers and IPv6 diversity prefix await configuration. Network, config publication and rotation are not yet implemented in this release.</p><p class="muted">Refresh reads one account through its configured connection. It does not request, attach or release IPs. Inventory is historical evidence, not permission or a promise of available allocation capacity. Costs remain unknown.</p></div>'+
 (d.accounts.length?d.accounts.map(a=>'<div class="card section" data-ip-account="'+esc(a.account_id)+'"><h3>'+esc(a.name)+' · '+esc(a.provider)+'</h3><p>'+esc(a.state)+' · '+esc(a.error_class||'No recorded error')+(a.http_status?' (HTTP '+esc(a.http_status)+')':'')+'</p><p class="muted">Last valid inventory: '+date(a.last_good_at)+' · Next allowed refresh: '+date(a.next_attempt_at)+(a.rate_limit_until?' · Provider cooldown: '+date(a.rate_limit_until):'')+'</p><button type="button" data-ip-refresh="'+esc(a.account_id)+'" '+(!a.enabled?'disabled':'')+'>Read this account</button><p data-ip-feedback role="status"></p>'+
 '<div class="tableWrap"><table class="table"><thead><tr><th>Family / product / scope</th><th>Observed limit</th><th>Used</th><th>Remaining</th><th>Last known limit</th><th>Evidence</th></tr></thead><tbody>'+
 (a.quotas.length?a.quotas.map(q=>'<tr><td>IPv'+esc(q.family)+' / '+esc(q.product)+' / '+esc(q.scope)+'</td><td>'+val(q.limit)+(q.capacity_changed?' · Changed':'')+'</td><td>'+val(q.used)+'</td><td>'+val(q.remaining)+'</td><td>'+val(q.last_known_limit)+' · '+date(q.last_known_at)+'</td><td>'+esc(q.source)+' · '+date(q.observed_at)+'</td></tr>').join(''):'<tr><td colspan="6">Quota unknown — no complete observation yet.</td></tr>')+'</tbody></table></div>'+
 '<details><summary>'+a.resources.length+' observed IP resources</summary><p>Discovered resources are protected. Feature-owned: 0; pending release: 0. Endpoint lifetime, health and cost have not been verified.</p><div class="tableWrap"><table class="table"><thead><tr><th>IP / prefix</th><th>Kind</th><th>Provider server</th><th>Region</th><th>Ownership</th></tr></thead><tbody>'+a.resources.map(x=>'<tr><td>'+esc(x.address||x.prefix)+'</td><td>'+esc(x.kind)+'</td><td>'+esc(x.server_id||'Unattached / not exposed')+'</td><td>'+esc(x.region||'Unknown')+'</td><td>Protected · not managed by this feature</td></tr>').join('')+'</tbody></table></div></details></div>').join(''):'<div class="card section">No accounts configured.</div>');
}
async function loadIPInventory(c,view){
 const d=await viewRead('/api/v1/ip-inventory',view);if(!currentPageView(view))return;c.innerHTML=renderIPInventory(d);
 c.querySelectorAll('[data-ip-refresh]').forEach(b=>b.onclick=async()=>{
  if(b.disabled)return;b.disabled=true;const box=b.closest('[data-ip-account]'),feedback=box.querySelector('[data-ip-feedback]');
  feedback.textContent='Reading account…';
  try{const result=await api('/api/v1/accounts/'+encodeURIComponent(b.dataset.ipRefresh)+'/ip-inventory/refresh',{method:'POST'});
   if(!currentPageView(view))return;
   feedback.textContent=result.state==='OBSERVED'?'Inventory saved.':'Read failed: '+(result.error_class||'unknown');
   await load('ip-inventory');
  }catch(e){if(currentPageView(view)&&feedback.isConnected){feedback.textContent='Refresh: '+e.message;b.disabled=false}}
 });
}
