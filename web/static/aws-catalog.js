/* AWS display adapter. Native resource ownership stays in the Go driver. */
(() => {
 'use strict';
 const approvalRequired=d=>!!(d.onboarding?.network_approval||d.onboarding?.resolved?.managed_network);
 function priceLabel(plan,region){
  const p=plan.PricesByRegion?.[region];
  if(!p||p.Currency!=='USD'||p.Scope!=='compute_only'||!Number.isFinite(p.Hourly)||p.Hourly<=0)return 'Compute price unavailable';
  return 'USD '+new Intl.NumberFormat('en-US',{maximumSignificantDigits:6}).format(p.Hourly)+'/h compute';
 }
 function planLabel(p,region){
  const ram=p.MemoryMB>=1024?new Intl.NumberFormat('en-US',{maximumFractionDigits:3}).format(p.MemoryMB/1024)+' GiB':p.MemoryMB+' MiB';
  return p.ID+' | '+ram+' RAM | '+p.CPU+' vCPU | '+priceLabel(p,region);
 }
 function priceNote(d,region){
  const status=(d.pricing||d.Pricing)?.Status;
  const issue=status==='permission_required'?' AWS pricing:GetProducts permission is required to load prices.':status==='rate_limited'?' AWS temporarily limited pricing requests; reload later.':status==='unavailable'||status==='partial'?' Some rates could not be verified; unknown prices are not zero.':'';
  return 'Prices for '+region+': Linux shared On-Demand compute only. EBS gp3 root storage is billed separately (at least 12 GiB, or the Ubuntu image minimum). Public IPv4, transfer, burst CPU credits and taxes are extra. A monthly estimate is hourly × 730, not a billing cap.'+issue;
 }
 function uniqueChoices(root,prefix,count,changed){
  const fields=Array.from({length:count},(_,i)=>root.querySelector('[name="'+prefix+(i?i+1:'')+'"]')).filter(Boolean);
  const seen=new Set();
  if(changed&&changed!==fields[0]&&changed.value===fields[0]?.value)changed.value='';
  if(fields[0]?.value)seen.add(fields[0].value);
  if(changed?.value)seen.add(changed.value);
  for(const field of fields){if(field===changed||field===fields[0]||!field.value)continue;if(seen.has(field.value)&&field.options[0]?.value==='')field.value='';else seen.add(field.value)}
 }
 const networkReviewText='Automatic networking creates a dedicated security group before the first server in each applicable region. It allows inbound IPv4 TCP/UDP ports 1–65535 for SSH and service ports, with AWS default outbound rules. Existing groups are unchanged.';
 const api={approvalRequired,priceLabel,planLabel,priceNote,uniqueChoices,networkReviewText};
 if(typeof module==='object'&&module.exports)module.exports=api;else window.AWSCatalog=api;
})();
