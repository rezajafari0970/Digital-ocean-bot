package healthverify

import (
	"context"
	"fmt"
	"os/exec"
)

type LocalRunner struct{}

func (LocalRunner) Run(ctx context.Context, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := string(out)
		if len(msg) > 1200 {
			msg = msg[len(msg)-1200:]
		}
		return msg, fmt.Errorf("health command: %w: %s", err, msg)
	}
	return string(out), nil
}
