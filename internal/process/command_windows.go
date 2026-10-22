package process

import (
	"context"
	"os/exec"
	"time"
)

// Command bounds waiting on inherited pipes; Windows descendant cleanup is not guaranteed.
func Command(ctx context.Context, argv []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.WaitDelay = time.Second
	return cmd
}
