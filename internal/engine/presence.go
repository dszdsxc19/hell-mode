package engine

import (
	"context"
	"time"

	"github.com/dszdsxc19/hell-mode/internal/fault"
	"github.com/dszdsxc19/hell-mode/internal/model"
	"github.com/dszdsxc19/hell-mode/internal/state"
)

// Presence is read-only; reaching Until never makes the human available.
func (e Engine) Presence() (model.Presence, error) {
	st, err := e.Store.Snapshot()
	if err != nil {
		return model.Presence{}, err
	}
	return st.Presence, nil
}

func (e Engine) watchStopped(presence model.Presence) bool {
	return presence.Mode == "ended" || (e.WatchEpoch != nil && *e.WatchEpoch != presence.WatchEpoch)
}

// SetPresence changes only the human boundary and notification permission.
// Returning leaves old tasks quiet; an explicit new run can rearm them.
func (e Engine) SetPresence(ctx context.Context, mode string, until time.Time) (model.Presence, error) {
	if mode != "available" && mode != "away" && mode != "ended" {
		return model.Presence{}, fault.New("invalid_argument", 2, "unsupported presence mode")
	}
	var result model.Presence
	err := e.Store.Update(func(st *state.State) error {
		now := e.now()
		if mode == "away" && !until.After(now) {
			return fault.New("invalid_argument", 2, "absence time must be in the future")
		}
		result = model.Presence{Mode: mode, Since: now, WatchEpoch: st.Presence.WatchEpoch}
		if mode == "away" {
			result.Until = until.UTC()
		}
		if mode == "ended" {
			result.WatchEpoch++
			st.WatchdogAt = time.Time{}
		}
		st.Presence = result
		if mode != "available" {
			for _, t := range st.Tasks {
				t.Quiet = true
			}
		}
		return nil
	})
	if err == nil && mode != "available" {
		err = e.cancelAllOutstanding(ctx)
	}
	return result, err
}

func (e Engine) cancelAllOutstanding(ctx context.Context) error {
	tasks, err := e.List()
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if err := e.CancelOutstanding(ctx, t.ID); err != nil {
			return err
		}
	}
	return nil
}
