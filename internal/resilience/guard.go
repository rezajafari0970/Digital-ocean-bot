package resilience

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// TaskPanicMarker is persisted; scheduler bypasses must respect its retry fence.
const TaskPanicMarker = "TASK_PANIC_CONTAINED"

var ErrTaskPanic = errors.New("task panic contained; remote outcome requires reconciliation")

type taskPanic struct{ location string }

func (p taskPanic) Error() string {
	return fmt.Sprintf("%s: %s: %s", TaskPanicMarker, ErrTaskPanic, p.location)
}
func (taskPanic) Unwrap() error             { return ErrTaskPanic }
func (taskPanic) RetryDelay() time.Duration { return 30 * time.Second }

// Guard is a task boundary, not a retry. It preserves the original error and
// never prints panic values or arguments, which may include credentials. The
// owner must durably record failure and reconcile any remote mutation outcome.
func Guard(ctx context.Context, fn func(context.Context) error) (err error) {
	defer func() {
		if recover() == nil {
			return
		}
		location := "unknown"
		var pcs [24]uintptr
		frames := runtime.CallersFrames(pcs[:runtime.Callers(3, pcs[:])])
		for {
			frame, more := frames.Next()
			if !strings.HasPrefix(frame.Function, "runtime.") && !strings.Contains(frame.Function, "/resilience.Guard") {
				location = fmt.Sprintf("%s (%s:%d)", frame.Function, filepath.Base(frame.File), frame.Line)
				break
			}
			if !more {
				break
			}
		}
		err = taskPanic{location: location}
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn(ctx)
}
