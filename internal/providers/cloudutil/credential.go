package cloudutil

import (
	"context"
	"encoding/json"
)

type JSONCredential []byte

func (c JSONCredential) Get(context.Context) ([]byte, error) { return append([]byte(nil), c...), nil }
func NewJSONCredential(v any) JSONCredential                 { b, _ := json.Marshal(v); return JSONCredential(b) }
