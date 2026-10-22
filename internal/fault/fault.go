// Package fault defines stable CLI error codes independently of presentation.
package fault

import "fmt"

type Error struct {
	Code    string
	Message string
	Exit    int
}

func (e *Error) Error() string { return e.Message }

// New supplies actionable messages while keeping subprocess and transport details private.
func New(code string, exit int, format string, args ...any) error {
	return &Error{Code: code, Exit: exit, Message: fmt.Sprintf(format, args...)}
}
