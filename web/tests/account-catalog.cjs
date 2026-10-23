const assert=require('node:assert/strict'),catalog=require('../static/account-catalog.js');
const d={
 regions:[{ID:'SBG',Available:true},{ID:'SBG5',Available:true},{ID:'WAW1',Available:true}],
 plans:[{ID:'b3-8',Available:true,AvailableRegions:['SBG5','WAW1'],IDsByRegion:{SBG5:'f1',WAW1:'f2'}}],
 images:[{ID:'ubuntu',Available:true,Family:'ubuntu',Version:'24.04',IDsByRegion:{SBG5:'i1'}}]
};
assert.deepEqual(catalog.regions(d).map(x=>x.ID),['SBG5']);
assert.equal(catalog.inRegion(d.plans[0],'SBG'),false);
assert.deepEqual(catalog.regions({...d,plans:[]}),[]);
assert.deepEqual(catalog.regions({...d,images:[]}),[]);
assert.deepEqual(catalog.regions({...d,images:[{...d.images[0],IDsByRegion:{}}]}),[]);
assert.deepEqual(catalog.regions({...d,plans:[{...d.plans[0],Available:false}]}),[]);
assert.deepEqual(catalog.regions({...d,images:[{...d.images[0],Version:'20.04'}]}),[]);
assert.equal(catalog.regions({...d,plans:[{ID:'global',Available:true}],images:[{...d.images[0],IDsByRegion:null}]}).length,3);
console.log('Regional plan/image availability and empty catalog guards PASS');

const ovh=require('../static/ovh-catalog.js');
const plan={ID:'b3-8',CPU:2,MemoryMB:8192,DiskGB:50,PricesByRegion:{GRA11:{Currency:'EUR',Hourly:0.05849,MonthlyEstimate:42.6977,Complete:true},BHS5:{Currency:'CAD',Hourly:0.09,MonthlyEstimate:65.7,Complete:false}}};
assert.match(ovh.priceLabel(plan,'GRA11'),/^EUR 0.05849\/h/);
assert.match(ovh.priceLabel(plan,'BHS5'),/^From CAD/);
assert.equal(ovh.priceLabel(plan,'SBG'),'Price unavailable');
assert.match(ovh.planLabel(plan,'GRA11'),/8 GB RAM/);
assert.match(ovh.status({Quota:0}),/quota: 0/);
assert.equal(ovh.status({Reason:'Out of stock'}),'Out of stock');
assert.match(ovh.priceLabel({...plan,PricesByRegion:{GRA11:{Currency:'EUR',Hourly:0,MonthlyEstimate:0,Complete:true}}},'GRA11'),/^EUR 0\/h/);
console.log('OVH regional currency, precise rates, unknown price and quota guards PASS');
