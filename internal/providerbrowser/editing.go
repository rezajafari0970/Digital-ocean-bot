package providerbrowser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// Only the currently focused text control is mirrored to its authenticated owner.
// This transient editing state is never logged or persisted. Cookies, arbitrary
// DOM and executable scripts are not exposed through the input protocol.
type FocusedInput struct {
	Token     string            `json:"token"`
	Value     string            `json:"value"`
	Start     int64             `json:"start"`
	End       int64             `json:"end"`
	Password  bool              `json:"password"`
	Multiline bool              `json:"multiline"`
	InputMode string            `json:"input_mode"`
	X         float64           `json:"x"`
	Y         float64           `json:"y"`
	Width     float64           `json:"width"`
	Height    float64           `json:"height"`
	Style     map[string]string `json:"style"`
}

const activeTextControl = `(() => {
 let el=document.activeElement;
 for(let n=0;n<12;n++){
  if(el?.shadowRoot?.activeElement){el=el.shadowRoot.activeElement;continue}
  if(el?.tagName==='IFRAME'){try{const next=el.contentDocument?.activeElement;if(next){el=next;continue}}catch{}}
  break;
 }
 if(!el||el.disabled||el.readOnly)return null;
 if(el.tagName==='TEXTAREA'||(el.tagName==='INPUT'&&['text','password','email','search','tel','url','number'].includes(el.type)))return el;
 return null;
})()`

func (w *Worker) focusedObject(ctx context.Context, fn func(context.Context, runtime.RemoteObjectID, *FocusedInput) error) error {
	return w.run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		obj, exception, err := runtime.Evaluate(activeTextControl).WithObjectGroup("provider-input").Do(c)
		if err != nil {
			return err
		}
		if exception != nil {
			return errors.New("focused input unavailable")
		}
		if obj == nil || obj.ObjectID == "" {
			return fn(c, "", nil)
		}
		defer runtime.ReleaseObject(obj.ObjectID).Do(c)
		node, err := dom.DescribeNode().WithObjectID(obj.ObjectID).Do(c)
		if err != nil {
			return err
		}
		data, exception, err := runtime.CallFunctionOn(`function(){
   if(this.value.length>8192)return null;
   const r=this.getBoundingClientRect();let x=r.x,y=r.y,doc=this.ownerDocument;
   for(let n=0;n<12;n++){let frame;try{frame=doc.defaultView.frameElement}catch{}if(!frame)break;const p=frame.getBoundingClientRect();x+=p.x;y+=p.y;doc=frame.ownerDocument}
   const css=this.ownerDocument.defaultView.getComputedStyle(this),style={};
   for(const key of ['fontFamily','fontSize','fontWeight','fontStyle','lineHeight','letterSpacing','textAlign','direction','paddingLeft','paddingRight','paddingTop','paddingBottom','color'])style[key]=css[key];
   return {value:this.value,start:this.selectionStart??this.value.length,end:this.selectionEnd??this.value.length,password:this.type==='password',multiline:this.tagName==='TEXTAREA',input_mode:this.inputMode||this.type||'text',x,y,width:r.width,height:r.height,style};
  }`).WithObjectID(obj.ObjectID).WithReturnByValue(true).Do(c)
		if err != nil {
			return err
		}
		if exception != nil || data == nil {
			return errors.New("focused input unavailable")
		}
		var field *FocusedInput
		if err = json.Unmarshal(data.Value, &field); err != nil {
			return err
		}
		if field != nil {
			if len(field.Value) > 8192 {
				field = nil
			} else {
				field.Token = fmt.Sprintf("%s:%d", chromedp.FromContext(w.current).Target.TargetID, node.BackendNodeID)
			}
		}
		return fn(c, obj.ObjectID, field)
	}))
}

func (w *Worker) edit(ctx context.Context, a Action) error {
	if a.Field == "" || len(a.Text) > 8192 || len(a.Expected) > 8192 || a.SelectionStart < 0 || a.SelectionEnd < a.SelectionStart || a.SelectionEnd > 8192 {
		return errors.New("invalid input edit")
	}
	return w.focusedObject(ctx, func(c context.Context, id runtime.RemoteObjectID, f *FocusedInput) error {
		if f == nil || f.Token != a.Field || f.Value != a.Expected {
			return errors.New("focused input changed")
		}
		if a.Kind == "edit" && f.Value != a.Text {
			if err := selectAllInput().Do(c); err != nil {
				return err
			}
			if a.Text == "" {
				if err := chromedp.KeyEvent(kb.Backspace).Do(c); err != nil {
					return err
				}
			} else if err := input.InsertText(a.Text).Do(c); err != nil {
				return err
			}
		}
		expected := a.Text
		if a.Kind == "select" {
			expected = a.Expected
		}
		args := []*runtime.CallArgument{}
		for _, v := range []any{expected, a.SelectionStart, a.SelectionEnd} {
			b, _ := json.Marshal(v)
			args = append(args, &runtime.CallArgument{Value: b})
		}
		result, exception, err := runtime.CallFunctionOn(`function(value,start,end){
   if(!this.isConnected)return false;
   try{this.setSelectionRange(start,end);return true}catch{return true}
  }`).WithObjectID(id).WithArguments(args).WithReturnByValue(true).Do(c)
		if err != nil {
			return err
		}
		if exception != nil || result == nil || string(result.Value) != "true" {
			return errors.New("focused input changed")
		}
		return nil
	})
}

func (w *Worker) editingState(ctx context.Context, s *State) {
	_ = w.focusedObject(ctx, func(_ context.Context, _ runtime.RemoteObjectID, f *FocusedInput) error { s.Focused = f; return nil })
	// Selection text is bounded and transient, just like the rendered pixels.
	// Password input values never enter the page-selection clipboard path.
	_ = w.run(ctx, chromedp.Evaluate(`String(window.getSelection()?.toString()||'').slice(0,8192)`, &s.Selection))
	if len(s.Selection) > 8192 {
		s.Selection = ""
	}
	s.Selection = strings.ReplaceAll(s.Selection, "\x00", "")
}

// Modifier keys must accompany the key event, not be typed as sequential keys.
func selectAllInput() chromedp.Action {
	return chromedp.Tasks{
		input.DispatchKeyEvent(input.KeyDown).WithKey("a").WithCode("KeyA").WithWindowsVirtualKeyCode(65).WithModifiers(input.ModifierCtrl),
		input.DispatchKeyEvent(input.KeyUp).WithKey("a").WithCode("KeyA").WithWindowsVirtualKeyCode(65).WithModifiers(input.ModifierCtrl),
	}
}
