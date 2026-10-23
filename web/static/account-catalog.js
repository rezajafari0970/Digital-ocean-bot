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
 const api={inRegion,plans,images,regions};
 if(typeof module==='object' && module.exports)module.exports=api;
 else window.AccountCatalog=api;
})();
