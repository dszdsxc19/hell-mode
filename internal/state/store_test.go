package state

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func TestCorruptOrFutureStateIsNotOverwritten(t *testing.T) {
	for _, raw := range []string{`{"version":4}`, `null`, `[]`, `{}`, `{"version":3}`, `{"version":1,"tasks":null}`, `{"version":3,"engagements":null}`, `{"version":3,"engagements":{"bad":null}}`, `{"version":2,"memory":null}`, `{"version":2,"presence":{"mode":"unknown"}}`, `{"version":2,"presence":{"mode":"away"}}`, `not-json`} {
		t.Run(raw, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "state.db")
			db, err := bolt.Open(path, 0600, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Update(func(tx *bolt.Tx) error {
				b, err := tx.CreateBucket([]byte("state"))
				if err != nil {
					return err
				}
				return b.Put([]byte("v1"), []byte(raw))
			}); err != nil {
				t.Fatal(err)
			}
			_ = db.Close()
			store := Store{Dir: dir}
			if err := store.Update(func(st *State) error { st.Version = 1; return nil }); err == nil {
				t.Fatal("invalid state accepted")
			}
			db, err = bolt.Open(path, 0600, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err := db.View(func(tx *bolt.Tx) error {
				if !bytes.Equal(tx.Bucket([]byte("state")).Get([]byte("v1")), []byte(raw)) {
					t.Fatal("overwrote invalid state")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLegacyStateMigratesOnWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	legacy := []byte(`{"version":1,"tasks":{"task_old":{"id":"task_old","armed":true,"verdict":"unmet"}},"tokens":{},"events":{},"plugins":{}}`)
	if err := db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucket([]byte("state"))
		if err != nil {
			return err
		}
		return bucket.Put([]byte("v1"), legacy)
	}); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	store := Store{Dir: dir}
	st, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if st.Version != 3 || st.Presence.Mode != "available" || st.Tasks["task_old"].Quiet || st.Tasks["task_old"].Verdict != "unmet" || st.Engagements == nil || st.Memory.Text != "" {
		t.Fatalf("legacy state changed semantics: %+v", st)
	}
	if err := store.Update(func(st *State) error { return nil }); err != nil {
		t.Fatal(err)
	}
	db, err = bolt.Open(path, 0600, &bolt.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.View(func(tx *bolt.Tx) error {
		var persisted State
		if err := json.Unmarshal(tx.Bucket([]byte("state")).Get([]byte("v1")), &persisted); err != nil {
			return err
		}
		if persisted.Version != 3 || persisted.Presence.Mode != "available" || persisted.Engagements == nil {
			t.Fatalf("migration not persisted: %+v", persisted)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
