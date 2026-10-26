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
 // Optional preferences are alternatives, not weights. Keep the primary choice.
 function uniqueChoices(root,prefix,count,changed){
  const fields=Array.from({length:count},(_,i)=>root.querySelector('[name="'+prefix+(i?i+1:'')+'"]')).filter(Boolean);
  const seen=new Set();
  if(changed&&changed!==fields[0]&&changed.value===fields[0]?.value)changed.value='';
  if(fields[0]?.value)seen.add(fields[0].value);
  if(changed?.value)seen.add(changed.value);
  for(const field of fields){if(field===changed||field===fields[0]||!field.value)continue;if(seen.has(field.value))field.value='';else seen.add(field.value)}
 }
 function verifiedLabel(d){
  const account=d.account||{},email=account.email||account.Email;
  return email||(d.provider==='kamatera'?'Kamatera API access':account.ID||account.id||d.provider);
 }
 const api={inRegion,plans,images,regions,approvalRequired,uniqueChoices,verifiedLabel};
 if(typeof module==='object' && module.exports)module.exports=api;
 else window.AccountCatalog=api;
})();
