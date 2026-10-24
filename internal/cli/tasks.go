package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dszdsxc19/hell-mode/internal/fault"
	"github.com/dszdsxc19/hell-mode/internal/model"
	"github.com/spf13/cobra"
)

type armInput struct {
	Goal             string   `json:"goal"`
	Verify           []string `json:"verify"`
	Workdir          string   `json:"workdir"`
	Deadline         string   `json:"deadline"`
	HeartbeatTimeout string   `json:"heartbeat_timeout"`
	VerifyTimeout    string   `json:"verify_timeout"`
	Channel          string   `json:"channel"`
	Interval         string   `json:"interval"`
	Window           string   `json:"window"`
	MaxAttempts      int      `json:"max_attempts"`
}

// parseTime accepts explicit RFC3339 instants or documented relative +duration values.
func parseTime(value string) (time.Time, error) {
	if strings.HasPrefix(value, "+") {
		d, err := time.ParseDuration(value[1:])
		if err != nil {
			return time.Time{}, fault.New("invalid_argument", 2, "invalid relative time: %s", err)
		}
		return time.Now().UTC().Add(d), nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fault.New("invalid_argument", 2, "time must be RFC3339 (with timezone) or +duration")
	}
	return t.UTC(), nil
}

func (a *app) armCommand() *cobra.Command {
	input := armInput{}
	var file, verifyJSON string
	var yes, dry bool
	cmd := &cobra.Command{Use: "arm", Short: "Freeze a goal contract and bounded notification policy", Long: `Create a task. Default channel log records simulations only. Monitoring runs
only while hell watch is alive; arm itself does not launch a daemon.
Use either --file (strict JSON, - for stdin) or individual contract flags.
Verification is an argv array, never an implicitly evaluated shell string.
--yes confirms the caller's chosen contract; --dry-run never writes state.`, Example: `  hell arm --goal "tests pass" --verify-json '["go","test","./..."]' --deadline +10m --dry-run --json
  hell arm --file contract.json --yes --json
  hell arm --file - --yes --no-input --json`, Args: args(cobra.NoArgs), RunE: func(cmd *cobra.Command, _ []string) error {
		if file != "" {
			for _, name := range []string{"goal", "verify-json", "workdir", "deadline", "heartbeat-timeout", "verify-timeout", "channel", "interval", "window", "max-attempts"} {
				if cmd.Flags().Changed(name) {
					return fault.New("invalid_argument", 2, "--file cannot be combined with --%s", name)
				}
			}
			if err := a.decodeInput(file, &input); err != nil {
				return err
			}
		} else {
			if err := json.Unmarshal([]byte(verifyJSON), &input.Verify); err != nil {
				return fault.New("invalid_argument", 2, "--verify-json must be a JSON argv array")
			}
		}
		if input.Workdir == "" {
			return fault.New("invalid_argument", 2, "workdir must not be empty")
		}
		deadline, err := parseTime(input.Deadline)
		if err != nil {
			return err
		}
		durations := make([]time.Duration, 4)
		for i, v := range []string{input.HeartbeatTimeout, input.VerifyTimeout, input.Interval, input.Window} {
			durations[i], err = time.ParseDuration(v)
			if err != nil {
				return fault.New("invalid_argument", 2, "invalid duration %q", v)
			}
		}
		t := model.Task{Goal: input.Goal, Verify: input.Verify, Workdir: input.Workdir, Deadline: deadline, HeartbeatTimeout: durations[0], VerifyTimeout: durations[1], Policy: model.Policy{Channel: input.Channel, Interval: durations[2], Window: durations[3], MaxAttempts: input.MaxAttempts}}
		e := a.engine()
		if err := e.ValidateTask(&t); err != nil {
			return err
		}
		if dry {
			a.data = map[string]any{"dry_run": true, "contract": t}
			return nil
		}
		if !yes {
			return fault.New("confirmation_required", 2, "inspect with --dry-run, then pass --yes to confirm this contract")
		}
		created, err := e.Arm(t)
		a.data = created
		return err
	}}
	f := cmd.Flags()
	f.StringVar(&file, "file", "", "Read a JSON contract file, or - for stdin (cannot mix contract flags)")
	f.StringVar(&input.Goal, "goal", "", "Human-defined target result")
	f.StringVar(&verifyJSON, "verify-json", "", "Frozen verification argv as JSON, e.g. '[\"go\",\"test\",\"./...\"]'")
	f.StringVar(&input.Workdir, "workdir", ".", "Existing working directory for verification and runner")
	f.StringVar(&input.Deadline, "deadline", "", "RFC3339 timestamp with timezone or +duration, e.g. +10m")
	f.StringVar(&input.HeartbeatTimeout, "heartbeat-timeout", "30s", "Maximum gap between runner heartbeats")
	f.StringVar(&input.VerifyTimeout, "verify-timeout", "2m", "Verification process timeout")
	f.StringVar(&input.Channel, "channel", "log", "log (simulation), pushover, or an explicitly registered plugin")
	f.StringVar(&input.Interval, "interval", "1m", "Interval between application send attempts (Pushover: at least 30s)")
	f.StringVar(&input.Window, "window", "10m", "Alarm lifetime, starting with the first incident")
	f.IntVar(&input.MaxAttempts, "max-attempts", 3, "Total application send attempts, including failed/unknown outcomes")
	f.BoolVar(&yes, "yes", false, "Confirm the supplied goal and policy without prompting")
	f.BoolVar(&dry, "dry-run", false, "Validate and print the plan without creating state")
	return cmd
}

