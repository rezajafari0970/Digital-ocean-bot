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

 // Region selection is local form state. Network evidence is invalidated
 // whenever its owning region changes; it is never moved across regions.
 function discoveryRegions(values){
  const result={},seen=new Set();
  for(const key of ['region','region_2','region_3','region_4','region_5']){
   const value=String(values[key]||'');
   result[key]=value&&!seen.has(value)?value:'';
   if(result[key])seen.add(result[key]);
  }
  return result;
 }
 function syncDiscovery(root,selection,changedKey){
  const fields=[...root.querySelectorAll('[data-discovery-key]')];
  const regions=['region','region_2','region_3','region_4','region_5'].map(key=>fields.find(el=>el.dataset.discoveryKey===key)).filter(Boolean);
  if(!regions.length)return;
  const values=discoveryRegions(Object.fromEntries(regions.map(el=>[el.dataset.discoveryKey,el.value])));
  for(const el of fields){
   const key=el.dataset.discoveryKey;
   if(/^subnet(?:_id|_[2-5])$/.test(key)){if(el.disabled)delete selection[key];else selection[key]=el.value}
  }
  let invalidated=false;
  for(const el of regions){
   const key=el.dataset.discoveryKey,value=values[key];
   if(key===changedKey||el.value!==value||(selection[key]!==undefined&&selection[key]!==value)){
    const subnetKey=key==='region'?'subnet_id':'subnet_'+key.slice(-1);
    delete selection[subnetKey];
    const subnet=fields.find(el=>el.dataset.discoveryKey===subnetKey);
    if(subnet){subnet.replaceChildren(new Option('Validate again to load subnets',''));subnet.disabled=true}
    invalidated=true;
   }
   el.value=value;selection[key]=value;
  }
  for(const el of regions){
   const key=el.dataset.discoveryKey;
   const used=new Set(Object.entries(values).filter(([other])=>other!==key).map(([,value])=>value).filter(Boolean));
   for(const option of el.options)option.disabled=key!=='region'&&!!option.value&&used.has(option.value);
  }
  let help=root.querySelector('[data-aws-region-help]');
  if(!help){help=document.createElement('p');help.dataset.awsRegionHelp='';help.className='muted';help.setAttribute('role','status');root.append(help)}
  help.textContent=invalidated?'Region choices changed. Validate again to verify their subnets; previous network choices are cleared.':'Additional regions are optional. Leave None for unused slots. A region can be selected only once.';
 }

 const networkReviewText='Automatic networking creates a dedicated security group before the first server in each applicable region. It allows inbound IPv4 TCP/UDP ports 1–65535 for SSH and service ports, with AWS default outbound rules. Existing groups are unchanged.';
 const api={approvalRequired,priceLabel,planLabel,priceNote,uniqueChoices,discoveryRegions,syncDiscovery,networkReviewText};
 if(typeof module==='object'&&module.exports)module.exports=api;else window.AWSCatalog=api;
})();
