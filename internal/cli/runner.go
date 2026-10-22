package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/dszdsxc19/hell-mode/internal/fault"
	"github.com/dszdsxc19/hell-mode/internal/model"
	"github.com/dszdsxc19/hell-mode/internal/process"
	"github.com/spf13/cobra"
)

// runCommand reserves stdout for the result and captures child output in private run logs.
func (a *app) runCommand() *cobra.Command {
	var rearm bool
	cmd := &cobra.Command{Use: "run TASK_ID -- COMMAND [ARG...]", Short: "Wrap an agent process with scoped events, heartbeats and final verification", Long: `Run an argv directly, without an implicit shell. The child receives
HELL_TASK_ID, HELL_RUN_ID, HELL_EVENT_TOKEN and HELL_STATE_DIR for hooks.
Child output is stored under state-dir/runs, and mirrored to stderr in human
mode. JSON mode never mixes child output with the result envelope.
Use --no-input for agents that must not read stdin. Real notification channels
require a recently seen watchdog; log simulations can run without one.
--rearm explicitly resumes a stopped task with the same contract and deadline.`, Example: `  hell run task_123 --json --no-input -- codex exec "fix the tests"
  hell run task_123 -- claude
  hell run task_123 --rearm -- sh -c 'exit 1'`, Args: args(cobra.MinimumNArgs(2)), RunE: func(cmd *cobra.Command, values []string) error {
		if cmd.ArgsLenAtDash() != 1 {
			return fault.New("invalid_argument", 2, "use run TASK_ID -- COMMAND [ARG...]")
		}
		e := a.engine()
		original, err := e.Get(values[0])
		if err != nil {
			return err
		}
		if original.Policy.Channel != "log" {
			st, err := e.Store.Snapshot()
			if err != nil {
				return err
			}
			if time.Since(st.WatchdogAt) > 5*time.Second {
				return fault.New("watchdog_unavailable", 5, "start hell watch with this state-dir before using a real notification channel")
			}
		}
		t, token, err := e.StartRun(values[0], rearm)
		if err != nil {
			return err
		}
		logDir := filepath.Join(a.dir, "runs")
		if err := os.MkdirAll(logDir, 0700); err != nil {
			return err
		}
		logPath := filepath.Join(logDir, t.CurrentRunID+".log")
		log, err := os.OpenFile(logPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		defer log.Close()
		child := process.Command(cmd.Context(), values[1:])
		child.Dir = t.Workdir
		child.Env = append(os.Environ(), "HELL_TASK_ID="+t.ID, "HELL_RUN_ID="+t.CurrentRunID, "HELL_EVENT_TOKEN="+token, "HELL_STATE_DIR="+a.dir)
		if !a.noInput {
			child.Stdin = a.in
		}
		var output io.Writer = log
		if !a.json {
			output = io.MultiWriter(log, a.errOut)
		}
		child.Stdout, child.Stderr = output, output
		if err := child.Start(); err != nil {
			code := -1
			_, _ = e.Event(model.Event{ID: model.ID("event"), TaskID: t.ID, RunID: t.CurrentRunID, Type: "failed", Source: "runner", ExitCode: &code}, token)
			a.data = map[string]any{"task_id": t.ID, "run_id": t.CurrentRunID, "log_path": logPath}
			return fault.New("child_start_failed", 1, "could not start child: %s", err)
		}
		done := make(chan error, 1)
		go func() { done <- child.Wait() }()
		heartbeat := t.HeartbeatTimeout / 3
		if heartbeat > 5*time.Second {
			heartbeat = 5 * time.Second
		}
		if heartbeat < time.Millisecond {
			heartbeat = time.Millisecond
		}
		ticker := time.NewTicker(heartbeat)
		defer ticker.Stop()
		var runErr error
	loop:
		for {
			select {
			case runErr = <-done:
				break loop
			case <-ticker.C:
				if _, err := e.Event(model.Event{ID: model.ID("event"), TaskID: t.ID, RunID: t.CurrentRunID, Type: "heartbeat", Source: "runner"}, token); err != nil {
					fmt.Fprintln(a.errOut, "heartbeat could not be persisted")
				}
			}
		}
		exitCode := child.ProcessState.ExitCode()
		eventType := "exited"
		if cmd.Context().Err() != nil {
			eventType = "failed"
		}
		_, err = e.Event(model.Event{ID: model.ID("event"), TaskID: t.ID, RunID: t.CurrentRunID, Type: eventType, Source: "runner", ExitCode: &exitCode}, token)
		if err != nil {
			return err
		}
		if cmd.Context().Err() != nil {
			a.data = map[string]any{"task_id": t.ID, "run_id": t.CurrentRunID, "child_exit_code": exitCode, "log_path": logPath}
			return fault.New("interrupted", 1, "runner interrupted; inspect task state before retrying")
		}
		// A watchdog may already be handling a Stop event; wait for its lease, then verify final output.
		var result *model.Task
		for {
			result, err = e.Verify(cmd.Context(), t.ID, t.CurrentRunID)
			var typed *fault.Error
			if !errors.As(err, &typed) || typed.Code != "verification_busy" {
				break
			}
			select {
			case <-cmd.Context().Done():
				return fault.New("interrupted", 1, "interrupted while waiting for verification")
			case <-time.After(20 * time.Millisecond):
			}
		}
		a.data = map[string]any{"task": result, "child_exit_code": exitCode, "log_path": logPath}
		if err != nil {
			return err
		}
		if err := verdictError(result); err != nil {
			return err
		}
		if runErr != nil {
			return fault.New("child_failed", 3, "child exited unsuccessfully; the separate verification result is recorded in task.verdict")
		}
		return nil
	}}
	cmd.Flags().BoolVar(&rearm, "rearm", false, "Explicitly re-enable a stopped task with its existing goal and deadline")
	return cmd
}
