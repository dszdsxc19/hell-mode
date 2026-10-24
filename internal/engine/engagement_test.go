package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// The real child process exercises stdin/stdout, timeout and environment boundaries.
func TestEngagementAgentHelper(t *testing.T) {
	start := -1
	for i, arg := range os.Args {
		if arg == "--engagement-helper" {
			start = i + 1
			break
		}
	}
	if start == -1 {
		return
	}
	mode, capture := os.Args[start], os.Args[start+1]
	raw, err := io.ReadAll(os.Stdin)
	if err != nil || os.WriteFile(capture, raw, 0600) != nil {
		os.Exit(2)
	}
	if os.Getenv("HELL_TASK_ID") != "" || os.Getenv("HELL_RUN_ID") != "" || os.Getenv("HELL_EVENT_TOKEN") != "" {
		os.Exit(3)
	}
	if mode == "blocked" {
		_ = os.WriteFile(capture+".entered", nil, 0600)
		for {
			if _, err := os.Stat(capture + ".release"); err == nil {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	switch mode {
	case "failed":
		fmt.Fprintln(os.Stderr, "private credential: do not persist this")
		os.Exit(1)
	case "malformed":
		fmt.Print("```json\n{}\n```")
	case "oversized":
		fmt.Print(strings.Repeat("x", 40<<10))
	case "empty":
	case "invalid-utf8":
		_, _ = os.Stdout.Write([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'})
	case "skip":
		fmt.Print(`{"kind":"skip","message":"","basis":"No current concern warrants a question."}`)
	case "timeout":
		time.Sleep(30 * time.Second)
	default:
		fmt.Print(`{"kind":"question","message":"What is the next small step on your project?","basis":"Your stated project focus and previous answer."}`)
	}
	os.Exit(0)
}

type engagementNotifier struct {
	mu            sync.Mutex
	notifications []protocol.Notification
	fail          bool
}

func (n *engagementNotifier) Call(_ context.Context, _, method string, notification protocol.Notification) (*protocol.Result, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if method != "notify" {
		return nil, fmt.Errorf("unexpected cancellation for a normal push")
	}
	n.notifications = append(n.notifications, notification)
	if n.fail {
		return nil, fmt.Errorf("delivery uncertain")
	}
	return &protocol.Result{State: "sent"}, nil
}

func engagementFixture(t *testing.T, mode string) (Engine, *model.Engagement, *time.Time, *engagementNotifier, string) {
	t.Helper()
	now := time.Date(2026, 10, 25, 3, 0, 0, 0, time.UTC)
	n := &engagementNotifier{}
	e := Engine{Store: state.Store{Dir: t.TempDir()}, Notifier: n, Now: func() time.Time { return now }}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	capture := filepath.Join(t.TempDir(), "request.json")
	g, err := e.CreateEngagement(model.Engagement{Every: time.Hour, NextAt: now.Add(-time.Hour), Agent: []string{exe, "-test.run=^TestEngagementAgentHelper$", "--", "--engagement-helper", mode, capture}, Workdir: t.TempDir(), Timeout: 2 * time.Second, Channel: "log"})
	if err != nil {
		t.Fatal(err)
	}
	return e, g, &now, n, capture
}

func readEngagementRequest(t *testing.T, path string) struct {
	ProtocolVersion string                  `json:"protocol_version"`
	Memory          model.PersonalMemory    `json:"memory"`
	History         []model.EngagementRound `json:"history"`
} {
	t.Helper()
	var result struct {
		ProtocolVersion string                  `json:"protocol_version"`
		Memory          model.PersonalMemory    `json:"memory"`
		History         []model.EngagementRound `json:"history"`
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func waitEngagementChild(t *testing.T, capture string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(capture + ".entered"); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("agent did not start")
}

func TestEngagementMemoryReplyAndNoCatchup(t *testing.T) {
	e, g, now, n, capture := engagementFixture(t, "question")
	t.Setenv("HELL_TASK_ID", "old-task")
	t.Setenv("HELL_RUN_ID", "old-run")
	t.Setenv("HELL_EVENT_TOKEN", "private-token")
	_, _ = e.SetMemory("old context")
	_, _ = e.SetMemory("private current concern")
	count, err := e.Tick(context.Background())
	if err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	request := readEngagementRequest(t, capture)
	if request.ProtocolVersion != "hell-mode/engage-v1" || request.Memory.Text != "private current concern" || len(request.History) != 0 {
		t.Fatalf("request=%+v", request)
	}
	got, _ := e.GetEngagement(g.ID)
	if len(got.Rounds) != 1 || got.Rounds[0].State != "waiting" || got.Rounds[0].Delivery.State != "sent" {
		t.Fatalf("engagement=%+v", got)
	}
	if n.notifications[0].Priority != "normal" || strings.Contains(n.notifications[0].Body, "private") || strings.Contains(n.notifications[0].Body, got.Rounds[0].Message) {
		t.Fatalf("notification leaked content or priority: %+v", n.notifications[0])
	}
	*now = now.Add(24 * time.Hour)
	if count, err := e.Tick(context.Background()); err != nil || count != 0 {
		t.Fatalf("waiting prompt replayed: count=%d err=%v", count, err)
	}
	roundID := got.Rounds[0].ID
	answered, err := e.ReplyEngagement(g.ID, roundID, "I will finish one CLI command.", false)
	if err != nil || !answered.NextAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("answered=%+v err=%v", answered, err)
	}
	if _, err := e.ReplyEngagement(g.ID, roundID, "stale", false); err == nil {
		t.Fatal("accepted a stale answer")
	}
	_, _ = e.SetMemory("new concern after answer")
	*now = now.Add(time.Hour)
	if _, err := e.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	request = readEngagementRequest(t, capture)
	if request.Memory.Text != "new concern after answer" || len(request.History) != 1 || request.History[0].Answer != "I will finish one CLI command." || request.History[0].Delivery != nil {
		t.Fatalf("latest answer was not supplied: %+v", request)
	}
	got, _ = e.GetEngagement(g.ID)
	if _, err := e.ReplyEngagement(g.ID, got.Rounds[1].ID, "", true); err != nil {
		t.Fatal(err)
	}
}

func TestEngagementConcurrentTicksClaimOneGenerator(t *testing.T) {
	e, g, _, n, capture := engagementFixture(t, "blocked")
	finished := make(chan error, 1)
	go func() { _, err := e.Tick(context.Background()); finished <- err }()
	waitEngagementChild(t, capture)
	if count, err := e.Tick(context.Background()); err != nil || count != 0 {
		t.Fatalf("second tick count=%d err=%v", count, err)
	}
	if err := os.WriteFile(capture+".release", nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	got, _ := e.GetEngagement(g.ID)
	if len(got.Rounds) != 1 || len(n.notifications) != 1 {
		t.Fatalf("rounds=%d sends=%d", len(got.Rounds), len(n.notifications))
	}
}

func TestEngagementLaterScheduleClaimsOnlyWhenItsGeneratorCanStart(t *testing.T) {
	e, first, _, n, firstCapture := engagementFixture(t, "blocked")
	secondConfig := *first
	secondConfig.Agent = append([]string(nil), first.Agent...)
	secondCapture := filepath.Join(t.TempDir(), "second-request.json")
	secondConfig.Agent[len(secondConfig.Agent)-2] = "question"
	secondConfig.Agent[len(secondConfig.Agent)-1] = secondCapture
	second, err := e.CreateEngagement(secondConfig)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID > second.ID {
		first, second = second, first
		_ = e.Store.Update(func(st *state.State) error {
			st.Engagements[first.ID].Agent[len(first.Agent)-2] = "blocked"
			st.Engagements[first.ID].Agent[len(first.Agent)-1] = firstCapture
			st.Engagements[second.ID].Agent[len(second.Agent)-2] = "question"
			st.Engagements[second.ID].Agent[len(second.Agent)-1] = secondCapture
			return nil
		})
	}
	_, _ = e.SetMemory("before first generation")
	finished := make(chan error, 1)
	go func() { _, err := e.Tick(context.Background()); finished <- err }()
	waitEngagementChild(t, firstCapture)
	notStarted, _ := e.GetEngagement(second.ID)
	if len(notStarted.Rounds) != 0 {
		t.Fatal("later generator already has a lease while the first is running")
	}
	_, _ = e.SetMemory("updated while first generation was running")
	_ = os.WriteFile(firstCapture+".release", nil, 0600)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	request := readEngagementRequest(t, secondCapture)
	if request.Memory.Text != "updated while first generation was running" || len(n.notifications) != 2 {
		t.Fatalf("later generator used stale context: %+v", request)
	}
}

func TestEngagementPauseResumeFencesInFlightGeneration(t *testing.T) {
	for _, boundary := range []string{"pause", "away", "ended"} {
		t.Run(boundary, func(t *testing.T) {
			e, g, now, n, capture := engagementFixture(t, "blocked")
			finished := make(chan error, 1)
			go func() { _, err := e.Tick(context.Background()); finished <- err }()
			waitEngagementChild(t, capture)
			if boundary == "pause" {
				if _, err := e.SetEngagementActive(g.ID, false); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := e.SetPresence(context.Background(), boundary, now.Add(time.Hour)); err != nil {
					t.Fatal(err)
				}
				if _, err := e.SetEngagementActive(g.ID, true); err == nil {
					t.Fatal("resumed during absence")
				}
				if _, err := e.SetPresence(context.Background(), "available", time.Time{}); err != nil {
					t.Fatal(err)
				}
				paused, _ := e.GetEngagement(g.ID)
				if paused.Active {
					t.Fatal("return automatically resumed conversations")
				}
			}
			resumed, err := e.SetEngagementActive(g.ID, true)
			if err != nil || !resumed.NextAt.Equal(now.Add(time.Hour)) {
				t.Fatalf("resume=%+v err=%v", resumed, err)
			}
			_ = os.WriteFile(capture+".release", nil, 0600)
			if err := <-finished; err != nil {
				t.Fatal(err)
			}
			got, _ := e.GetEngagement(g.ID)
			if got.Rounds[0].State != "skipped" || len(n.notifications) != 0 {
				t.Fatalf("late generation published: %+v", got.Rounds)
			}
		})
	}
}

func TestEngagementPauseResumeInvalidatesUnsentDelivery(t *testing.T) {
	e, g, now, n, _ := engagementFixture(t, "question")
	round := model.EngagementRound{ID: "ready-round", State: "waiting", Kind: "question", Message: "Prompt", Basis: "Memory", StartedAt: *now, Delivery: &model.Attempt{ID: "unsent-attempt", State: "pending", At: *now, LeaseUntil: now.Add(time.Minute)}}
	if err := e.Store.Update(func(st *state.State) error { st.Engagements[g.ID].Rounds = []model.EngagementRound{round}; return nil }); err != nil {
		t.Fatal(err)
	}
	_, _ = e.SetEngagementActive(g.ID, false)
	_, _ = e.SetEngagementActive(g.ID, true)
	sent, err := e.sendEngagement(context.Background(), engagementClaim{EngagementID: g.ID, RoundID: round.ID, Epoch: 0, Config: *g}, round.Delivery.ID)
	if err != nil || sent || len(n.notifications) != 0 {
		t.Fatalf("stale claim sent after resume: sent=%v err=%v", sent, err)
	}
	got, _ := e.GetEngagement(g.ID)
	if got.Rounds[0].State != "waiting" || got.Rounds[0].Delivery.State != "cancelled" {
		t.Fatalf("prompt should remain answerable after cancelling unsent push: %+v", got)
	}
}

func TestEngagementPausePreservesInFlightDeliveryAndLateReceipt(t *testing.T) {
	e, g, _, _, _ := engagementFixture(t, "question")
	n := &fakeNotifier{entered: make(chan struct{}), release: make(chan struct{})}
	e.Notifier = n
	finished := make(chan error, 1)
	go func() { _, err := e.Tick(context.Background()); finished <- err }()
	select {
	case <-n.entered:
	case <-time.After(4 * time.Second):
		t.Fatal("notification did not start")
	}
	if _, err := e.SetEngagementActive(g.ID, false); err != nil {
		t.Fatal(err)
	}
	paused, _ := e.GetEngagement(g.ID)
	if paused.Active || paused.Rounds[0].Delivery.State != "sending" {
		t.Fatal("pause lost the in-flight request lease")
	}
	close(n.release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	got, _ := e.GetEngagement(g.ID)
	if got.Rounds[0].Delivery.Receipt == "" || got.Rounds[0].Delivery.State != "queued" || got.Rounds[0].State != "waiting" || n.cancels != 0 {
		t.Fatalf("late normal notification result was not preserved: %+v", got)
	}
}

func TestEngagementFailuresSkipAndUncertainSend(t *testing.T) {
	for _, mode := range []string{"failed", "malformed", "oversized", "empty", "invalid-utf8", "timeout", "skip"} {
		t.Run(mode, func(t *testing.T) {
			e, g, now, n, _ := engagementFixture(t, mode)
			if mode == "timeout" {
				_ = e.Store.Update(func(st *state.State) error { st.Engagements[g.ID].Timeout = 30 * time.Millisecond; return nil })
			}
			if _, err := e.Tick(context.Background()); err != nil {
				t.Fatal(err)
			}
			got, _ := e.GetEngagement(g.ID)
			wantState := "failed"
			if mode == "skip" {
				wantState = "skipped"
			}
			if got.Rounds[0].State != wantState || len(n.notifications) != 0 || !got.NextAt.Equal(now.Add(time.Hour)) || strings.Contains(got.LastError, "credential") {
				t.Fatalf("got=%+v notifications=%d", got, len(n.notifications))
			}
			if wantState == "failed" && got.Rounds[0].Error != got.LastError {
				t.Fatal("generation failure was not preserved on its round")
			}
			if mode == "failed" {
				if !strings.Contains(got.LastError, "exit 1") {
					t.Fatal("failure lost its safe exit code")
				}
				previousError := got.Rounds[0].Error
				_ = e.Store.Update(func(st *state.State) error {
					st.Engagements[g.ID].Agent[len(g.Agent)-2] = "question"
					return nil
				})
				*now = now.Add(time.Hour)
				_, _ = e.Tick(context.Background())
				got, _ = e.GetEngagement(g.ID)
				if got.LastError != "" || got.Rounds[0].Error != previousError || got.Rounds[1].State != "waiting" {
					t.Fatal("success erased historical failure evidence")
				}
			}
		})
	}
	e, g, now, n, _ := engagementFixture(t, "question")
	n.fail = true
	_, _ = e.Tick(context.Background())
	got, _ := e.GetEngagement(g.ID)
	if got.Rounds[0].State != "waiting" || got.Rounds[0].Delivery.State != "unknown" {
		t.Fatalf("uncertain delivery lost answerable prompt: %+v", got)
	}
	*now = now.Add(24 * time.Hour)
	_, _ = e.Tick(context.Background())
	if len(n.notifications) != 1 {
		t.Fatal("uncertain normal notification automatically retried")
	}
}

func TestEngagementRestartRecoversLostClaimsWithoutResending(t *testing.T) {
	e, g, now, n, _ := engagementFixture(t, "question")
	if err := e.Store.Update(func(st *state.State) error {
		st.Engagements[g.ID].Rounds = []model.EngagementRound{{ID: "lost", State: "generating", StartedAt: now.Add(-time.Minute), LeaseUntil: now.Add(-time.Second)}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_, _ = e.Tick(context.Background())
	got, _ := e.GetEngagement(g.ID)
	if got.Rounds[0].State != "unknown" || len(n.notifications) != 0 || !got.NextAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("lost generation not recovered: %+v", got)
	}
	*now = now.Add(time.Hour)
	_, _ = e.Tick(context.Background())
	got, _ = e.GetEngagement(g.ID)
	if len(got.Rounds) != 2 || got.Rounds[1].State != "waiting" {
		t.Fatalf("scheduled retry missing: %+v", got)
	}
	_ = e.Store.Update(func(st *state.State) error {
		delivery := st.Engagements[g.ID].Rounds[1].Delivery
		delivery.State, delivery.LeaseUntil = "sending", now.Add(-time.Second)
		return nil
	})
	_, _ = e.Tick(context.Background())
	got, _ = e.GetEngagement(g.ID)
	if got.Rounds[1].Delivery.State != "unknown" || len(n.notifications) != 1 {
		t.Fatalf("lost send was replayed: %+v", got)
	}
}

func TestEngagementValidationAndExplicitMemory(t *testing.T) {
	e, g, now, _, _ := engagementFixture(t, "question")
	for _, mutate := range []func(*model.Engagement){
		func(g *model.Engagement) { g.Every = 0 },
		func(g *model.Engagement) { g.Timeout = 6 * time.Minute },
		func(g *model.Engagement) { g.Agent = []string{" "} },
		func(g *model.Engagement) { g.Workdir = "" },
		func(g *model.Engagement) { g.Channel = "missing-plugin" },
	} {
		bad := *g
		mutate(&bad)
		if err := e.ValidateEngagement(&bad); err == nil {
			t.Fatalf("invalid engagement accepted: %+v", bad)
		}
	}
	if _, err := e.SetMemory(string([]byte{0xff})); err == nil {
		t.Fatal("accepted invalid UTF-8")
	}
	if _, err := e.SetMemory("user context"); err != nil {
		t.Fatal(err)
	}
	cleared, err := e.SetMemory("")
	if err != nil || cleared.Text != "" || !cleared.UpdatedAt.Equal(*now) {
		t.Fatalf("clear=%+v err=%v", cleared, err)
	}
	_, _ = e.SetPresence(context.Background(), "away", now.Add(time.Hour))
	newSchedule := *g
	created, err := e.CreateEngagement(newSchedule)
	if err != nil || created.Active || len(created.Rounds) != 0 {
		t.Fatalf("created while away: %+v err=%v", created, err)
	}
}
