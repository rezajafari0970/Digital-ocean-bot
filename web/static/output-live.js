(()=>{
const url=new URL(location.href);url.searchParams.delete('view');
const out=document.getElementById('configs');
const status=document.getElementById('outputStatus');
let busy=false,timer;
function showStatus(text){if(status){status.textContent=text;status.hidden=!text}}
function clear(){out.textContent=''}
async function refresh(){
 if(busy||document.hidden)return;busy=true;
 if(!out.textContent)showStatus('Loading current configs…');
 const controller=new AbortController(),timeout=setTimeout(()=>controller.abort(),5000);
 try{
  const response=await fetch(url,{cache:'no-store',signal:controller.signal});
  if(!response.ok)throw Error('Output unavailable');
  const text=await response.text(),lines=text.trim()?text.trim().split(/\r?\n/):[];
  if(lines.some(x=>!/^vless:\/\/[^\s]+$/.test(x)))throw Error('Invalid Output');
  // Status is separate from the copyable URI block and hidden when it has data.
  // A page hidden during a request must not republish stale credentials.
  out.textContent=document.hidden?'':lines.length?lines.join('\n')+'\n':'';
  showStatus(document.hidden?'':lines.length?'':'No configs currently match this output. Refreshing automatically.');
 }catch{clear();showStatus(document.hidden?'':'Output is temporarily unavailable. Retrying automatically.')}
 finally{clearTimeout(timeout);busy=false;timer=setTimeout(refresh,1000)}
}
document.addEventListener('visibilitychange',()=>{clearTimeout(timer);if(document.hidden){clear();showStatus('')}else refresh()});
refresh();
})();
