package state

import (
	"bytes"
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func TestCorruptOrFutureStateIsNotOverwritten(t *testing.T) {
	for _, raw := range []string{`{"version":2}`, `{"version":1,"tasks":null}`, `not-json`} {
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
