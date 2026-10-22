// Package engine owns goal evidence, event authority and bounded alarm transitions.
package engine

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/dszdsxc19/hell-mode/internal/fault"
	"github.com/dszdsxc19/hell-mode/internal/model"
	"github.com/dszdsxc19/hell-mode/internal/process"
	"github.com/dszdsxc19/hell-mode/internal/state"
	"github.com/dszdsxc19/hell-mode/pkg/protocol"
)

// Notifier is the extension seam shared by log, Pushover and process plugins.
type Notifier interface {
	Call(context.Context, string, string, protocol.Notification) (*protocol.Result, error)
}
type Engine struct {
	Store    state.Store
	Notifier Notifier
	Now      func() time.Time
}

func (e Engine) now() time.Time {
	if e.Now != nil {
		return e.Now().UTC()
	}
	return time.Now().UTC()
}

// ValidateTask checks real scheduling and execution invariants before any durable mutation.
func (e Engine) ValidateTask(t *model.Task) error {
	if strings.TrimSpace(t.Goal) == "" || len(t.Verify) == 0 || t.Verify[0] == "" {
		return fault.New("invalid_argument", 2, "goal and a non-empty verification argv are required")
	}
	if !t.Deadline.After(e.now()) {
		return fault.New("invalid_argument", 2, "deadline must be in the future")
	}
	if t.HeartbeatTimeout <= 0 || t.VerifyTimeout <= 0 || t.Policy.Interval <= 0 || t.Policy.Window <= 0 || t.Policy.MaxAttempts < 1 {
		return fault.New("invalid_argument", 2, "timeouts, interval, window and max-attempts must be positive")
	}
	if t.Policy.Channel == "" {
		return fault.New("invalid_argument", 2, "channel is required")
	}
	if t.Policy.Channel == "pushover" && t.Policy.Interval < 30*time.Second {
		return fault.New("invalid_argument", 2, "Pushover interval must be at least 30s")
	}
	path, err := filepath.Abs(t.Workdir)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return fault.New("invalid_argument", 2, "workdir must be an existing directory")
	}
	t.Workdir = path
	return nil
}

// Arm freezes the contract; later agent events have no fields for editing it.
func (e Engine) Arm(t model.Task) (*model.Task, error) {
	if err := e.ValidateTask(&t); err != nil {
		return nil, err
	}
	t.ID, t.CreatedAt, t.Armed, t.Verdict = model.ID("task"), e.now(), true, "pending"
	t.Runs, t.Checks = []model.Run{}, []model.Check{}
	err := e.Store.Update(func(st *state.State) error {
		if t.Policy.Channel != "log" && t.Policy.Channel != "pushover" {
			if _, ok := st.Plugins[t.Policy.Channel]; !ok {
				return fault.New("not_found", 4, "register notification plugin %q first", t.Policy.Channel)
			}
		}
		st.Tasks[t.ID] = &t
		return nil
	})
	return &t, err
}

func task(st *state.State, id string) (*model.Task, error) {
	t, ok := st.Tasks[id]
	if !ok {
		return nil, fault.New("not_found", 4, "task %q was not found; use hell status", id)
	}
	return t, nil
}

// Get returns the same durable representation used by event and watchdog operations.
func (e Engine) Get(id string) (*model.Task, error) {
	st, err := e.Store.Snapshot()
	if err != nil {
		return nil, err
	}
	return task(st, id)
}

