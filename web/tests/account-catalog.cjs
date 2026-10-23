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
