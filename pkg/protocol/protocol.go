// Package protocol is the versioned, language-neutral subprocess plugin contract.
package protocol

import "time"

const Version = "hell-mode/v1"

// Manifest registers an explicitly trusted executable; discovery never runs arbitrary PATH entries.
type Manifest struct {
	ProtocolVersion string   `json:"protocol_version"`
	Name            string   `json:"name"`
	Command         []string `json:"command"`
	TimeoutMS       int      `json:"timeout_ms"`
	Env             []string `json:"env,omitempty"`
}

// Notification contains only operational identifiers and a redacted failure category.
type Notification struct {
	TaskID     string    `json:"task_id"`
	IncidentID string    `json:"incident_id"`
	AttemptID  string    `json:"attempt_id"`
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	Receipt    string    `json:"receipt,omitempty"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// Request is exactly one JSON object written to a newly started plugin's stdin.
type Request struct {
	ProtocolVersion string        `json:"protocol_version"`
	ID              string        `json:"id"`
	Method          string        `json:"method"`
	Notification    *Notification `json:"notification,omitempty"`
}

// Response must echo both request identity and protocol version; stdout contains no logs.
type Response struct {
	ProtocolVersion string  `json:"protocol_version"`
	ID              string  `json:"id"`
	Result          *Result `json:"result,omitempty"`
	Error           *Error  `json:"error,omitempty"`
}

// Result describes capabilities or a provider receipt; queued never means a human is awake.
type Result struct {
	Name         string   `json:"name,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	State        string   `json:"state,omitempty"`
	Receipt      string   `json:"receipt,omitempty"`
}

// Error lets a plugin report a machine-readable failure without inventing a success receipt.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
