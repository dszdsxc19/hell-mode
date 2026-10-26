package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dszdsxc19/hell-mode/internal/model"
	"github.com/dszdsxc19/hell-mode/internal/state"
	"github.com/dszdsxc19/hell-mode/pkg/protocol"
)

type fakeNotifier struct {
	mu             sync.Mutex
	sends, cancels int
	fail           bool
	entered        chan struct{}
	release        chan struct{}
	notifications  []protocol.Notification
}

func (f *fakeNotifier) Call(_ context.Context, _, method string, n protocol.Notification) (*protocol.Result, error) {
	f.mu.Lock()
	if method == "cancel" {
		f.cancels++
		f.mu.Unlock()
		return &protocol.Result{State: "cancelled"}, nil
	}
	f.sends++
	f.notifications = append(f.notifications, n)
	f.mu.Unlock()
	if f.entered != nil {
		close(f.entered)
		<-f.release
	}
	if f.fail {
		return nil, errors.New("delivery uncertain")
	}
	return &protocol.Result{State: "queued", Receipt: "receipt:" + n.AttemptID}, nil
}

func fixture(t *testing.T, argv ...string) (Engine, *model.Task, *time.Time, *fakeNotifier) {
	t.Helper()
	now := time.Now().UTC()
	notifier := &fakeNotifier{}
	e := Engine{Store: state.Store{Dir: t.TempDir()}, Notifier: notifier, Now: func() time.Time { return now }}
	task, err := e.Arm(model.Task{Goal: "test goal", Verify: argv, Workdir: t.TempDir(), Deadline: now.Add(time.Hour), HeartbeatTimeout: time.Minute, VerifyTimeout: time.Second, Policy: model.Policy{Channel: "log", Interval: time.Minute, MaxAttempts: 2, Window: 10 * time.Minute}})
	if err != nil {
		t.Fatal(err)
	}
	return e, task, &now, notifier
}

func TestFailedVerificationAndBoundedRecovery(t *testing.T) {
	e, task, now, n := fixture(t, "sh", "-c", "exit 1")
	checked, err := e.Verify(context.Background(), task.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if checked.Verdict != "unmet" {
		t.Fatal(checked)
	}
	if _, err := e.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	// A new Engine instance uses the durable budget instead of restarting the sequence.
	e = Engine{Store: e.Store, Notifier: n, Now: func() time.Time { return *now }}
	if _, err := e.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n.sends != 1 {
		t.Fatal(n.sends)
	}
	*now = now.Add(time.Minute)
	if _, err := e.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Minute)
	if _, err := e.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, _ := e.Get(task.ID)
	if got.Verdict != "unmet" || got.Incident.State != "exhausted" || n.sends != 2 {
		t.Fatalf("task=%+v sends=%d", got, n.sends)
	}
}

