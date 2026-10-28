/* Display contract for live Kamatera public base estimates and native shapes. */
(() => {
 'use strict';
 const scope='kamatera-public-base-hourly-v1';
 const types={A:'Availability',B:'General Purpose',D:'Dedicated',T:'Burstable'};
 function priceLabel(plan,region){
  const p=plan.PricesByRegion?.[region];
  if(!p?.Complete||p.Scope!==scope||p.Currency!=='USD'||!Number.isFinite(p.Hourly)||p.Hourly<=0||!Number.isFinite(p.MonthlyEstimate)||p.MonthlyEstimate<=0)return 'Price unavailable';
  return '≈USD '+new Intl.NumberFormat('en-US',{maximumFractionDigits:6}).format(p.Hourly)+'/h | ≈'+p.MonthlyEstimate.toFixed(2)+'/730h base'+(plan.DetailsByRegion?.[region]?.CPUType==='T'?' + CPU usage':'');
 }
 function planLabel(plan,region){
  const d=plan.DetailsByRegion?.[region],type=d?.CPUType||/^\d+([ABTD])-/.exec(plan.ID||'')?.[1]||'';
  const ram=plan.MemoryMB>=1024?(plan.MemoryMB/1024)+' GB':plan.MemoryMB+' MB';
  return ram+' RAM | '+plan.CPU+' vCPU '+type+(types[type]?' '+types[type]:'')+' | '+plan.DiskGB+' GB SSD | '+priceLabel(plan,region)+' | '+plan.ID;
 }
 function optionLabel(plan){
  const match=/^(\d+[ABTD])-(\d+)-(\d+)$/.exec(plan.ID||'');
  if(!match)return String(plan.ID||'');
  const ram=Number(match[2]);
  return match[1]+' · '+(ram>=1024?ram/1024+' GB':ram+' MB')+' · '+match[3]+' GB SSD';
 }
 function mount(root,catalog,region){
  let box=root.querySelector('[data-kamatera-pricing]');
  if(!box){box=document.createElement('div');box.dataset.kamateraPricing='';box.className='muted';root.querySelector('[name=size]')?.closest('label')?.before(box)}
  const selected=root.querySelector('[name=size]')?.value;
  const plan=(catalog.plans||catalog.sizes||[]).find(x=>x.ID===selected),detail=plan?.DetailsByRegion?.[region];
  const pricing=catalog.pricing||{},available=priceLabel(plan||{},region)!=='Price unavailable';
  box.replaceChildren();
  if(plan){const chosen=document.createElement("p"),strong=document.createElement("strong");strong.textContent=planLabel(plan,region);chosen.append(strong);box.append(chosen)}
  const summary=document.createElement('p');
  summary.textContent='Prices for '+region+'. '+(detail?.CatalogOrigin==='provider_preset'?'Provider preset. ':detail?.CatalogOrigin==='custom_preset'?'Custom configuration verified by the provider API. ':'')+'Hourly billing; internet traffic is charged separately. '+(available?'Public powered-on base estimate for Ubuntu and one IPv4. The 730-hour figure is not a monthly cap. ':'A verified price is unavailable for this configuration. Refresh the catalog to try again. ');
  box.append(summary);
  const terms=document.createElement('p');terms.textContent='Taxes, account discounts, traffic charges and Type T CPU usage charges are excluded. Powered-off servers still incur storage and IP charges. Backups and managed service are disabled. Saved accounts can start building servers.';box.append(terms);
  const source=document.createElement('p'),link=document.createElement('a');link.href='https://www.kamatera.com/pricing/';link.target='_blank';link.rel='noopener noreferrer';link.style.color='inherit';link.textContent='Official Kamatera calculator';source.append(link);
  const observed=new Date(pricing.ObservedAt||'');
  if(Number.isFinite(observed.getTime())&&observed.getUTCFullYear()>2000)source.append(' · Checked '+observed.toISOString());
  box.append(source);
 }
 const api={priceLabel,planLabel,optionLabel,mount};
 if(typeof module==='object'&&module.exports)module.exports=api;else window.KamateraCatalog=api;
})();
