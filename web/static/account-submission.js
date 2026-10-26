/* One save per wizard, with callbacks scoped to that dialog lifetime. */
(() => {
 'use strict';
 function mount({form,submit,status,error,onSettled=()=>{}}){
  let disposed=false,pending=false,timer=null,restore=null;
  function unlock(){clearTimeout(timer);timer=null;if(restore){restore();restore=null}}
  return {
   get pending(){return pending},
   dispose(){disposed=true;unlock()},
   async run(request,onSaved){
    if(disposed||pending)return;
    pending=true;error.textContent='';
    const controls=[...form.querySelectorAll('input,select,textarea,button')].filter(el=>!['modalClose','modalCancel'].includes(el.id));
    const states=controls.map(el=>[el,el.disabled]),label=submit.textContent;
    controls.forEach(el=>el.disabled=true);
    submit.textContent='Saving…';submit.setAttribute('aria-busy','true');
    restore=()=>{for(const [el,disabled] of states)el.disabled=disabled;submit.textContent=label;submit.removeAttribute('aria-busy')};
    status.textContent='Saving account… Please wait.';
    // Do not abort or retry a mutation whose server outcome may be unknown.
    timer=setTimeout(()=>{if(!disposed)status.textContent='Still waiting for the save result. Do not submit again.'},10000);
    let result;
    try{result=await request()}
    catch(e){
     if(!disposed){error.textContent=e.message||'The save result could not be confirmed.';status.textContent='Save was not confirmed. Your entries are preserved. Review the error; if the connection was interrupted, check Accounts before retrying.'}
     return;
    }finally{pending=false;if(!disposed)unlock();onSettled()}
    if(!disposed)onSaved(result);
   }
  };
 }
 const api={mount};
 if(typeof module==='object'&&module.exports)module.exports=api;else window.AccountSubmission=api;
})();
