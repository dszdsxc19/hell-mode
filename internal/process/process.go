// Package process bounds child output and gives cancellation a process-tree boundary.
package process

import "bytes"

// Buffer retains a bounded prefix while continuing to drain pipes, avoiding child deadlocks.
type Buffer struct {
	bytes.Buffer
	Limit     int
	Truncated bool
}

func (b *Buffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.Limit - b.Len()
	if remaining < n {
		b.Truncated = true
	}
	if remaining > 0 {
		if remaining < n {
			p = p[:remaining]
		}
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
}
