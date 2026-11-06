/* Native local editing controls; only user input and bounded editing state cross the bridge. */
(() => {
 'use strict';
 window.ProviderInput={create({canvas,viewport,action,getScreen,connected,message,fit}){
  const q=s=>document.querySelector(s),single=q('[data-ime]'),multi=q('[data-ime-multiline]');
  let editor=single,focus=null,remoteFocus=null,selection='',regions=[],gesture=null,remoteTarget='';
  let waiting=false,composing=false,revision=0,ack=0,sending=false,editTimer=0,blurTimer=0,lastSelection='',startupDeletes=0,editorOpen=false,hoverTime=0,scrollDelta=0,scrolling=false,pointerBusy=false,pointerNext=null;
  const supported=['Enter','Tab','Backspace','Delete','Escape','ArrowUp','ArrowDown','ArrowLeft','ArrowRight','Home','End'];
  const point=e=>{
   const r=canvas.getBoundingClientRect(),s=getScreen(),scale=Math.min(r.width/s.width,r.height/s.height);
   return {x:Math.max(0,Math.min(s.width-1,(e.clientX-r.left)/scale)),y:Math.max(0,Math.min(s.height-1,(e.clientY-r.top)/scale))};
  };
  function setPosition(rect){
   const c=canvas.getBoundingClientRect(),v=viewport.getBoundingClientRect(),s=getScreen(),k=Math.min(c.width/s.width,c.height/s.height);
   Object.assign(editor.style,{left:(c.left-v.left+rect.x*k)+'px',top:(c.top-v.top+rect.y*k)+'px',width:Math.max(1,rect.width*k)+'px',height:Math.max(1,rect.height*k)+'px'});
   for(const [key,value]of Object.entries(rect.style||{})){
    if(['fontSize','lineHeight','letterSpacing','paddingLeft','paddingRight','paddingTop','paddingBottom'].includes(key)){
     if(/^-?[\d.]+px$/.test(value))editor.style[key]=(parseFloat(value)*k)+'px';
    }else if(['fontFamily','fontWeight','fontStyle','textAlign','direction'].includes(key))editor.style[key]=value;
   }
   editor.style.caretColor=rect.style?.color||'#172033';
  }
  function clear(){
   clearTimeout(editTimer);clearTimeout(blurTimer);waiting=false;editorOpen=false;focus=null;revision=ack=0;startupDeletes=0;
   for(const e of [single,multi]){e.value='';e.blur();e.hidden=true}
  }
  function prepare(rect){
   clearTimeout(blurTimer);editorOpen=true;waiting=true;revision=ack=0;startupDeletes=0;focus=null;
   editor=single;multi.hidden=true;single.type='text';single.inputMode='text';single.value='';single.hidden=false;
   setPosition(rect);single.focus({preventScroll:true});
  }
  function seed(field){
   const buffered=editor.value;let value=field.value,start=field.start,end=field.end;
   while(startupDeletes-->0){if(start===end&&start>0){start-=value.codePointAt(start-2)>65535?2:1}value=value.slice(0,start)+value.slice(end);end=start}
   const next=field.multiline?multi:single;
   if(next!==editor){editor.hidden=true;editor=next;editor.hidden=false}
   if(editor===single)editor.type=field.password?'password':'text';
   editor.inputMode=['email','tel','url','numeric','decimal','search'].includes(field.input_mode)?field.input_mode:'text';
   editor.value=buffered?value.slice(0,start)+buffered+value.slice(end):value;
   const caret=buffered?start+buffered.length:start;
   editor.setSelectionRange(caret,buffered?caret:end);
   setPosition(field);editor.focus({preventScroll:true});waiting=false;focus=field;
   if(buffered||editor.value!==field.value){revision++;schedule()}else{ack=revision;lastSelection=editor.selectionStart+':'+editor.selectionEnd}
  }
  function sync(field){
   if(!editorOpen)return;
   if(waiting){if(field)seed(field);else clear();return}
   if(!field||!focus||field.token!==focus.token){clear();return}
   focus=field;setPosition(field);
   if(revision===ack&&!sending&&!composing&&document.activeElement===editor&&editor.value!==field.value){
    editor.value=field.value;editor.setSelectionRange(field.start,field.end);
   }
  }
  function schedule(){clearTimeout(editTimer);editTimer=setTimeout(flush,35)}
  async function flush(){
   clearTimeout(editTimer);
   if(!editorOpen||waiting||composing||sending||!focus||revision===ack)return;
   const field=focus.token,value=editor.value,start=editor.selectionStart,end=editor.selectionEnd,version=revision;
   sending=true;
   try{
    const frame=await action(()=>{
     if(!remoteFocus||remoteFocus.token!==field)throw Error('The selected field changed. Select it again.');
     return {kind:value===remoteFocus.value?'select':'edit',field,expected:remoteFocus.value,text:value,selection_start:start,selection_end:end};
    });
    if(focus?.token===field)ack=version;
    if(focus?.token===field&&revision===version&&frame?.focused?.token===field){
     focus=frame.focused;if(editor.value!==focus.value)editor.value=focus.value;
     // Keep the native caret/selection: email and number controls do not expose selectionStart remotely.
     editor.setSelectionRange(Math.min(start,editor.value.length),Math.min(end,editor.value.length));
    }
   }catch(e){message(e.message||'The edit could not be sent. Select the field again.');clear()}
   finally{sending=false;if(revision!==ack)schedule()}
  }
  function afterEdits(payload){
   // Snapshot and queue before a tap opens another local editor or resizes the keyboard.
   clearTimeout(editTimer);
   let edited=Promise.resolve();
   if(editorOpen&&!waiting&&focus&&revision!==ack){
    const field=focus.token,value=editor.value,start=editor.selectionStart,end=editor.selectionEnd,version=revision;
    edited=action(()=>{
     if(!remoteFocus||remoteFocus.token!==field)throw Error('The selected field changed. Select it again.');
     return {kind:value===remoteFocus.value?'select':'edit',field,expected:remoteFocus.value,text:value,selection_start:start,selection_end:end};
    }).then(()=>{if(focus?.token===field)ack=version});
   }
   const next=action(payload);
   if(editorOpen&&payload.kind==='key'&&['Tab','ShiftTab'].includes(payload.key)){
    // Buffer input for the next field. Key-up/selection from the old field must
    // not queue a stale edit after the remote focus has already moved.
    editor.value='';focus=null;revision=ack=0;startupDeletes=0;waiting=true;
   }
   return Promise.all([edited,next]).then(([,frame])=>frame);
  }
  function key(e){
   if(composing||e.isComposing)return;
   if(e.target===editor&&editorOpen){
    if(e.key==='Tab'||(e.key==='Enter'&&editor!==multi)||e.key==='Escape'){
     e.preventDefault();afterEdits({kind:'key',key:e.key==='Tab'&&e.shiftKey?'ShiftTab':e.key}).catch(e=>message(e.message));
     if(e.key==='Escape')clear();
    }
    return; // Native selection, IME, cut/copy/paste, undo and ordinary caret movement.
   }
   if(supported.includes(e.key)){e.preventDefault();action({kind:'key',key:e.key==='Tab'&&e.shiftKey?'ShiftTab':e.key}).catch(e=>message(e.message))}
   else if((e.ctrlKey||e.metaKey)&&e.key.toLowerCase()==='a'){e.preventDefault();action({kind:'key',key:'SelectAll'}).catch(e=>message(e.message))}
   else if(e.key.length===1&&!e.ctrlKey&&!e.metaKey&&!e.altKey){e.preventDefault();action({kind:'text',text:e.key}).catch(e=>message(e.message))}
  }
  for(const e of [single,multi]){
   e.addEventListener('keydown',key);
   e.addEventListener('compositionstart',()=>{composing=true});
   e.addEventListener('compositionend',()=>{composing=false;revision++;schedule()});
   e.addEventListener('beforeinput',event=>{if(waiting&&event.inputType==='deleteContentBackward'&&!e.value)startupDeletes++});
   e.addEventListener('input',()=>{revision++;if(!composing)schedule()});
   const selected=()=>{if(!editorOpen||waiting||e!==editor)return;const next=e.selectionStart+':'+e.selectionEnd;if(next!==lastSelection){lastSelection=next;revision++;schedule()}};
   e.addEventListener('select',selected);e.addEventListener('keyup',selected);e.addEventListener('pointerup',selected);
   e.addEventListener('blur',()=>{blurTimer=setTimeout(flush,0)});
  }
  canvas.addEventListener('keydown',key);
  canvas.addEventListener('paste',e=>{const text=e.clipboardData?.getData('text/plain');if(text){e.preventDefault();action({kind:'text',text}).catch(e=>message(e.message))}});
  canvas.addEventListener('copy',e=>{if(selection){e.preventDefault();e.clipboardData.setData('text/plain',selection)}});
  q('[data-keyboard]').onclick=()=>{
   if(!connected())return;
   if(editorOpen&&focus){editor.hidden=false;editor.focus({preventScroll:true});return}
   if(focus){editor.value='';editorOpen=true;waiting=true;seed(focus);return}
   const r=regions[0];
   if(r){const p={x:r.x+Math.min(10,r.width/2),y:r.y+r.height/2};prepare(r);action({kind:'tap',...p}).catch(e=>{clear();message(e.message)})}
   else message('Tap a text field first. Use Paste in the browser menu for a selected frame field.');
  };
  document.querySelectorAll('[data-key]').forEach(b=>b.onclick=()=>{
   if(editorOpen&&b.dataset.key==='SelectAll'){editor.focus({preventScroll:true});editor.select();revision++;schedule();return}
   afterEdits({kind:'key',key:b.dataset.key}).catch(e=>message(e.message));
  });
  q('[data-copy]').onclick=async()=>{
   const text=editorOpen?editor.value.slice(editor.selectionStart,editor.selectionEnd):selection;
   if(!text){message('Select text to copy.');return}
   try{
    if(navigator.clipboard?.writeText)await navigator.clipboard.writeText(text);
    else{
     const t=document.createElement('textarea');t.value=text;t.style.cssText='position:fixed;top:0;left:0;font-size:16px';document.body.append(t);t.select();
     const ok=document.execCommand('copy');t.remove();if(!ok)throw Error();
    }
    message('Copied.');
   }catch{message('Use the Copy command on the selected text.')}
  };
  q('[data-paste]').onclick=()=>{
   // A native, visible paste control also works when Async Clipboard is unavailable (HTTP).
   q('[data-clipboard]').showModal();q('[data-paste-text]').value='';q('[data-paste-text]').focus();
  };
  q('[data-paste-send]').onclick=async()=>{
   const text=q('[data-paste-text]').value;q('[data-clipboard]').close();q('[data-paste-text]').value='';
   if(!text)return;
   try{
    if(editorOpen&&focus){
     editor.setRangeText(text,editor.selectionStart,editor.selectionEnd,'end');revision++;await flush();editor.focus({preventScroll:true});
    }else await action({kind:'text',text});
   }catch(e){message(e.message)}
  };
  q('[data-paste-cancel]').onclick=()=>{q('[data-paste-text]').value='';q('[data-clipboard]').close()};
  function movePointer(payload){pointerNext=payload;if(pointerBusy)return;const next=pointerNext;pointerNext=null;pointerBusy=true;action(next).catch(e=>message(e.message)).finally(()=>{pointerBusy=false;if(pointerNext)movePointer(pointerNext)})}
  canvas.onpointerdown=e=>{
   if(!connected()||gesture||(e.pointerType==='mouse'&&e.button!==0))return;e.preventDefault();pointerNext=null;
   const p=point(e),r=canvas.getBoundingClientRect(),s=getScreen();
   gesture={id:e.pointerId,touch:e.pointerType==='touch',start:p,last:p,startX:e.clientX,startY:e.clientY,moved:false,mouseDown:false,scale:Math.min(r.width/s.width,r.height/s.height)};
   canvas.setPointerCapture(e.pointerId);
   // No remote down yet: a screenshot/network round trip must not turn a tap into a long press.
  };
  function drainScroll(p){
   if(scrolling||!scrollDelta)return;
   const delta=Math.max(-1200,Math.min(1200,scrollDelta));scrollDelta-=delta;scrolling=true;
   action({kind:'scroll',...p,delta}).catch(e=>message(e.message)).finally(()=>{scrolling=false;drainScroll(p)});
  }
  canvas.onpointermove=e=>{
   if(!connected())return;
   if(!gesture){if(e.pointerType==='mouse'&&Date.now()-hoverTime>150){hoverTime=Date.now();movePointer({kind:'pointer',phase:'move',buttons:0,...point(e)})}return}
   if(e.pointerId!==gesture.id)return;e.preventDefault();const g=gesture,p={x:g.start.x+(e.clientX-g.startX)/g.scale,y:g.start.y+(e.clientY-g.startY)/g.scale},s=getScreen();
   p.x=Math.max(0,Math.min(s.width-1,p.x));p.y=Math.max(0,Math.min(s.height-1,p.y));
   if(Math.hypot(e.clientX-g.startX,e.clientY-g.startY)>8)g.moved=true;
   if(g.moved){
    if(g.touch){scrollDelta+=g.last.y-p.y;drainScroll(g.start)}
    else{
     if(!g.mouseDown){g.mouseDown=true;action({kind:'pointer',phase:'down',buttons:1,...g.start}).catch(e=>message(e.message))}
     movePointer({kind:'pointer',phase:'move',buttons:1,...p});
    }
   }
   g.last=p;
  };
  function release(e,cancelled){
   if(!gesture||e.pointerId!==gesture.id)return;e.preventDefault();const g=gesture;gesture=null;pointerNext=null;
   if(g.mouseDown)action({kind:'pointer',phase:'move',buttons:1,...g.last}).catch(e=>message(e.message));
   if(g.mouseDown)action({kind:'pointer',phase:'up',buttons:0,...g.last}).catch(e=>message(e.message));
   if(cancelled||g.moved){fit();return}
   const p=g.start,rect=regions.find(r=>p.x>=r.x&&p.y>=r.y&&p.x<=r.x+r.width&&p.y<=r.y+r.height);
   // Enqueue the atomic remote tap before opening the local keyboard can resize the viewport.
   const result=afterEdits({kind:'tap',touch:g.touch,...p});
   if(rect)prepare(rect);else{clear();canvas.focus({preventScroll:true})}
   result.catch(e=>{clear();message(e.message)});fit();
  }
  canvas.onpointerup=e=>release(e,false);canvas.onpointercancel=e=>release(e,true);
  canvas.addEventListener('wheel',e=>{e.preventDefault();scrollDelta+=Math.max(-1200,Math.min(1200,e.deltaY));drainScroll(point(e))},{passive:false});
  return {
   update(frame,source){
    if(remoteTarget&&frame.target!==remoteTarget)clear();
    remoteTarget=frame.target;regions=Array.isArray(frame.editable)?frame.editable:[];selection=frame.selection||'';
    const next=frame.focused||null;remoteFocus=next;
    const focusAction=source?.kind==='tap'||(source?.kind==='key'&&['Tab','ShiftTab'].includes(source.key));
    if(waiting&&!focusAction)return;
    if(editorOpen)sync(next);else focus=next;
   },
   isGesturing:()=>Boolean(gesture),
   reset(){clear();remoteFocus=null;selection='';regions=[];gesture=null;scrollDelta=0},
   destroy(){clear();remoteFocus=null;selection='';regions=[];gesture=null}
  };
 }};
})();
