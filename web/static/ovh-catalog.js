/* OVH display policy shared by onboarding and account editing. */
(() => {
 'use strict';
 const amount=n=>new Intl.NumberFormat('en-US',{maximumFractionDigits:8}).format(n);
 function priceLabel(plan,region){
  const p=plan.PricesByRegion?.[region];
  if(!p||!/^[A-Z]{3}$/.test(p.Currency)||!Number.isFinite(p.Hourly)||p.Hourly<0)return 'Price unavailable';
  return (p.Complete?'':'From ')+p.Currency+' '+amount(p.Hourly)+'/h | ≈'+p.MonthlyEstimate.toFixed(2)+'/730h';
 }
 function planLabel(plan,region){
  const x=plan.DetailsByRegion?.[region]||plan;
  const ram=x.MemoryMB>=1024?amount(x.MemoryMB/1024)+' GB':x.MemoryMB+' MB';
  return plan.ID+' | '+priceLabel(plan,region)+' | '+ram+' RAM | '+x.CPU+' vCPU | '+x.DiskGB+' GB disk';
 }
 function status(x){
  if(!x)return 'Not offered in this region';
  if(x.Reason)return x.Reason;
  if(x.Quota===0)return 'Supported · current quota: 0';
  return 'Supported'+(Number.isInteger(x.Quota)?' · quota: '+x.Quota:'');
 }
 function mount(root,data,region){
  let panel=root.querySelector('[data-ovh-catalog]');
  if(!panel){
   panel=document.createElement('section');panel.dataset.ovhCatalog='';
   const note=document.createElement('p');note.className='muted';note.dataset.ovhPricing='';
   const selected=document.createElement('div');selected.dataset.ovhSelected='';
   const details=document.createElement('details');
   const summary=document.createElement('summary');summary.textContent='All OVH plans and regional availability';
   const search=document.createElement('input');search.type='search';search.placeholder='Search plan, region or family';search.setAttribute('aria-label','Search all OVH plans');
   const count=document.createElement('p');
   const rows=document.createElement('div');rows.style.cssText='max-height:24rem;overflow:auto;overflow-wrap:anywhere';
   details.append(summary,search,count,rows);panel.append(note,selected,details);
   root.querySelector('[name=size]')?.closest('label').before(panel);
   panel.render=(d,r)=>{
    const pricing=d.pricing;
    note.textContent=pricing?.Status==='available'
     ?'Official OVH catalog '+pricing.CatalogID+' · '+pricing.Subsidiary+' / '+pricing.Currency+' · '+(pricing.Basis==='account_market'?'account market':pricing.Basis==='selected_market'?'selected market':'public reference; account market not verified')+'. Updated '+new Date(pricing.ObservedAt).toLocaleString()+'. Hourly compute + listed local disk; IPv4, network, taxes and other extras excluded. 730-hour amounts are estimates, not monthly caps or invoices.'
     :'OVH prices are temporarily unavailable. Plan discovery remains available. Reload provider data to retry.';
    const refreshSelected=()=>{
     selected.replaceChildren();const selectedIDs=new Set();
     for(const name of ['size','size2','size3']){
      const id=root.querySelector('[name='+name+']')?.value,p=(d.plans||[]).find(p=>p.ID===id),price=p?.PricesByRegion?.[r];
      if(!p||selectedIDs.has(id))continue;selectedIDs.add(id);
      const line=document.createElement('p');line.className='muted';
      const meta=p.DetailsByRegion?.[r];
      line.textContent=p.ID+' · '+r+' · '+status(meta)+(meta?' · network '+meta.InboundMbps+'/'+meta.OutboundMbps+' Mbps':'')+(price?' · '+(price.Components||[]).map(c=>c.Name+' ('+c.Quantity+'): '+price.Currency+' '+amount(c.Hourly)+'/h').join(' + '):'');
      selected.append(line);
     }
    };
    refreshSelected();
    for(const name of ['size','size2','size3']){const el=root.querySelector('[name='+name+']');if(el)el.onchange=refreshSelected}
    const draw=()=>{
     rows.replaceChildren();const q=search.value.trim().toLowerCase();
     const all=d.plans||[],shown=all.filter(p=>(p.ID+' '+Object.keys(p.DetailsByRegion||{}).join(' ')+' '+Object.values(p.DetailsByRegion||{}).map(x=>x.Type+' '+x.OS).join(' ')).toLowerCase().includes(q));
     count.textContent=shown.length+' / '+all.length+' plan variants · '+all.filter(p=>p.Available).length+' supported and in stock in at least one enabled region';
     for(const p of shown){
      const regions=Object.keys(p.DetailsByRegion||{}).sort(),defaultRegion=regions.includes(r)?r:regions[0];
      const item=document.createElement('details'),title=document.createElement('summary');
      title.textContent=planLabel(p,defaultRegion)+' · '+defaultRegion;
      item.append(title);
      for(const id of regions){
       const x=p.DetailsByRegion[id],line=document.createElement('p');line.className='muted';
       line.textContent=id+' · '+planLabel(p,id)+' · '+x.OS+' · '+status(x)+(!p.PricesByRegion?.[id]?' · No matching official hourly rate in this market':'');
       item.append(line);
      }
      rows.append(item);
     }
    };
    search.oninput=draw;draw();
   };
  }
  panel.render(data,region);
 }
 const api={priceLabel,planLabel,status,mount};
 if(typeof module==='object'&&module.exports)module.exports=api;else window.OVHCatalog=api;
})();
