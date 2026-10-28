/* Regional catalog selection shared by account onboarding controls. */
(() => {
 'use strict';
 function inRegion(item, region) {
  if(!item.Available || !region)return false;
  if(item.IDsByRegion != null && !item.IDsByRegion[region])return false;
  return !Array.isArray(item.AvailableRegions) || !item.AvailableRegions.length || item.AvailableRegions.includes(region);
 }
 const plans = d => (d.plans || d.sizes || []).filter(x=>x.Available);
 const images = d => (d.images || []).filter(x=>x.Available && x.Family==='ubuntu' && ['22.04','24.04','26.04'].includes(x.Version));
 function regions(d) {
  const ps=plans(d),ims=images(d);
  return (d.regions || []).filter(r=>r.Available && ps.some(p=>inRegion(p,r.ID)) && ims.some(im=>inRegion(im,r.ID)));
 }
 // Network consent belongs to the preview contract, not an AWS display module.
 const approvalRequired=d=>!!(d.onboarding?.network_approval||d.onboarding?.resolved?.managed_network);
 // Keep native phone pickers short. Complete specifications/prices live in the
 // selected-plan summary, outside the platform-owned option popup.
 function planOptionLabel(plan){
  const ram=Number(plan.MemoryMB),cpu=Number(plan.CPU);
  return String(plan.ID||'')+(ram>0?' · '+(ram>=1024?ram/1024+' GB':ram+' MB'):'')+(cpu>0?' / '+cpu+' CPU':'');
 }
 // Only restore options disabled by this helper; catalog restrictions belong
 // to their own owner. A selected option stays enabled so FormData retains it.
 const duplicateOptions=new WeakSet();
 function disableDuplicateChoices(fields){
  for(const field of fields)for(const option of field.options){
   if(duplicateOptions.has(option)){option.disabled=false;duplicateOptions.delete(option)}
   if(!option.disabled&&option.value&&field.value!==option.value&&fields.some(other=>other!==field&&other.value===option.value)){
    option.disabled=true;duplicateOptions.add(option);
   }
  }
 }
 // Optional preferences are alternatives, not weights. Keep the primary choice.
 function uniqueChoices(root,prefix,count,changed){
  const fields=Array.from({length:count},(_,i)=>root.querySelector('[name="'+prefix+(i?i+1:'')+'"]')).filter(Boolean);
  const seen=new Set();
  if(changed&&changed!==fields[0]&&changed.value===fields[0]?.value)changed.value='';
  if(fields[0]?.value)seen.add(fields[0].value);
  if(changed?.value)seen.add(changed.value);
  for(const field of fields){if(field===changed||field===fields[0]||!field.value)continue;if(seen.has(field.value))field.value='';else seen.add(field.value)}
  disableDuplicateChoices(fields);
 }
 function verifiedLabel(d){
  const account=d.account||{},email=account.email||account.Email;
  return email||(d.provider==='kamatera'?'Kamatera API access':account.ID||account.id||d.provider);
 }
 const api={inRegion,plans,images,regions,approvalRequired,uniqueChoices,disableDuplicateChoices,planOptionLabel,verifiedLabel};
 if(typeof module==='object' && module.exports)module.exports=api;
 else window.AccountCatalog=api;
})();
