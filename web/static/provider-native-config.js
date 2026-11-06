/* A dedicated, fail-closed web profile. No provider master credential is exported. */
(() => {
 'use strict';
 function build(session,origin){
  const u=new URL(origin);
  if(u.protocol!=='https:'||u.username||u.password||u.pathname!=='/'||u.search||u.hash)throw Error('Open the panel over HTTPS before creating a native browser profile.');
  if(!/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(session.credential)||!/^\/api\/v1\/provider-browser\/native-stream\/[a-f0-9]{48}$/.test(session.path))throw Error('Invalid temporary connection details.');
  const host=u.hostname.replace(/^\[|\]$/g,''),port=Number(u.port||443),literal=/^[\d.]+$/.test(host)||host.includes(':');
  const controller={type:'field',port:String(port),network:'tcp',outboundTag:'panel-only'};
  controller[literal?'ip':'domain']=[literal?host:'full:'+host];
  return {
   remarks:'Browser · '+session.provider+' · '+session.country+' · '+new Date(session.expires_at).toISOString(),
   log:{loglevel:'none'},
   dns:{tag:'browser-dns',queryStrategy:'UseIPv4',disableFallback:true,servers:['https://1.1.1.1/dns-query']},
   inbounds:[{tag:'socks',listen:'127.0.0.1',port:10808,protocol:'socks',settings:{auth:'noauth',udp:true,userLevel:0},sniffing:{enabled:true,destOverride:['http','tls'],routeOnly:true}}],
   outbounds:[
    {tag:'browser-proxy',protocol:'vless',settings:{vnext:[{address:host,port,users:[{id:session.credential,encryption:'none'}]}]},streamSettings:{network:'ws',security:'tls',tlsSettings:{serverName:host},wsSettings:{path:session.path,headers:{Host:u.host}}},mux:{enabled:false}},
    {tag:'browser-resolver',protocol:'dns',settings:{nonIPQuery:'drop'}},
    {tag:'blocked',protocol:'blackhole',settings:{}},
    {tag:'panel-only',protocol:'freedom',settings:{}}
   ],
   routing:{domainStrategy:'AsIs',rules:[
    controller,
    {type:'field',port:'53',network:'udp',outboundTag:'browser-resolver'},
    {type:'field',network:'udp',outboundTag:'blocked'},
    {type:'field',ip:['0.0.0.0/8','10.0.0.0/8','100.64.0.0/10','127.0.0.0/8','169.254.0.0/16','172.16.0.0/12','192.168.0.0/16','198.18.0.0/15','224.0.0.0/4','240.0.0.0/4','::/0'],outboundTag:'blocked'},
    {type:'field',network:'tcp',port:'80,443',outboundTag:'browser-proxy'},
    {type:'field',network:'tcp,udp',outboundTag:'blocked'}
   ]}
  };
 }
 globalThis.ProviderNativeConfig={build};
})();
