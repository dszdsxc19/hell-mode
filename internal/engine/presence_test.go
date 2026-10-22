package engine

import (
	"context"
	"testing"
	"time"

	"github.com/dszdsxc19/hell-mode/internal/model"
	"github.com/dszdsxc19/hell-mode/internal/state"
)

func TestAbsenceSurvivesTimerAndReturnDoesNotReplay(t *testing.T) {
	ctx := context.Background()
	e, old, now, n := fixture(t, "sh", "-c", "exit 1")
	if _, err := e.Verify(ctx, old.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	until := now.Add(25 * time.Minute)
	if _, err := e.SetPresence(ctx, "away", until); err != nil {
		t.Fatal(err)
	}
	got, _ := e.Get(old.ID)
	if !got.Quiet || got.Verdict != "unmet" || !got.Incident.Attempts[0].Cancelled || n.cancels != 1 {
		t.Fatalf("absence lost evidence or failed to cancel: %+v", got)
	}

	// A new task can still finish and be verified while nobody is available.
	during, err := e.Arm(model.Task{Goal: "created during absence", Verify: old.Verify, Workdir: old.Workdir, Deadline: old.Deadline, HeartbeatTimeout: old.HeartbeatTimeout, VerifyTimeout: old.VerifyTimeout, Policy: old.Policy})
	if err != nil {
		t.Fatal(err)
	}
	run, token, err := e.StartRun(during.ID, true)
	if err != nil || !run.Quiet {
		t.Fatalf("rearm bypassed absence: task=%+v err=%v", run, err)
	}
	if _, err := e.Event(model.Event{ID: "ended-during-absence", TaskID: during.ID, RunID: run.CurrentRunID, Type: "exited", Source: "test"}, token); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ = e.Get(during.ID)
	if got.Verdict != "unmet" || len(got.Checks) != 1 || len(got.Incident.Attempts) != 0 {
		t.Fatalf("quiet task was not verified silently: %+v", got)
	}

	*now = until.Add(time.Minute)
	// Recreating the engine models a restarted watchdog after the timer expires.
	e = Engine{Store: e.Store, Notifier: n, Now: func() time.Time { return *now }}
	if _, err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	presence, _ := e.Presence()
	if presence.Mode != "away" || !presence.Until.Equal(until) || n.sends != 1 {
		t.Fatalf("timer resumed contact: %+v sends=%d", presence, n.sends)
	}
	if _, err := e.SetPresence(ctx, "available", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ = e.Get(during.ID)
	if !got.Quiet || got.Verdict != "unmet" || n.sends != 1 {
		t.Fatalf("return replayed old work: %+v sends=%d", got, n.sends)
	}

	// Explicitly rearming one old task is the only action here that restores sends.
	run, _, err = e.StartRun(old.ID, true)
	if err != nil || run.Quiet {
		t.Fatalf("explicit rearm did not restore contact: task=%+v err=%v", run, err)
	}
	if _, err := e.Verify(ctx, old.ID, run.CurrentRunID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if n.sends != 2 {
		t.Fatalf("rearmed task did not notify: sends=%d", n.sends)
	}
}

func TestEndPreservesPendingWorkAndWatchEpoch(t *testing.T) {
	ctx := context.Background()
	e, task, _, n := fixture(t, "sh", "-c", "exit 1")
	run, token, err := e.StartRun(task.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Event(model.Event{ID: "pending-at-end", TaskID: task.ID, RunID: run.CurrentRunID, Type: "turn_completed", Source: "test"}, token); err != nil {
		t.Fatal(err)
	}
	ended, err := e.SetPresence(ctx, "ended", time.Time{})
	if err != nil || ended.WatchEpoch != 1 {
		t.Fatalf("end did not stop this watch generation: %+v %v", ended, err)
	}
	if _, err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := e.Get(task.ID)
	if got.Verdict != "pending" || !got.PendingVerification || len(got.Checks) != 0 || !got.Quiet || n.sends != 0 {
		t.Fatalf("ended watchdog consumed pending work: %+v", got)
	}
	returned, err := e.SetPresence(ctx, "available", time.Time{})
	if err != nil || returned.WatchEpoch != ended.WatchEpoch {
		t.Fatalf("return revived old watches: %+v %v", returned, err)
	}
	if _, err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ = e.Get(task.ID)
	if got.Verdict != "unmet" || len(got.Checks) != 1 || n.sends != 0 {
		t.Fatalf("return did not preserve quiet evidence: %+v", got)
	}
}

func TestHumanBoundaryCancelsLateReceipt(t *testing.T) {
	for _, mode := range []string{"away", "ended"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			e, task, now, n := fixture(t, "sh", "-c", "exit 1")
			if _, err := e.Verify(ctx, task.ID, ""); err != nil {
				t.Fatal(err)
			}
			n.entered, n.release = make(chan struct{}), make(chan struct{})
			done := make(chan error, 1)
			go func() { _, err := e.Tick(ctx); done <- err }()
			select {
			case <-n.entered:
			case <-time.After(3 * time.Second):
				t.Fatal("send did not start")
			}
			if _, err := e.SetPresence(ctx, mode, now.Add(25*time.Minute)); err != nil {
				t.Fatal(err)
			}
			close(n.release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			got, _ := e.Get(task.ID)
			if !got.Quiet || got.Verdict != "unmet" || n.sends != 1 || n.cancels != 1 || !got.Incident.Attempts[0].Cancelled {
				t.Fatalf("late receipt kept ringing: task=%+v sends=%d cancels=%d", got, n.sends, n.cancels)
			}
		})
	}
}

func TestOldWatchCannotResumeAfterEndAndBack(t *testing.T) {
	ctx := context.Background()
	e, template, now, n := fixture(t, "sh", "-c", "exit 1")
	oldWatch := e
	epoch := uint64(0)
	oldWatch.WatchEpoch = &epoch
	if _, err := e.SetPresence(ctx, "ended", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SetPresence(ctx, "available", time.Time{}); err != nil {
		t.Fatal(err)
	}
	// This new task is contactable; Quiet cannot accidentally mask a stale-watch bug.
	fresh, err := e.Arm(model.Task{Goal: "new task after returning", Verify: template.Verify, Workdir: template.Workdir, Deadline: template.Deadline, HeartbeatTimeout: template.HeartbeatTimeout, VerifyTimeout: template.VerifyTimeout, Policy: template.Policy})
	if err != nil {
		t.Fatal(err)
	}
	run, token, err := e.StartRun(fresh.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Event(model.Event{ID: "new-queued-check", TaskID: fresh.ID, RunID: run.CurrentRunID, Type: "turn_completed", Source: "test"}, token); err != nil {
		t.Fatal(err)
	}
	if _, err := oldWatch.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := e.Get(fresh.ID)
	if !got.PendingVerification || len(got.Checks) != 0 {
		t.Fatalf("old watch adopted the new epoch: %+v", got)
	}
	// The transaction guard also protects a queued check admitted before an end.
	if _, err := oldWatch.verify(ctx, fresh.ID, run.CurrentRunID, true); err == nil {
		t.Fatal("old watch claimed a queued verification")
	}
	if _, err := e.Verify(ctx, fresh.ID, run.CurrentRunID); err != nil {
		t.Fatal(err)
	}
	// A previously admitted send must recheck the same bound epoch before dispatch.
	if err := e.Store.Update(func(st *state.State) error {
		st.Tasks[fresh.ID].Incident.Attempts = append(st.Tasks[fresh.ID].Incident.Attempts, model.Attempt{ID: "admitted-send", State: "sending", At: *now, LeaseUntil: now.Add(time.Minute)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	got, _ = e.Get(fresh.ID)
	if err := oldWatch.send(ctx, fresh.ID, got.Incident.ID, "admitted-send"); err != nil {
		t.Fatal(err)
	}
	got, _ = e.Get(fresh.ID)
	if n.sends != 0 || got.Incident.Attempts[0].State != "cancelled" {
		t.Fatalf("old watch dispatched a new-epoch notification: %+v", got)
	}
}

func TestRealChannelRearmRequiresFreshWatchAfterEnd(t *testing.T) {
	ctx := context.Background()
	e, template, _, _ := fixture(t, "sh", "-c", "exit 1")
	policy := template.Policy
	policy.Channel = "pushover"
	real, err := e.Arm(model.Task{Goal: "real channel", Verify: template.Verify, Workdir: template.Workdir, Deadline: template.Deadline, HeartbeatTimeout: template.HeartbeatTimeout, VerifyTimeout: template.VerifyTimeout, Policy: policy})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.StartRun(real.ID, false); err == nil {
		t.Fatal("real notification run started without a watcher")
	}
	if _, err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SetPresence(ctx, "ended", time.Time{}); err != nil {
		t.Fatal(err)
	}
	st, err := e.Store.Snapshot()
	if err != nil || !st.WatchdogAt.IsZero() {
		t.Fatalf("ended watch retained its heartbeat: %+v %v", st, err)
	}
	if _, err := e.SetPresence(ctx, "available", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.StartRun(real.ID, true); err == nil {
		t.Fatal("return reused the ended watch's heartbeat")
	}
	got, _ := e.Get(real.ID)
	if !got.Quiet || len(got.Runs) != 0 {
		t.Fatalf("failed rearm changed the task: %+v", got)
	}
	// Quiet work remains possible without a watcher; resuming contact requires one.
	run, token, err := e.StartRun(real.ID, false)
	if err != nil || !run.Quiet {
		t.Fatalf("quiet run was blocked: %+v %v", run, err)
	}
	if _, err := e.Event(model.Event{ID: "quiet-run-ended", TaskID: real.ID, RunID: run.CurrentRunID, Type: "exited", Source: "test"}, token); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if resumed, _, err := e.StartRun(real.ID, true); err != nil || resumed.Quiet {
		t.Fatalf("fresh watch did not permit explicit rearm: %+v %v", resumed, err)
	}
}