func (a *app) statusCommand() *cobra.Command {
	return &cobra.Command{Use: "status [TASK_ID]", Short: "Read tasks, evidence and notification receipts (unmet goals still exit 0)", Example: "  hell status --json\n  hell status task_123 --json", Args: args(cobra.MaximumNArgs(1)), RunE: func(_ *cobra.Command, values []string) error {
		e := a.engine()
		var err error
		if len(values) == 0 {
			a.data, err = e.List()
		} else {
			a.data, err = e.Get(values[0])
		}
		return err
	}}
}

func (a *app) verifyCommand() *cobra.Command {
	return &cobra.Command{Use: "verify TASK_ID", Short: "Run the frozen verification argv; exit 3 if unmet, 1 if unknown", Long: "Executes the task's configured command locally in its frozen workdir. May change goal evidence and open or resolve an incident.", Example: "  hell verify task_123 --json", Args: args(cobra.ExactArgs(1)), RunE: func(cmd *cobra.Command, values []string) error {
		t, err := a.engine().Verify(cmd.Context(), values[0], "")
		a.data = t
		if err != nil {
			return err
		}
		return verdictError(t)
	}}
}

func verdictError(t *model.Task) error {
	if t.Verdict == "unmet" {
		return fault.New("goal_unmet", 3, "verification did not meet the goal; inspect task checks")
	}
	if t.Verdict == "unknown" {
		return fault.New("outcome_unknown", 1, "verification could not establish a result; inspect task checks")
	}
	return nil
}

func (a *app) actionCommand(action string) *cobra.Command {
	var until string
	cmd := &cobra.Command{Use: action + " TASK_ID", Short: map[string]string{"takeover": "Stop this alarm because the human is taking responsibility", "snooze": "Defer an active alarm until an explicit time", "disarm": "Stop monitoring without changing the goal verdict"}[action], Long: "Human management action. Requires the human's instruction; agent run-event credentials do not authorize it. Goal evidence remains unchanged.", Example: "  hell " + action + " task_123 --json", Args: args(cobra.ExactArgs(1)), RunE: func(cmd *cobra.Command, values []string) error {
		var at time.Time
		var err error
		if action == "snooze" {
			at, err = parseTime(until)
			if err != nil {
				return err
			}
		}
		a.data, err = a.engine().Act(cmd.Context(), values[0], action, at)
		return err
	}}
	if action == "snooze" {
		cmd.Flags().StringVar(&until, "until", "", "RFC3339 timestamp or +duration, e.g. +5m")
		cmd.Example = "  hell snooze task_123 --until +5m --json"
	}
	return cmd
}

