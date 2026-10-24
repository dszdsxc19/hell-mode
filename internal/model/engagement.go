package model

import "time"

// PersonalMemory is the human-owned context shared with an explicitly configured agent.
type PersonalMemory struct {
	Text      string    `json:"text"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Engagement schedules a conversation independently of task alarms and evidence.
type Engagement struct {
	ID        string            `json:"id"`
	Focus     string            `json:"focus"`
	Every     time.Duration     `json:"every_ns"`
	NextAt    time.Time         `json:"next_at"`
	Agent     []string          `json:"agent"`
	Workdir   string            `json:"workdir"`
	Timeout   time.Duration     `json:"timeout_ns"`
	Channel   string            `json:"channel"`
	Active    bool              `json:"active"`
	CreatedAt time.Time         `json:"created_at"`
	Rounds    []EngagementRound `json:"rounds"`
	LastError string            `json:"last_error,omitempty"`
}

// EngagementRound preserves the generated prompt, its grounding and the human's answer.
// A waiting round blocks new prompts until the human explicitly answers or skips it.
type EngagementRound struct {
	ID         string    `json:"id"`
	State      string    `json:"state"`
	StartedAt  time.Time `json:"started_at"`
	LeaseUntil time.Time `json:"lease_until,omitempty"`
	Kind       string    `json:"kind,omitempty"`
	Message    string    `json:"message,omitempty"`
	Basis      string    `json:"basis,omitempty"`
	Error      string    `json:"error,omitempty"`
	Answer     string    `json:"answer,omitempty"`
	AnsweredAt time.Time `json:"answered_at,omitempty"`
	Delivery   *Attempt  `json:"delivery,omitempty"`
}
