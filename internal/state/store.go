// Package state serializes cross-process updates in short bbolt transactions.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dszdsxc19/hell-mode/internal/model"
	"github.com/dszdsxc19/hell-mode/pkg/protocol"
	bolt "go.etcd.io/bbolt"
)

// State is versioned so an incompatible binary refuses to silently overwrite future data.
type State struct {
	Version    int                          `json:"version"`
	Tasks      map[string]*model.Task       `json:"tasks"`
	Tokens     map[string]string            `json:"tokens"`
	Events     map[string]bool              `json:"events"`
	Plugins    map[string]protocol.Manifest `json:"plugins"`
	WatchdogAt time.Time                    `json:"watchdog_at"`
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
	return &State{Version: 1, Tasks: map[string]*model.Task{}, Tokens: map[string]string{}, Events: map[string]bool{}, Plugins: map[string]protocol.Manifest{}}
}

// load rejects corrupt or future state before any caller can overwrite it.
func load(tx *bolt.Tx) (*State, error) {
	st := empty()
	bucket := tx.Bucket([]byte("state"))
	if bucket == nil {
		return st, nil
	}
	if raw := bucket.Get([]byte("v1")); raw != nil {
		if err := json.Unmarshal(raw, st); err != nil {
			return nil, fmt.Errorf("decode state: %w", err)
		}
	}
	if st.Version != 1 {
		return nil, fmt.Errorf("unsupported state version %d", st.Version)
	}
	if st.Tasks == nil || st.Tokens == nil || st.Events == nil || st.Plugins == nil {
		return nil, fmt.Errorf("state is missing required collections")
	}
	for id, t := range st.Tasks {
		if t == nil || t.ID != id {
			return nil, fmt.Errorf("state contains an invalid task record")
		}
	}
	return st, nil
}