func (a *app) eventCommand() *cobra.Command {
	var file string
	cmd := &cobra.Command{Use: "event", Short: "Submit a versioned run observation using HELL_EVENT_TOKEN", Long: `Read a strict event object from --file (- for stdin). Required fields:
id, task_id, run_id, type, source. Types: heartbeat, exited, failed, waiting,
turn_completed. Optional fields: exit_code, background_work.
The per-run token comes from hell run and is read only from HELL_EVENT_TOKEN.
Stale/duplicate events are harmless. End events enqueue verification for hell watch.`, Example: "  hell event --file event.json --json", Args: args(cobra.NoArgs), RunE: func(cmd *cobra.Command, _ []string) error {
		if file == "" {
			return fault.New("invalid_argument", 2, "--file is required")
		}
		var event model.Event
		if err := a.decodeInput(file, &event); err != nil {
			return err
		}
		return a.applyEvent(cmd.Context(), event)
	}}
	cmd.Flags().StringVar(&file, "file", "", "Event JSON path or - for stdin")
	return cmd
}

func (a *app) applyEvent(ctx context.Context, event model.Event) error {
	e := a.engine()
	_, err := e.Event(event, os.Getenv("HELL_EVENT_TOKEN"))
	if err != nil {
		return err
	}
	a.data, err = e.Get(event.TaskID)
	return err
}

func (a *app) watchCommand() *cobra.Command {
	var once bool
	var interval time.Duration
	cmd := &cobra.Command{Use: "watch", Short: "Run the independent local watchdog (foreground; --once for schedulers)", Long: `Persist and recover bounded notification attempts in a separate process.
This local implementation shares --state-dir with the runner; it cannot monitor
a powered-off machine remotely. Supervise it externally when experimenting.
With --json, continuous mode writes one envelope per tick as NDJSON.
AI conversations generate separately so model latency does not block task scans.
--once waits for generation and sends before returning.
end makes this watch exit silently at its next state check. An ended watch
does not restart when the human returns. No process detaches itself and no
system service is installed.`, Example: "  hell watch --json\n  hell watch --once --json", Args: args(cobra.NoArgs), RunE: func(cmd *cobra.Command, _ []string) error {
		if interval < time.Millisecond {
			return fault.New("invalid_argument", 2, "--interval must be at least 1ms")
		}
		e := a.engine()
		started, err := e.Presence()
		if err != nil {
			return err
		}
		e.WatchEpoch = &started.WatchEpoch
		watchCtx, cancelWatch := context.WithCancel(cmd.Context())
		type engagementResult struct {
			count int
			err   error
		}
		generated := make(chan engagementResult, 1)
		generating := false
		defer func() {
			cancelWatch()
			if generating {
				// Cancellation stops the bounded child process before the CLI exits.
				select {
				case <-generated:
				case <-time.After(2 * time.Second):
				}
			}
		}()
		for {
			if !once && watchCtx.Err() != nil {
				a.data, a.silent = nil, true
				return nil
			}
			var count int
			if once {
				count, err = e.Tick(watchCtx)
			} else {
				count, err = e.TickTasks(watchCtx)
			}
			if err != nil {
				return err
			}
			presence, err := e.Presence()
			if err != nil {
				return err
			}
			stopped := presence.Mode == "ended" || presence.WatchEpoch != started.WatchEpoch
			a.data = map[string]any{"attempts_claimed": count, "at": time.Now().UTC(), "stopped": stopped}
			if once {
				return nil
			}
			if stopped {
				a.silent = true
				return nil
			}
			if generating {
				select {
				case result := <-generated:
					generating = false
					if result.err != nil {
						return result.err
					}
					count += result.count
				default:
				}
			}
			if !generating {
				generating = true
				go func() {
					count, err := e.TickEngagements(watchCtx)
					generated <- engagementResult{count, err}
				}()
			}
			a.data = map[string]any{"attempts_claimed": count, "at": time.Now().UTC(), "stopped": false}
			if a.json {
				if err := json.NewEncoder(a.out).Encode(map[string]any{"schema_version": "hell-mode/v1", "ok": true, "command": "hell watch", "data": a.data, "error": nil}); err != nil {
					return err
				}
			} else if count > 0 {
				fmt.Fprintf(a.errOut, "watchdog claimed %d notification attempt(s)\n", count)
			}
			select {
			case <-cmd.Context().Done():
				a.data = nil
				a.silent = true
				return nil
			case <-time.After(interval):
			}
		}
	}}
	cmd.Flags().BoolVar(&once, "once", false, "Perform one durable tick and exit")
	cmd.Flags().DurationVar(&interval, "interval", time.Second, "Time between watchdog scans")
	return cmd
}
