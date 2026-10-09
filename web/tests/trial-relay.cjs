const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict'),path=require('node:path');
const src=fs.readFileSync(path.join(__dirname,'../static/app.js'),'utf8');
const start=src.indexOf('function trialRelayCard('),end=src.indexOf('window.accountUpCloudTrial=',start);
const ctx={esc:x=>String(x).replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('"','&quot;'),formatDate:x=>String(x)};
vm.createContext(ctx);vm.runInContext(src.slice(start,end),ctx);
const relay={status:'healthy',healthy:17,selected:20,checked_at:'2026-10-10T00:00:00Z',panels:[{panel_id:'server-a',healthy:9,selected:10},{panel_id:'server-b',healthy:8,selected:10}]};
const html=ctx.creationBlockCard('account',null,'',true,0,relay);
assert.ok(html.includes('17 healthy / 20 configured'));
assert.ok(html.includes('9 healthy / 10 paths'));
assert.ok(html.includes('8 healthy / 10 paths'));
assert.ok(!html.includes('no enabled proxy'));
assert.ok(html.includes('SOCKS · Healthy'));
relay.status='unavailable';relay.panels[1].healthy=0;relay.healthy=9;
assert.ok(ctx.trialRelayCard(relay,0).includes('No verified path on one or more servers'));
relay.status='degraded';
assert.ok(ctx.trialRelayCard(relay,0).includes('Reduced redundancy'));
assert.ok(ctx.trialRelayCard(null,0).includes('Protected traffic stays blocked'));
relay.panels[0].panel_id='<script>';
assert.ok(!ctx.trialRelayCard(relay,0).includes('<script>'));
if(process.env.DOB_UI_ARTIFACT_DIR){
 const dir=process.env.DOB_UI_ARTIFACT_DIR;
 const css=fs.readFileSync(path.join(__dirname,'../static/app.css'),'utf8');
 fs.writeFileSync(path.join(dir,'trial-relay-ui.html'),'<meta name="viewport" content="width=device-width,initial-scale=1"><style>'+css+'</style><main style="padding:12px;max-width:390px"><h1>Accounts</h1>'+html+'</main>');
}
console.log('TRIAL_RELAY_UI_PASS');
