// Package model defines the durable facts shared by the CLI, runner and watchdog.
package model

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// ID uses randomness so independently started CLI processes never share counters.
func ID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}

// Policy bounds application send attempts; provider-internal retries have their own limits.
type Policy struct {
	Channel     string        `json:"channel"`
	Interval    time.Duration `json:"interval_ns"`
	MaxAttempts int           `json:"max_attempts"`
	Window      time.Duration `json:"window_ns"`
}

// Task keeps goal evidence separate from whether a human has stopped the alarm.
type Task struct {
	ID                     string        `json:"id"`
	Goal                   string        `json:"goal"`
	Verify                 []string      `json:"verify"`
	Workdir                string        `json:"workdir"`
	Deadline               time.Time     `json:"deadline"`
	HeartbeatTimeout       time.Duration `json:"heartbeat_timeout_ns"`
	VerifyTimeout          time.Duration `json:"verify_timeout_ns"`
	Policy                 Policy        `json:"policy"`
	CreatedAt              time.Time     `json:"created_at"`
	Armed                  bool          `json:"armed"`
	Verdict                string        `json:"verdict"`
	CurrentRunID           string        `json:"current_run_id,omitempty"`
	Runs                   []Run         `json:"runs"`
	Checks                 []Check       `json:"checks"`
	Incident               *Incident     `json:"incident,omitempty"`
	IncidentHistory        []Incident    `json:"incident_history,omitempty"`
	PendingVerification    bool          `json:"pending_verification"`
	VerificationLeaseUntil time.Time     `json:"verification_lease_until"`
}

// Run identifies one attempt; events from an earlier run cannot change its successor.
type Run struct {
	ID            string    `json:"id"`
	State         string    `json:"state"`
	StartedAt     time.Time `json:"started_at"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	ExitCode      *int      `json:"exit_code,omitempty"`
}

// CurrentRun returns the current durable attempt, if execution has started.
func (t *Task) CurrentRun() *Run {
	for i := range t.Runs {
		if t.Runs[i].ID == t.CurrentRunID {
			return &t.Runs[i]
		}
	}
	return nil
}

// Check records bounded command evidence, including uncertainty from timeout or launch failure.
type Check struct {
	RunID    string    `json:"run_id"`
	At       time.Time `json:"at"`
	Verdict  string    `json:"verdict"`
	ExitCode int       `json:"exit_code"`
	Output   string    `json:"output,omitempty"`
	Error    string    `json:"error,omitempty"`
}

// Incident owns the bounded notification schedule for the current failure.
type Incident struct {
	ID        string    `json:"id"`
	State     string    `json:"state"`
	Reason    string    `json:"reason"`
	OpenedAt  time.Time `json:"opened_at"`
	ExpiresAt time.Time `json:"expires_at"`
	NextAt    time.Time `json:"next_at"`
	Attempts  []Attempt `json:"attempts"`
}

// Attempt is persisted before an external side effect to recover ambiguous sends honestly.
type Attempt struct {
	ID             string    `json:"id"`
	State          string    `json:"state"`
	At             time.Time `json:"at"`
	LeaseUntil     time.Time `json:"lease_until"`
	Receipt        string    `json:"receipt,omitempty"`
	Error          string    `json:"error,omitempty"`
	Cancelled      bool      `json:"cancelled"`
	CancelAttempts int       `json:"cancel_attempts"`
}

// Event is a restricted observation, never a request to disarm or alter a contract.
type Event struct {
	ID             string `json:"id"`
	TaskID         string `json:"task_id"`
	RunID          string `json:"run_id"`
	Type           string `json:"type"`
	Source         string `json:"source"`
	ExitCode       *int   `json:"exit_code,omitempty"`
	BackgroundWork bool   `json:"background_work,omitempty"`
}
