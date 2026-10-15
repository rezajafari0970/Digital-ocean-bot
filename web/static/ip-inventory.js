/* Status/settings use the common view lifecycle. No provider write is exposed. */
const ipPolicyNumbers=[
 ['desired','Desired count'],['per_run','IPs per allocation batch'],['max_concurrent','Concurrent operations'],
 ['spacing_min_seconds','Minimum allocation interval (seconds)'],['spacing_max_seconds','Maximum allocation interval (seconds)'],
 ['lifetime_min_seconds','Minimum endpoint lifetime (seconds)'],['lifetime_max_seconds','Maximum endpoint lifetime (seconds)'],
 ['allocation_bits','Provider allocation prefix length'],['diversity_bits','IPv6 diversity prefix length'],
 ['history_seconds','Historical uniqueness retention (seconds)'],['funded_horizon_seconds','Funded horizon (seconds)']
];
function ipPolicyForm(a,family){
 const rec=(a.policies||[]).find(p=>p.family===family)||{revision:0,policy:{family,enabled:false}};
 const p=rec.policy,products=a.provider==='linode'?(family===4?['public']:['routed']):a.provider==='upcloud'?(family===4?['public','floating']:['public']):['reserved'];
 const field=(key,label,options)=>'<label>'+esc(label)+'<select name="'+key+'"><option value="">Unset</option>'+options.map(v=>'<option value="'+v+'" '+(p[key]===v?'selected':'')+'>'+v+'</option>').join('')+'</select></label>';
 return '<details><summary>IPv'+family+' settings · revision '+esc(rec.revision)+' · '+(p.enabled?'Planning enabled':'Disabled')+'</summary><form data-ip-policy="'+family+'" data-revision="'+esc(rec.revision)+'"><p>Saving settings grants no allocation, release or network-change authority. All operating values must be explicit before planning can be enabled.</p><label><input type="checkbox" name="enabled" '+(p.enabled?'checked':'')+'> Enable planning</label><div class="ip-policy-fields">'+
 field('product','Provider IP product',products)+ipPolicyNumbers.filter(([k])=>family===6||!['diversity_bits','allocation_bits'].includes(k)).map(([k,label])=>'<label>'+esc(label)+'<input type="number" min="0" step="1" name="'+k+'" value="'+esc(p[k]==null?'':p[k])+'" placeholder="Unset"></label>').join('')+
 field('active_scope','Active subnet uniqueness scope',['GLOBAL','ACCOUNT'])+field('history_scope','History uniqueness scope',['GLOBAL','ACCOUNT'])+field('currency','Budget currency',['USD','EUR'])+
 '<label>Budget in selected currency<input type="text" inputmode="decimal" name="budget" value="'+esc(p.budget_micros==null?'':String(p.budget_micros/1000000))+'" placeholder="Unset"></label></div><p class="muted">IPv6 uses one address per selected diversity subnet. Allocation prefix and diversity prefix are separate decisions. Unbounded client leases can outlive the selected endpoint lifetime and delay release.</p><button type="submit">Save IPv'+family+' settings</button><p data-ip-policy-feedback role="status"></p></form></details>';
}

