// Package state serializes cross-process updates in short bbolt transactions.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dszdsxc19/hell-mode/internal/model"
	"github.com/dszdsxc19/hell-mode/pkg/protocol"
	bolt "go.etcd.io/bbolt"
)

// State is versioned so an incompatible binary refuses to silently overwrite future data.
type State struct {
	Version     int                          `json:"version"`
	Tasks       map[string]*model.Task       `json:"tasks"`
	Tokens      map[string]string            `json:"tokens"`
	Events      map[string]bool              `json:"events"`
	Plugins     map[string]protocol.Manifest `json:"plugins"`
	WatchdogAt  time.Time                    `json:"watchdog_at"`
	Presence    model.Presence               `json:"presence"`
	Memory      model.PersonalMemory         `json:"memory"`
	Engagements map[string]*model.Engagement `json:"engagements"`
}

type Store struct{ Dir string }

// Update keeps database locks out of command execution and network requests.
func (s Store) Update(fn func(*State) error) error {
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return err
	}
	db, err := bolt.Open(filepath.Join(s.Dir, "state.db"), 0600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return fmt.Errorf("open state: %w", err)
	}
	defer db.Close()
	return db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("state"))
		if err != nil {
			return err
		}
		st, err := load(tx)
		if err != nil {
			return err
		}
		if err := fn(st); err != nil {
			return err
		}
		raw, err := json.Marshal(st)
		if err != nil {
			return err
		}
		return bucket.Put([]byte("v1"), raw)
	})
}

// Snapshot returns a detached copy; callers cannot mutate durable state outside a transaction.
func (s Store) Snapshot() (*State, error) {
	path := filepath.Join(s.Dir, "state.db")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return empty(), nil
	} else if err != nil {
		return nil, err
	}
	db, err := bolt.Open(path, 0600, &bolt.Options{ReadOnly: true, Timeout: 3 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	defer db.Close()
	var result *State
	err = db.View(func(tx *bolt.Tx) error { var err error; result, err = load(tx); return err })
	return result, err
}

func empty() *State {
	return &State{Version: 3, Tasks: map[string]*model.Task{}, Tokens: map[string]string{}, Events: map[string]bool{}, Plugins: map[string]protocol.Manifest{}, Presence: model.Presence{Mode: "available"}, Engagements: map[string]*model.Engagement{}}
}

// load rejects corrupt or future state before any caller can overwrite it.
func load(tx *bolt.Tx) (*State, error) {
	st := empty()
	bucket := tx.Bucket([]byte("state"))
	if bucket == nil {
		return st, nil
	}
	if raw := bucket.Get([]byte("v1")); raw != nil {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil || fields == nil || fields["version"] == nil {
			return nil, fmt.Errorf("decode state: expected a versioned object")
		}
		st.Version = 0
		if err := json.Unmarshal(raw, st); err != nil {
			return nil, fmt.Errorf("decode state: %w", err)
		}
		if st.Version == 3 && fields["engagements"] == nil {
			return nil, fmt.Errorf("state is missing engagements")
		}
		if strings.TrimSpace(string(fields["memory"])) == "null" || strings.TrimSpace(string(fields["presence"])) == "null" {
			return nil, fmt.Errorf("state contains a null record")
		}
	}
	if st.Version != 1 && st.Version != 2 && st.Version != 3 {
		return nil, fmt.Errorf("unsupported state version %d", st.Version)
	}
	// Older binaries refuse v3 instead of silently dropping memory or conversations.
	st.Version = 3
	if st.Presence.Mode == "" {
		st.Presence.Mode = "available"
	}
	switch st.Presence.Mode {
	case "available", "ended":
	case "away":
		if st.Presence.Until.IsZero() {
			return nil, fmt.Errorf("away presence is missing its agreed time")
		}
	default:
		return nil, fmt.Errorf("state contains an invalid presence mode")
	}
	if st.Tasks == nil || st.Tokens == nil || st.Events == nil || st.Plugins == nil || st.Engagements == nil {
		return nil, fmt.Errorf("state is missing required collections")
	}
	for id, t := range st.Tasks {
		if t == nil || t.ID != id {
			return nil, fmt.Errorf("state contains an invalid task record")
		}
	}
	for id, engagement := range st.Engagements {
		if engagement == nil || engagement.ID != id || id == "" || engagement.Every <= 0 || engagement.Timeout <= 0 || engagement.Timeout > 5*time.Minute || len(engagement.Agent) == 0 || engagement.Agent[0] == "" || engagement.Workdir == "" || engagement.Channel == "" || engagement.CreatedAt.IsZero() || engagement.NextAt.IsZero() || engagement.Rounds == nil {
			return nil, fmt.Errorf("state contains an invalid engagement record")
		}
		unresolved := 0
		seen := map[string]bool{}
		for _, round := range engagement.Rounds {
			if round.ID == "" || seen[round.ID] || round.StartedAt.IsZero() {
				return nil, fmt.Errorf("state contains an invalid engagement round")
			}
			seen[round.ID] = true
			switch round.State {
			case "generating":
				if round.LeaseUntil.IsZero() {
					return nil, fmt.Errorf("state contains an invalid generation lease")
				}
				unresolved++
			case "waiting":
				unresolved++
			case "answered", "skipped", "failed", "unknown":
			default:
				return nil, fmt.Errorf("state contains an invalid engagement round state")
			}
		}
		if unresolved > 1 {
			return nil, fmt.Errorf("state contains multiple unresolved engagement rounds")
		}
	}
	return st, nil
}
