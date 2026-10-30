/* Three ordered country slots. AUTO placeholders never widen an explicit policy. */
(() => {
 'use strict';
 const escape=value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
 const names=['proxy_country','proxy_country2','proxy_country3'];
 function normalize(values){
  if(!Array.isArray(values))values=[values||'AUTO'];
  if(values.length>3)throw Error('Choose at most three countries.');
  const result=values.map(x=>String(x||'AUTO').trim().toUpperCase()).filter(x=>x!=='AUTO');
  if(result.some(x=>!/^[A-Z]{2}$/.test(x)))throw Error('Choose a valid country.');
  if(new Set(result).size!==result.length)throw Error('Choose different countries for each priority.');
  return result.length?result:['AUTO'];
 }
 function read(source){return normalize(names.map(n=>source?.get?source.get(n):source.querySelector('[name='+n+']')?.value))}
 function set(form,values){names.forEach((n,i)=>{const e=form.querySelector('[name='+n+']');if(e)e.value=values?.[i]||'AUTO'})}
 function field(options,selected=['AUTO']){
  if(!Array.isArray(selected))selected=[selected];
  const values=[{code:'AUTO',label:'AUTO — automatic selection'},...options.filter(x=>x.code!=='AUTO')];
  return names.map((name,i)=>'<label>Proxy country — priority '+(i+1)+'<select name="'+name+'" aria-label="Proxy country priority '+(i+1)+'">'+values.map(x=>'<option value="'+escape(x.code)+'" '+(x.code===(selected[i]||'AUTO')?'selected':'')+'>'+escape(x.label)+(x.code==='AUTO'?'':' ('+escape(x.code)+')')+'</option>').join('')+'</select></label>').join('')+'<p class="muted">Countries are tried in order across the assigned proxies. With selected countries, AUTO slots are unused; traffic stops if none can be verified. All AUTO uses automatic selection.</p>';
 }
 function validate(countries,primary,others=[]){
  if(!primary&&others.some(Boolean))throw Error('Select the primary proxy first, or clear all proxies for direct access.');
  if(normalize(countries)[0]!=='AUTO'&&!primary)throw Error('Select a proxy before choosing a country.');
 }
 function status(policy){
  const auto=policy.countries?.length===1&&policy.countries[0]==='AUTO';
  const verified=policy.identity_verified===true&&(auto||policy.applied_revision===policy.revision&&policy.countries?.includes(policy.active_stage)&&policy.active_stage===policy.active_country?.toUpperCase());
  return [auto?'Automatic selection':verified?'Country verified':'Country verification pending',policy.active_country?(verified?'Verified country: ':'Previous observation: ')+policy.active_country.toUpperCase():'',policy.active_proxy,auto?'':policy.detail].filter(Boolean).join(' · ');
 }
 const api={field,validate,status,normalize,read,set,names};
 if(typeof module==='object'&&module.exports)module.exports=api;else window.ProxyCountry=api;
})();