function ipFundingForm(a){
 const f=a.funding||{},date=f.funded_until?new Date(f.funded_until).toISOString().slice(0,16):'';
 return '<details><summary>Shared account budget · '+esc(f.currency||'Unset')+' · revision '+esc(f.revision||0)+'</summary><form data-ip-funding data-revision="'+esc(f.revision||0)+'"><p>This ceiling applies across IPv4 and IPv6. Saving it does not approve a purchase. Retained and unknown liabilities continue to count; lowering a ceiling does not interrupt service.</p><div class="ip-policy-fields"><label>Currency<select name="currency"><option value="">Unset</option>'+['USD','EUR'].map(c=>'<option '+(c===f.currency?'selected':'')+'>'+c+'</option>').join('')+'</select></label><label>Account budget<input name="budget" inputmode="decimal" placeholder="Unset" value="'+esc(f.budget_micros==null?'':f.budget_micros/1000000)+'"></label><label>Funding valid until (UTC)<input type="datetime-local" name="funded_until" value="'+esc(date)+'"></label><label>Account concurrency ceiling<input type="number" min="1" max="8" name="max_concurrent" placeholder="Unset" value="'+esc(f.max_concurrent==null?'':f.max_concurrent)+'"></label></div><p>Reserved estimated commitments: '+esc(a.reserved_commitment_micros==null?'Unknown':a.reserved_commitment_micros/1000000)+' '+esc(f.currency||'')+'. Actual charges: Unknown.</p><button type="submit">Save account budget</button><p data-ip-funding-feedback role="status"></p></form></details>';
}
function ipParseBudget(raw){
 const b=String(raw||'').trim();
 if(!/^\d+(\.\d{1,6})?$/.test(b))throw Error('Enter an explicit nonnegative budget with at most six decimal places');
 const parts=b.split('.'),n=Number(parts[0])*1000000+Number((parts[1]||'').padEnd(6,'0'));
 if(!Number.isSafeInteger(n)||n>1000000000000)throw Error('Budget is too large');return n;
}
function renderIPInventory(d){
 if(!['READ_ONLY_FOUNDATION','LIFECYCLE_PREPARATION'].includes(d.stage)||!Array.isArray(d.accounts))throw Error('Invalid IP inventory response');
 const val=x=>x==null?'Unknown':esc(x),date=x=>x?esc(formatDate(x)):'Never';
 return '<style>.ip-policy-fields{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(230px,100%),1fr));gap:12px;margin:14px 0}.ip-policy-fields label{display:flex;flex-direction:column;gap:5px}.ip-policy-fields input,.ip-policy-fields select{width:100%;box-sizing:border-box}</style>'+
 '<div class="card section"><h3>Additional IPs</h3><p><strong>Inventory and settings available · Allocation and rotation disabled</strong></p><p>Durable operation and ownership records are being prepared. Provider execution, guest network changes and publication of additional-IP configs require completed verification and an approved canary plan.</p><p class="muted">Refresh reads one account through its configured connection. Inventory is evidence, not permission or a promise of available allocation capacity. Unknown costs remain unknown.</p></div>'+
 (d.accounts.length?d.accounts.map(a=>{
 const operations=a.operations||[],owned=a.managed_allocations||[],endpoints=a.endpoints||[];
 return '<div class="card section" data-ip-account="'+esc(a.account_id)+'"><h3>'+esc(a.name)+' · '+esc(a.provider)+'</h3><p>'+esc(a.state)+' · '+esc(a.error_class||'No recorded error')+(a.http_status?' (HTTP '+esc(a.http_status)+')':'')+'</p><p class="muted">Last valid inventory: '+date(a.last_good_at)+' · Next allowed refresh: '+date(a.next_attempt_at)+(a.rate_limit_until?' · Provider cooldown: '+date(a.rate_limit_until):'')+'</p><button type="button" data-ip-refresh="'+esc(a.account_id)+'" '+(!a.enabled?'disabled':'')+'>Read this account</button><p data-ip-feedback role="status"></p>'+
 '<div class="tableWrap"><table class="table"><thead><tr><th>Family / product / scope</th><th>Observed limit</th><th>Used</th><th>Remaining</th><th>Last known limit</th><th>Evidence</th></tr></thead><tbody>'+
 (a.quotas.length?a.quotas.map(q=>'<tr><td>IPv'+esc(q.family)+' / '+esc(q.product)+' / '+esc(q.scope)+'</td><td>'+val(q.limit)+(q.capacity_changed?' · Changed':'')+'</td><td>'+val(q.used)+'</td><td>'+val(q.remaining)+'</td><td>'+val(q.last_known_limit)+' · '+date(q.last_known_at)+'</td><td>'+esc(q.source)+' · '+date(q.observed_at)+'</td></tr>').join(''):'<tr><td colspan="6">Quota unknown — no complete observation yet.</td></tr>')+'</tbody></table></div>'+
 ipFundingForm(a)+ipPolicyForm(a,4)+ipPolicyForm(a,6)+
 '<details><summary>'+a.resources.length+' observed IP resources</summary><p>Discovery does not confer ownership or authority to release an IP.</p><div class="tableWrap"><table class="table"><thead><tr><th>IP / prefix</th><th>Kind</th><th>Provider server</th><th>Region</th><th>Ownership</th></tr></thead><tbody>'+a.resources.map(x=>'<tr><td>'+esc(x.address||x.prefix)+'</td><td>'+esc(x.kind)+'</td><td>'+esc(x.server_id||'Unattached / not exposed')+'</td><td>'+esc(x.region||'Unknown')+'</td><td>Protected · discovery only</td></tr>').join('')+'</tbody></table></div></details>'+
 '<details><summary>'+owned.length+' feature-owned allocations · '+endpoints.length+' active endpoint records</summary><div class="tableWrap"><table class="table"><thead><tr><th>Provider resource</th><th>State / server</th><th>Acquired</th><th>Hourly cost</th><th>Cost source</th></tr></thead><tbody>'+owned.map(x=>'<tr><td>'+esc(x.product)+' / '+esc(x.prefix||'Unknown')+'</td><td>'+esc(x.state)+' / '+esc(x.server_id)+'</td><td>'+date(x.acquired_at)+'</td><td>'+val(x.hourly_micros==null?null:x.hourly_micros/1000000)+' '+esc(x.currency||'')+'</td><td>'+esc(x.cost_source||'Unknown')+' / '+date(x.cost_checked_at)+'</td></tr>').join('')+'</tbody></table></div><div class="tableWrap"><table class="table"><thead><tr><th>Endpoint / diversity subnet</th><th>Phase</th><th>Selected lifetime ends</th><th>Protected leases</th></tr></thead><tbody>'+endpoints.map(x=>'<tr><td>'+esc(x.address)+' / '+esc(x.prefix||'IPv4 host')+'</td><td>'+esc(x.phase)+'</td><td>'+date(x.expires_at)+'</td><td>'+esc(x.protected_leases)+'</td></tr>').join('')+'</tbody></table></div></details>'+
 '<details><summary>'+operations.length+' recent operation records</summary><p>Unknown outcomes retain their reservations and require reconciliation; no automatic replay.</p><div class="tableWrap"><table class="table"><thead><tr><th>Action / family</th><th>Server</th><th>State</th><th>Error</th><th>Next attempt</th></tr></thead><tbody>'+operations.map(x=>'<tr><td>'+esc(x.action)+' / IPv'+esc(x.family)+'</td><td>'+esc(x.server_id)+'</td><td>'+esc(x.state)+'</td><td>'+esc(x.error_class||'None')+'</td><td>'+date(x.next_attempt_at)+'</td></tr>').join('')+'</tbody></table></div></details></div>';
 }).join(''):'<div class="card section">No accounts configured.</div>');
}
function ipPolicyFromForm(form,family){
 const f=new FormData(form),p={family,enabled:f.has('enabled')};
 for(const [k] of ipPolicyNumbers){
  const v=f.get(k);
  if(v==null||v===''){p[k]=null;continue}
  const n=Number(v);if(!Number.isSafeInteger(n)||n<0)throw Error('Use a nonnegative whole number for '+k);p[k]=n;
 }
 for(const k of ['product','active_scope','history_scope','currency'])p[k]=String(f.get(k)||'');
 const b=String(f.get('budget')||'').trim();
 if(!b)p.budget_micros=null;
 else{
  if(!/^\d+(\.\d{1,6})?$/.test(b))throw Error('Budget must be a nonnegative amount with at most six decimal places');
  const parts=b.split('.');const n=Number(parts[0])*1000000+Number((parts[1]||'').padEnd(6,'0'));
  if(!Number.isSafeInteger(n)||n>1000000000000)throw Error('Budget is too large');p.budget_micros=n;
 }
 return p;
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
 c.querySelectorAll('[data-ip-policy]').forEach(form=>form.onsubmit=async e=>{
  e.preventDefault();const feedback=form.querySelector('[data-ip-policy-feedback]'),button=form.querySelector('button[type=submit]');button.disabled=true;
  try{
   const family=Number(form.dataset.ipPolicy),policy=ipPolicyFromForm(form,family),account=form.closest('[data-ip-account]').dataset.ipAccount;
   await api('/api/v1/accounts/'+encodeURIComponent(account)+'/ip-policy/'+family,{method:'PUT',body:JSON.stringify({revision:Number(form.dataset.revision),policy})});
   if(currentPageView(view))await load('ip-inventory');
  }catch(error){if(currentPageView(view)&&feedback.isConnected){feedback.textContent='Settings: '+error.message;button.disabled=false}}
 });
 c.querySelectorAll('[data-ip-funding]').forEach(form=>form.onsubmit=async e=>{
  e.preventDefault();const feedback=form.querySelector('[data-ip-funding-feedback]'),button=form.querySelector('button');button.disabled=true;
  try{
   const data=new FormData(form),end=String(data.get('funded_until')||''),max=Number(data.get('max_concurrent'));
   if(!end||!Number.isSafeInteger(max)||max<1||max>8)throw Error('Set a funding end time and account concurrency');
   const body={revision:Number(form.dataset.revision),currency:String(data.get('currency')||''),budget_micros:ipParseBudget(data.get('budget')),funded_until:new Date(end+'Z').toISOString(),max_concurrent:max};
   const account=form.closest('[data-ip-account]').dataset.ipAccount;
   await api('/api/v1/accounts/'+encodeURIComponent(account)+'/ip-funding',{method:'PUT',body:JSON.stringify(body)});
   if(currentPageView(view))await load('ip-inventory');
  }catch(error){if(currentPageView(view)&&feedback.isConnected){feedback.textContent='Budget: '+error.message;button.disabled=false}}
 });

}
