package rollingreboot

import (
	"context"
	"testing"
)

func TestExecutorDisabledIsNoop(t *testing.T) {
	e := Executor{Enabled: false}
	if err := e.RunOne(context.Background()); err != nil {
		t.Fatal(err)
	}
}