func TestNotificationKeepsRedactedFactsAcrossUnknownDelivery(t *testing.T) {
	e, task, now, n := fixture(t, "sh", "-c", "printf 'private verification output'; exit 1")
	if err := e.Store.Update(func(st *state.State) error {
		st.Tasks[task.ID].Goal = "private goal"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Verify(context.Background(), task.ID, ""); err != nil {
		t.Fatal(err)
	}
	n.fail = true
	if _, err := e.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Restart after an unknown delivery: the next message uses the durable attempt
	// number without claiming that the first message was delivered or ignored.
	e = Engine{Store: e.Store, Notifier: n, Now: func() time.Time { return *now }}
	n.fail = false
	*now = now.Add(time.Minute)
	if _, err := e.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(n.notifications) != 2 {
		t.Fatal(n.notifications)
	}
	for i, notification := range n.notifications {
		for _, fact := range []string{task.ID, "verification_unmet", "hell status " + task.ID, fmt.Sprintf("第 %d 次通知尝试", i+1)} {
			if !strings.Contains(notification.Body, fact) {
				t.Fatalf("notification lost operational fact %q: %+v", fact, notification)
			}
		}
		for _, claim := range []string{"private", "忽略", "已读", "装没看见"} {
			if strings.Contains(notification.Body, claim) {
				t.Fatalf("notification leaked content or invented human behavior: %+v", notification)
			}
		}
	}
	if n.notifications[0].Body == n.notifications[1].Body {
		t.Fatal("repeat did not advance the recorded attempt")
	}
	got, err := e.Get(task.ID)
	if err != nil || got.Verdict != "unmet" || got.Incident.Attempts[0].State != "unknown" || got.Incident.Attempts[1].State != "queued" {
		t.Fatal(got, err)
	}
}

func TestTakeoverAndSnoozeDoNotPassGoal(t *testing.T) {
	e, task, now, n := fixture(t, "sh", "-c", "exit 1")
	_, _ = e.Verify(context.Background(), task.ID, "")
	_, _ = e.Tick(context.Background())
	_, err := e.Act(context.Background(), task.ID, "snooze", now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Minute)
	_, _ = e.Tick(context.Background())
	if n.sends != 1 || n.cancels != 1 {
		t.Fatalf("sends=%d cancels=%d", n.sends, n.cancels)
	}
	*now = now.Add(time.Minute)
	_, _ = e.Tick(context.Background())
	if n.sends != 2 {
		t.Fatal(n.sends)
	}
	_, err = e.Act(context.Background(), task.ID, "takeover", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Minute)
	_, _ = e.Tick(context.Background())
	got, _ := e.Get(task.ID)
	if got.Verdict != "unmet" || got.Armed || got.Incident.State != "taken_over" || n.sends != 2 {
		t.Fatal(got)
	}
}

func TestEventAuthorityDedupAndOldRunFence(t *testing.T) {
	e, task, _, _ := fixture(t, "sh", "-c", "exit 0")
	running, token, err := e.StartRun(task.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	event := model.Event{ID: "end-1", TaskID: task.ID, RunID: running.CurrentRunID, Type: "exited", Source: "test"}
	if _, err := e.Event(event, "wrong"); err == nil {
		t.Fatal("accepted wrong credential")
	}
	check, err := e.Event(event, token)
	if err != nil || !check {
		t.Fatalf("check=%v err=%v", check, err)
	}
	check, err = e.Event(event, token)
	if err != nil || check {
		t.Fatal("duplicate scheduled verification")
	}
	second, _, err := e.StartRun(task.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	event.ID = "late-wait"
	event.Type = "waiting"
	if _, err := e.Event(event, token); err != nil {
		t.Fatal(err)
	}
	got, _ := e.Get(task.ID)
	if got.CurrentRunID != second.CurrentRunID || got.Incident != nil || got.Verdict != "pending" {
		t.Fatal(got)
	}
}

func TestLostRunnerAndLateEventsAfterSuccess(t *testing.T) {
	e, task, now, n := fixture(t, "sh", "-c", "exit 0")
	running, token, err := e.StartRun(task.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	*now = now.Add(2 * time.Minute)
	_, err = e.Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got, _ := e.Get(task.ID)
	if got.CurrentRun().State != "lost" || got.Verdict != "unknown" || n.sends != 1 {
		t.Fatal(got)
	}
	if _, err := e.Verify(context.Background(), task.ID, running.CurrentRunID); err != nil {
		t.Fatal(err)
	}
	_, err = e.Event(model.Event{ID: "late", TaskID: task.ID, RunID: running.CurrentRunID, Type: "waiting", Source: "test"}, token)
	if err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Hour)
	_, _ = e.Tick(context.Background())
	got, _ = e.Get(task.ID)
	if got.Verdict != "met" || got.Incident.State != "resolved" || n.sends != 1 {
		t.Fatal(got)
	}
}

func TestConcurrentWatchdogsClaimOnce(t *testing.T) {
	e, task, _, n := fixture(t, "sh", "-c", "exit 1")
	_, _ = e.Verify(context.Background(), task.ID, "")
	var group sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() { defer group.Done(); _, err := e.Tick(context.Background()); errs <- err }()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if n.sends != 1 {
		t.Fatalf("duplicate sends: %d", n.sends)
	}
}

func TestTakeoverDuringSendCancelsLateReceipt(t *testing.T) {
	e, task, _, n := fixture(t, "sh", "-c", "exit 1")
	_, _ = e.Verify(context.Background(), task.ID, "")
	n.entered = make(chan struct{})
	n.release = make(chan struct{})
	done := make(chan error, 1)
	go func() { _, err := e.Tick(context.Background()); done <- err }()
	select {
	case <-n.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("send not started")
	}
	if _, err := e.Act(context.Background(), task.ID, "takeover", time.Time{}); err != nil {
		t.Fatal(err)
	}
	close(n.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	got, _ := e.Get(task.ID)
	if n.cancels != 1 || !got.Incident.Attempts[0].Cancelled || got.Verdict != "unmet" {
		t.Fatal(got)
	}
}

func TestUnknownSendUsesBudgetAndTimeoutIsNotUnmet(t *testing.T) {
	e, task, _, n := fixture(t, "sh", "-c", "sleep 5")
	n.fail = true
	if err := e.Store.Update(func(st *state.State) error { st.Tasks[task.ID].VerifyTimeout = 20 * time.Millisecond; return nil }); err != nil {
		t.Fatal(err)
	}
	got, err := e.Verify(context.Background(), task.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Verdict != "unknown" {
		t.Fatal(got)
	}
	_, err = e.Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got, _ = e.Get(task.ID)
	if got.Incident.Attempts[0].State != "unknown" || len(got.Incident.Attempts) != 1 {
		t.Fatal(got)
	}
	if len(n.notifications) != 1 || !strings.Contains(n.notifications[0].Body, "verification_unknown") || strings.Contains(n.notifications[0].Body, "验收没过") {
		t.Fatal("unknown verification was presented as a failed check", n.notifications)
	}
}

func TestVerificationCannotOverwriteNewRun(t *testing.T) {
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready")
	release := filepath.Join(dir, "release")
	e, task, _, _ := fixture(t, "sh", "-c", `touch "$1"; while [ ! -f "$2" ]; do sleep 0.01; done`, "verify", ready, release)
	running, token, err := e.StartRun(task.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := e.Verify(context.Background(), task.ID, running.CurrentRunID); done <- err }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("verification not started")
		}
		time.Sleep(5 * time.Millisecond)
	}
	_, err = e.Event(model.Event{ID: "exit", TaskID: task.ID, RunID: running.CurrentRunID, Type: "exited", Source: "test"}, token)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := e.StartRun(task.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(release, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err == nil {
		t.Fatal("stale verification accepted")
	}
	got, _ := e.Get(task.ID)
	if got.CurrentRunID != second.CurrentRunID || got.Verdict != "pending" {
		t.Fatal(got)
	}
}

func TestQueuedVerificationIsClaimedOnceAndSurvivesCallbackExit(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "checks")
	e, task, _, n := fixture(t, "sh", "-c", `echo checked >> "$1"; exit 1`, "verify", marker)
	running, token, err := e.StartRun(task.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	check, err := e.Event(model.Event{ID: "stop", TaskID: task.ID, RunID: running.CurrentRunID, Type: "turn_completed", Source: "host"}, token)
	if err != nil || !check {
		t.Fatalf("%v %v", check, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("event callback executed verification")
	}
	var group sync.WaitGroup
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		group.Add(1)
		go func() { defer group.Done(); _, err := e.Tick(context.Background()); errs <- err }()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, _ := e.Get(task.ID)
	content, err := os.ReadFile(marker)
	if err != nil || string(content) != "checked\n" || len(got.Checks) != 1 || n.sends != 1 {
		t.Fatalf("checks=%d output=%q sends=%d err=%v", len(got.Checks), content, n.sends, err)
	}
}

func TestExitAfterTakeoverStillClosesRunAndRearmKeepsHistory(t *testing.T) {
	e, task, _, _ := fixture(t, "sh", "-c", "exit 1")
	running, token, err := e.StartRun(task.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.Event(model.Event{ID: "wait", TaskID: task.ID, RunID: running.CurrentRunID, Type: "waiting", Source: "host"}, token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Act(context.Background(), task.ID, "takeover", time.Time{}); err != nil {
		t.Fatal(err)
	}
	_, err = e.Event(model.Event{ID: "end", TaskID: task.ID, RunID: running.CurrentRunID, Type: "exited", Source: "runner"}, token)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := e.Get(task.ID)
	if got.CurrentRun().State != "exited" || got.Armed || got.PendingVerification {
		t.Fatal(got)
	}
	newRun, _, err := e.StartRun(task.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(newRun.IncidentHistory) != 1 || newRun.IncidentHistory[0].State != "taken_over" || newRun.CurrentRunID == running.CurrentRunID {
		t.Fatal(newRun)
	}
}

func TestLostVerificationLeaseRecordsUnknown(t *testing.T) {
	e, task, now, _ := fixture(t, "sh", "-c", "exit 0")
	if err := e.Store.Update(func(st *state.State) error {
		st.Tasks[task.ID].VerificationLeaseUntil = now.Add(-time.Second)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, _ := e.Get(task.ID)
	if got.Verdict != "unknown" || got.Incident.Reason != "verification_lost" {
		t.Fatal(got)
	}
}
