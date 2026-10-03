package main

import (
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"testing"
)

func TestScaleInventoryPreservesManualAndRejectsDrift(t *testing.T) {
	manual := sanaei.Client{ID: "manual", Email: "manual", Enable: true}
	owned := sanaei.Client{ID: "owned", Email: "owned", Enable: true, TotalGB: 100, LimitHWID: 2}
	baseline := map[string]sanaei.Client{"manual": manual}
	wanted := map[string]sanaei.Client{"owned": owned}
	runtime := owned
	runtime.LimitHWID = 0
	current := map[string]sanaei.Client{"manual": manual, "owned": runtime}
	if err := scaleMatch(current, baseline, wanted); err != nil {
		t.Fatal(err)
	}
	current["manual"] = owned
	if scaleMatch(current, baseline, wanted) == nil {
		t.Fatal("manual drift accepted")
	}
	current["manual"] = manual
	runtime.Email = "conflict"
	current["owned"] = runtime
	if scaleMatch(current, baseline, wanted) == nil {
		t.Fatal("owned conflict accepted")
	}
	delete(current, "owned")
	if scaleMatch(current, baseline, wanted) == nil {
		t.Fatal("missing client accepted")
	}
	if err := scaleMatch(map[string]sanaei.Client{"manual": manual}, baseline, map[string]sanaei.Client{}); err != nil {
		t.Fatal(err)
	}
}