// List sorts by task identity so machine consumers never rely on Go map iteration order.
func (e Engine) List() ([]*model.Task, error) {
	st, err := e.Store.Snapshot()
	if err != nil {
		return nil, err
	}
	result := []*model.Task{}
	for _, t := range st.Tasks {
		result = append(result, t)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func digest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// StartRun rotates a per-run event credential; old credentials cannot mutate a new attempt.
func (e Engine) StartRun(id string, rearm bool) (*model.Task, string, error) {
	token := model.ID("event")
	var result *model.Task
	err := e.Store.Update(func(st *state.State) error {
		t, err := task(st, id)
		if err != nil {
			return err
		}
		if r := t.CurrentRun(); r != nil && r.State == "running" {
			return fault.New("conflict", 5, "task already has a running attempt")
		}
		if t.Incident != nil {
			if t.Armed {
				return fault.New("conflict", 5, "take over or disarm the current incident before starting another run")
			}
			for _, attempt := range t.Incident.Attempts {
				if attempt.State == "sending" && e.now().Before(attempt.LeaseUntil) {
					return fault.New("conflict", 5, "old notification send is still in flight")
				}
				if attempt.Receipt != "" && !attempt.Cancelled && e.now().Before(t.Incident.ExpiresAt) {
					return fault.New("conflict", 5, "old notification cancellation is pending; keep the watchdog running")
				}
			}
		}
		if !t.Armed && !rearm {
			return fault.New("conflict", 5, "task is not armed; use run --rearm to explicitly resume monitoring")
		}
		if !t.Deadline.After(e.now()) {
			return fault.New("conflict", 5, "task deadline has passed; arm a new task with a future deadline")
		}
		r := model.Run{ID: model.ID("run"), State: "running", StartedAt: e.now(), LastHeartbeat: e.now()}
		t.Armed, t.Verdict, t.CurrentRunID = true, "pending", r.ID
		t.PendingVerification, t.VerificationLeaseUntil = false, time.Time{}
		t.Runs = append(t.Runs, r)
		if t.Incident != nil {
			t.IncidentHistory = append(t.IncidentHistory, *t.Incident)
		}
		t.Incident = nil
		st.Tokens[r.ID] = digest(token)
		result = t
		return nil
	})
	return result, token, err
}

// Event validates authority and deduplicates before deriving any state transition.
func (e Engine) Event(event model.Event, token string) (bool, error) {
	if event.ID == "" || event.TaskID == "" || event.RunID == "" || event.Source == "" || !slices.Contains([]string{"heartbeat", "exited", "failed", "waiting", "turn_completed"}, event.Type) {
		return false, fault.New("invalid_argument", 2, "event requires id, task_id, run_id, source and a supported type")
	}
	needsCheck := false
	err := e.Store.Update(func(st *state.State) error {
		t, err := task(st, event.TaskID)
		if err != nil {
			return err
		}
		if token == "" || subtle.ConstantTimeCompare([]byte(st.Tokens[event.RunID]), []byte(digest(token))) != 1 {
			return fault.New("unauthorized", 5, "invalid run event credential")
		}
		key := event.TaskID + "/" + event.RunID + "/" + event.ID
		if st.Events[key] || t.CurrentRunID != event.RunID {
			return nil
		}
		st.Events[key] = true
		r := t.CurrentRun()
		if r == nil {
			return fault.New("conflict", 5, "run has no durable record")
		}
		switch event.Type {
		case "heartbeat":
			if r.State == "running" {
				r.LastHeartbeat = e.now()
			}
		case "exited", "failed":
			r.State = "exited"
			r.ExitCode = event.ExitCode
			needsCheck = t.Armed && t.Verdict != "met"
			if event.Type == "failed" && t.Armed && t.Verdict != "met" {
				t.Verdict = "unknown"
				openIncident(t, "run_failed", e.now())
			}
		case "turn_completed":
			needsCheck = t.Armed && t.Verdict != "met" && !event.BackgroundWork
		case "waiting":
			openIncident(t, "waiting_for_input", e.now())
		}
		if needsCheck {
			t.PendingVerification = true
		}
		return nil
	})
	return needsCheck, err
}

// openIncident merges repeated failure signals without resetting the notification budget.
func openIncident(t *model.Task, reason string, now time.Time) {
	if !t.Armed {
		return
	}
	if t.Incident != nil {
		t.Incident.Reason = reason
		return
	}
	t.Incident = &model.Incident{ID: model.ID("incident"), State: "open", Reason: reason, OpenedAt: now, ExpiresAt: now.Add(t.Policy.Window), NextAt: now, Attempts: []model.Attempt{}}
}

// Verify executes only the frozen argv, outside storage locks, and fences stale results.
func (e Engine) Verify(ctx context.Context, id, runID string) (*model.Task, error) {
	return e.verify(ctx, id, runID, false)
}

func (e Engine) verify(ctx context.Context, id, runID string, queuedOnly bool) (*model.Task, error) {
	t, err := e.Get(id)
	if err != nil {
		return nil, err
	}
	if runID == "" {
		runID = t.CurrentRunID
	}
	if runID != t.CurrentRunID {
		return t, fault.New("conflict", 5, "verification refers to a stale run")
	}
	err = e.Store.Update(func(st *state.State) error {
		current, err := task(st, id)
		if err != nil {
			return err
		}
		if current.CurrentRunID != runID {
			return fault.New("conflict", 5, "verification refers to a stale run")
		}
		if queuedOnly && !current.PendingVerification {
			return fault.New("conflict", 5, "queued verification was already claimed")
		}
		if e.now().Before(current.VerificationLeaseUntil) {
			return fault.New("verification_busy", 5, "verification is already running; query status before retrying")
		}
		current.PendingVerification = false
		current.VerificationLeaseUntil = e.now().Add(current.VerifyTimeout + 5*time.Second)
		t = current
		return nil
	})
	if err != nil {
		return t, err
	}
	ctx, cancel := context.WithTimeout(ctx, t.VerifyTimeout)
	defer cancel()
	cmd := process.Command(ctx, t.Verify)
	cmd.Dir = t.Workdir
	output := &process.Buffer{Limit: 16 << 10}
	cmd.Stdout, cmd.Stderr = output, output
	runErr := cmd.Run()
	check := model.Check{RunID: runID, At: e.now(), Verdict: "met", ExitCode: 0, Output: output.String()}
	if runErr != nil {
		check.Error = runErr.Error()
		check.Verdict, check.ExitCode = "unmet", -1
		if cmd.ProcessState != nil {
			check.ExitCode = cmd.ProcessState.ExitCode()
		}
		if ctx.Err() != nil || cmd.ProcessState == nil {
			check.Verdict = "unknown"
		}
	}
	if output.Truncated {
		check.Output += "\n[output truncated]"
	}
	err = e.Store.Update(func(st *state.State) error {
		current, err := task(st, id)
		if err != nil {
			return err
		}
		if current.CurrentRunID != runID {
			return fault.New("conflict", 5, "new run started while verifying; stale result discarded")
		}
		current.Checks = append(current.Checks, check)
		current.VerificationLeaseUntil = time.Time{}
		current.Verdict = check.Verdict
		if check.Verdict == "met" {
			current.PendingVerification = false
			current.Armed = false
			if current.Incident != nil {
				current.Incident.State = "resolved"
			}
		} else {
			openIncident(current, "verification_"+check.Verdict, e.now())
		}
		t = current
		return nil
	})
	if err == nil {
		err = e.CancelOutstanding(ctx, id)
	}
	if err == nil {
		t, err = e.Get(id)
	}
	return t, err
}

// Act stops or defers alarms without changing evidence about whether the goal was met.
func (e Engine) Act(ctx context.Context, id, action string, until time.Time) (*model.Task, error) {
	var result *model.Task
	err := e.Store.Update(func(st *state.State) error {
		t, err := task(st, id)
		if err != nil {
			return err
		}
		switch action {
		case "takeover", "disarm":
			t.Armed = false
			if t.Incident != nil {
				if action == "takeover" {
					t.Incident.State = "taken_over"
				} else {
					t.Incident.State = "disarmed"
				}
			}
		case "snooze":
			if t.Incident == nil || !t.Armed || !slices.Contains([]string{"open", "snoozed"}, t.Incident.State) {
				return fault.New("conflict", 5, "there is no active alarm to snooze")
			}
			if !until.After(e.now()) {
				return fault.New("invalid_argument", 2, "snooze time must be in the future")
			}
			t.Incident.State = "snoozed"
			t.Incident.NextAt = until
		default:
			return fault.New("invalid_argument", 2, "unknown human action")
		}
		result = t
		return nil
	})
	if err == nil {
		err = e.CancelOutstanding(ctx, id)
	}
	if err == nil {
		result, err = e.Get(id)
	}
	return result, err
}

// Tick atomically claims attempts, then performs external calls without holding a database lock.
func (e Engine) Tick(ctx context.Context) (int, error) {
	// Host callbacks only enqueue verification; expensive checks run here, outside callback lifetime.
	queued, err := e.List()
	if err != nil {
		return 0, err
	}
	for _, t := range queued {
		if !t.Armed || !t.PendingVerification || e.now().Before(t.VerificationLeaseUntil) {
			continue
		}
		if _, err := e.verify(ctx, t.ID, t.CurrentRunID, true); err != nil {
			if typed, ok := err.(*fault.Error); ok && (typed.Code == "verification_busy" || typed.Code == "conflict") {
				continue
			}
			return 0, err
		}
	}
	type claim struct{ taskID, incidentID, attemptID string }
	claims := []claim{}
	err = e.Store.Update(func(st *state.State) error {
		now := e.now()
		st.WatchdogAt = now
		for _, t := range st.Tasks {
			if !t.Armed || t.Verdict == "met" {
				continue
			}
			if !t.VerificationLeaseUntil.IsZero() {
				if now.Before(t.VerificationLeaseUntil) {
					continue
				}
				t.VerificationLeaseUntil = time.Time{}
				t.Verdict = "unknown"
				openIncident(t, "verification_lost", now)
			}
			if !now.Before(t.Deadline) {
				if t.Verdict == "pending" {
					t.Verdict = "unknown"
				}
				openIncident(t, "deadline_exceeded", now)
			}
			if r := t.CurrentRun(); r != nil && r.State == "running" && now.Sub(r.LastHeartbeat) >= t.HeartbeatTimeout {
				r.State = "lost"
				if t.Verdict == "pending" {
					t.Verdict = "unknown"
				}
				openIncident(t, "heartbeat_lost", now)
			}
			in := t.Incident
			if in == nil {
				continue
			}
			if !now.Before(in.ExpiresAt) {
				in.State = "exhausted"
				t.Armed = false
				continue
			}
			if in.State == "snoozed" && !now.Before(in.NextAt) {
				in.State = "open"
			}
			if in.State != "open" || now.Before(in.NextAt) {
				continue
			}
			inflight := false
			for i := range in.Attempts {
				a := &in.Attempts[i]
				if a.State == "sending" {
					if now.Before(a.LeaseUntil) {
						inflight = true
					} else {
						a.State = "unknown"
						a.Error = "watchdog lost the send result"
					}
				}
			}
			if inflight {
				continue
			}
			if len(in.Attempts) >= t.Policy.MaxAttempts {
				in.State = "exhausted"
				t.Armed = false
				continue
			}
			a := model.Attempt{ID: model.ID("attempt"), State: "sending", At: now, LeaseUntil: now.Add(45 * time.Second)}
			in.Attempts = append(in.Attempts, a)
			in.NextAt = now.Add(t.Policy.Interval)
			claims = append(claims, claim{t.ID, in.ID, a.ID})
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	for _, c := range claims {
		if err := e.send(ctx, c.taskID, c.incidentID, c.attemptID); err != nil {
			return 0, err
		}
	}
	tasks, err := e.List()
	if err != nil {
		return len(claims), err
	}
	for _, t := range tasks {
		if err := e.CancelOutstanding(ctx, t.ID); err != nil {
			return len(claims), err
		}
	}
	return len(claims), nil
}

func (e Engine) send(ctx context.Context, id, incidentID, attemptID string) error {
	t, err := e.Get(id)
	if err != nil {
		return err
	}
	in := t.Incident
	if in != nil && in.ID == incidentID {
		for _, attempt := range in.Attempts {
			if attempt.ID == attemptID && (attempt.State != "sending" || !e.now().Before(attempt.LeaseUntil)) {
				return nil
			}
		}
	}
	if in == nil || in.ID != incidentID || !t.Armed || in.State != "open" {
		return e.Store.Update(func(st *state.State) error {
			current, err := task(st, id)
			if err != nil {
				return err
			}
			if current.Incident != nil && current.Incident.ID == incidentID {
				for i := range current.Incident.Attempts {
					if current.Incident.Attempts[i].ID == attemptID {
						current.Incident.Attempts[i].State = "cancelled"
					}
				}
			}
			return nil
		})
	}
	expires := e.now().Add(t.Policy.Interval)
	if in.ExpiresAt.Before(expires) {
		expires = in.ExpiresAt
	}
	n := protocol.Notification{TaskID: id, IncidentID: incidentID, AttemptID: attemptID, Title: "hell-mode: task needs attention", Body: fmt.Sprintf("Task %s: %s. Inspect with hell status %s. Entertainment only.", id, in.Reason, id), ExpiresAt: expires}
	result, sendErr := e.Notifier.Call(ctx, t.Policy.Channel, "notify", n)
	return e.Store.Update(func(st *state.State) error {
		t, err := task(st, id)
		if err != nil {
			return err
		}
		if t.Incident == nil || t.Incident.ID != incidentID {
			return nil
		}
		for i := range t.Incident.Attempts {
			a := &t.Incident.Attempts[i]
			if a.ID != attemptID {
				continue
			}
			if sendErr != nil {
				a.State = "unknown"
				a.Error = sendErr.Error()
			} else {
				a.State = result.State
				a.Receipt = result.Receipt
			}
		}
		return nil
	})
}

// CancelOutstanding persists failed cancellations for bounded retry by a later watchdog tick.
func (e Engine) CancelOutstanding(ctx context.Context, id string) error {
	t, err := e.Get(id)
	if err != nil {
		return err
	}
	in := t.Incident
	if in == nil || (t.Armed && in.State == "open") {
		return nil
	}
	for _, a := range in.Attempts {
		if a.Receipt == "" || a.Cancelled || a.CancelAttempts >= 5 {
			continue
		}
		_, cancelErr := e.Notifier.Call(ctx, t.Policy.Channel, "cancel", protocol.Notification{TaskID: id, IncidentID: in.ID, AttemptID: a.ID, Receipt: a.Receipt})
		if err := e.Store.Update(func(st *state.State) error {
			t, err := task(st, id)
			if err != nil {
				return err
			}
			if t.Incident == nil || t.Incident.ID != in.ID {
				return nil
			}
			for i := range t.Incident.Attempts {
				current := &t.Incident.Attempts[i]
				if current.ID == a.ID {
					current.CancelAttempts++
					if cancelErr == nil {
						current.Cancelled = true
					} else {
						current.Error = "notification cancellation failed; watchdog will retry within its limit"
					}
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}
