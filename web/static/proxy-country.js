/* The same single-country constraint is used before and after account creation. */
(() => {
 'use strict';
 const escape=value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
 function field(options,value='AUTO'){
  const values=[{code:'AUTO',label:'AUTO — automatic selection'},...options.filter(x=>x.code!=='AUTO')];
  return '<label>Proxy country<select name="proxy_country" aria-label="Proxy country">'+values.map(x=>'<option value="'+escape(x.code)+'" '+(x.code===value?'selected':'')+'>'+escape(x.label)+(x.code==='AUTO'?'':' ('+escape(x.code)+')')+'</option>').join('')+'</select></label><p class="muted">A selected country applies to provider requests through every proxy assigned to this account. If it cannot be verified, requests stop. AUTO uses automatic selection.</p>';
 }
 function validate(country,primary,others=[]){
  if(!primary&&others.some(Boolean))throw Error('Select the primary proxy first, or clear all proxies for direct access.');
  if(country&&country!=='AUTO'&&!primary)throw Error('Select a proxy before choosing a country.');
 }
 function status(policy){
  const auto=policy.countries?.length===1&&policy.countries[0]==='AUTO';
  const verified=policy.identity_verified===true&&(auto||policy.applied_revision===policy.revision&&policy.active_stage===policy.countries?.[0]);
  return [auto?'Automatic selection':verified?'Country verified':'Country verification pending',policy.active_country?(verified?'Verified country: ':'Previous observation: ')+policy.active_country.toUpperCase():'',policy.active_proxy,auto?'':policy.detail].filter(Boolean).join(' · ');
 }
 const api={field,validate,status};
 if(typeof module==='object'&&module.exports)module.exports=api;else window.ProxyCountry=api;
})();
