package model

import "time"

// Presence records the human's contact boundary, independently of task evidence.
// Until is informational: only an explicit human action ends an absence.
type Presence struct {
	Mode       string    `json:"mode"`
	Since      time.Time `json:"since"`
	Until      time.Time `json:"until,omitempty"`
	WatchEpoch uint64    `json:"watch_epoch"`
}
