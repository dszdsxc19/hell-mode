package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dszdsxc19/hell-mode/internal/model"
	"github.com/dszdsxc19/hell-mode/internal/state"
	"github.com/dszdsxc19/hell-mode/pkg/protocol"
)

const engagementTestAgent = `["sh","-c","printf '%s' '{\"kind\":\"question\",\"message\":\"哪个实验能验证你的判断？\",\"basis\":\"你正在验证产品想法。\"}'"]`

func decodeEngagement(t *testing.T, response envelope) model.Engagement {
	t.Helper()
	var result model.Engagement
	if err := json.Unmarshal(response.Data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestEngageDiscoveryMemoryAndPreviewDoNotCreateState(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	for _, argv := range [][]string{
		{"engage", "--help"}, {"engage", "create", "--help"}, {"engage", "memory"}, {"engage", "list"},
		{"engage", "create", "--agent-json", engagementTestAgent, "--dry-run", "--yes"},
		{"engage", "create", "--file", "-", "--dry-run"},
	} {
		code, response, _ := invoke(t, dir, `{"agent":["sh","-c","exit 99"]}`, argv...)
		if code != 0 || !response.OK {
			t.Fatalf("argv=%v code=%d response=%+v", argv, code, response)
		}
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("read or preview created state: %v", err)
	}
	code, response, _ := invoke(t, dir, "", "schema")
	if code != 0 || !strings.Contains(string(response.Data), `"hell engage reply"`) || !strings.Contains(string(response.Data), `"engage_create"`) || !strings.Contains(string(response.Data), `"engage_agent_response"`) {
		t.Fatalf("engagement contract not discoverable: %s", response.Data)
	}
}

func TestEngageMemoryReplaceReadAndClear(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	text := "已确认：最近关注 AI 对人的主动提问。\n推测：希望借助短任务改善表达。\n"
	for _, want := range []string{text, ""} {
		code, response, _ := invoke(t, dir, want, "engage", "memory", "--file", "-")
		if code != 0 || !response.OK {
			t.Fatal(response)
		}
		code, response, _ = invoke(t, dir, "", "engage", "memory")
		var got model.PersonalMemory
		if err := json.Unmarshal(response.Data, &got); err != nil {
			t.Fatal(err)
		}
		if code != 0 || got.Text != want || got.UpdatedAt.IsZero() {
			t.Fatalf("memory changed or wasn't persisted: %+v", got)
		}
	}
}

func TestEngageCLIQuestionAnswerPauseAndHumanBoundary(t *testing.T) {
	dir := t.TempDir()
	code, response, _ := invoke(t, dir, "", "engage", "create", "--every", "1ns", "--at", "+0s", "--agent-json", engagementTestAgent, "--yes")
	if code != 0 || !response.OK {
		t.Fatal(response)
	}
	created := decodeEngagement(t, response)
	if !created.Active || !filepath.IsAbs(created.Agent[0]) {
		t.Fatalf("invalid saved contract: %+v", created)
	}
	code, response, _ = invoke(t, dir, "", "watch", "--once")
	if code != 0 || !response.OK {
		t.Fatal(response)
	}
	code, response, _ = invoke(t, dir, "", "engage", "show", created.ID)
	got := decodeEngagement(t, response)
	if code != 0 || len(got.Rounds) != 1 || got.Rounds[0].State != "waiting" || got.Rounds[0].Delivery.State != "simulated" {
		t.Fatalf("missing question: %+v", got)
	}
	roundID := got.Rounds[0].ID
	var out, stderr bytes.Buffer
	code = Execute(context.Background(), []string{"--state-dir", dir, "engage", "show", got.ID}, strings.NewReader(""), &out, &stderr, "test")
	if code != 0 || !strings.Contains(out.String(), got.Rounds[0].Message) || !strings.Contains(out.String(), got.Rounds[0].Basis) || stderr.Len() != 0 {
		t.Fatalf("unreadable invitation: code=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
	}
	invoke(t, dir, "", "watch", "--once")
	_, response, _ = invoke(t, dir, "", "engage", "show", got.ID)
	if again := decodeEngagement(t, response); len(again.Rounds) != 1 {
		t.Fatal("unanswered invitation did not block the next question")
	}
	answer := "我会先邀请三个真实用户试一次。\n保留原文。"
	code, response, _ = invoke(t, dir, answer, "engage", "reply", got.ID, "--round", roundID, "--file", "-")
	got = decodeEngagement(t, response)
	if code != 0 || got.Rounds[0].State != "answered" || got.Rounds[0].Answer != answer {
		t.Fatalf("reply lost: %+v", got)
	}
	code, response, _ = invoke(t, dir, "", "engage", "skip", got.ID, "--round", roundID)
	if code != 5 || response.OK {
		t.Fatal("stale round was processed twice")
	}
	invoke(t, dir, "", "vanish", "--for", "25m")
	code, response, _ = invoke(t, dir, "", "engage", "resume", got.ID)
	if code != 5 || response.OK {
		t.Fatal("resumed while the human was away")
	}
	invoke(t, dir, "", "back")
	_, response, _ = invoke(t, dir, "", "engage", "show", got.ID)
	if decodeEngagement(t, response).Active {
		t.Fatal("back resumed a paused plan")
	}
	code, response, _ = invoke(t, dir, "", "engage", "resume", got.ID)
	if code != 0 || !decodeEngagement(t, response).Active {
		t.Fatal("explicit resume failed")
	}
	code, response, _ = invoke(t, dir, "", "engage", "pause", got.ID)
	if code != 0 || decodeEngagement(t, response).Active {
		t.Fatal("pause failed")
	}
}

func TestEngageRejectsInvalidConfigurationWithoutWrites(t *testing.T) {
	tests := []struct {
		argv  []string
		input string
		code  int
	}{
		{[]string{"create", "--agent-json", engagementTestAgent}, "", 2},
		{[]string{"create", "--agent-json", engagementTestAgent, "--every", "0s", "--yes"}, "", 2},
		{[]string{"create", "--agent-json", engagementTestAgent, "--timeout", "6m", "--yes"}, "", 2},
		{[]string{"create", "--agent-json", `["sh",null]`, "--yes"}, "", 2},
		{[]string{"create", "--agent-json", `[]`, "--yes"}, "", 2},
		{[]string{"create", "--agent-json", engagementTestAgent, "--workdir", "", "--yes"}, "", 2},
		{[]string{"create", "--agent-json", engagementTestAgent, "--channel", "missing", "--yes"}, "", 4},
		{[]string{"create", "--file", "-", "--yes"}, `{"agent":["sh"],"unexpected":true}`, 2},
		{[]string{"create", "--file", "-", "--yes"}, `{"agent":["sh"],"focus":null}`, 2},
		{[]string{"create", "--file", "-", "--yes"}, `{"agent":["sh"]} {}`, 2},
		{[]string{"create", "--file", "-", "--every", "4h", "--yes"}, `{"agent":["sh"]}`, 2},
		{[]string{"create", "--file", "", "--yes"}, "", 2},
		{[]string{"memory", "--file", ""}, "", 2},
		{[]string{"memory", "--file", "-"}, strings.Repeat("a", (1<<20)+1), 2},
		{[]string{"memory", "--file", "-"}, string([]byte{0xff}), 2},
		{[]string{"reply", "missing", "--round", "r", "--text", "", "--file", "-"}, "", 2},
		{[]string{"reply", "missing", "--round", "r", "--text", " "}, "", 2},
		{[]string{"skip", "missing"}, "", 2},
		{[]string{"show", "missing"}, "", 4},
	}
	for _, test := range tests {
		dir := filepath.Join(t.TempDir(), "absent")
		code, response, _ := invoke(t, dir, test.input, append([]string{"engage"}, test.argv...)...)
		if code != test.code || response.OK || response.Error == nil {
			t.Fatalf("argv=%v code=%d response=%+v", test.argv, code, response)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("invalid input created state: argv=%v err=%v", test.argv, err)
		}
	}
}

func TestEngagePluginRemovalRequiresPausedPlanAndResumeRequiresRegistration(t *testing.T) {
	dir := t.TempDir()
	store := state.Store{Dir: dir}
	if err := store.Update(func(st *state.State) error {
		st.Plugins["demo"] = protocol.Manifest{Name: "demo", Command: []string{"sh"}, ProtocolVersion: protocol.Version}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	code, response, _ := invoke(t, dir, "", "engage", "create", "--agent-json", engagementTestAgent, "--channel", "demo", "--yes")
	if code != 0 {
		t.Fatal(response)
	}
	g := decodeEngagement(t, response)
	code, _, _ = invoke(t, dir, "", "plugin", "remove", "demo")
	if code != 5 {
		t.Fatal("removed a plugin used by an active conversation")
	}
	invoke(t, dir, "", "engage", "pause", g.ID)
	code, response, _ = invoke(t, dir, "", "plugin", "remove", "demo")
	if code != 0 {
		t.Fatal(response)
	}
	code, response, _ = invoke(t, dir, "", "engage", "resume", g.ID)
	if code != 4 || response.OK {
		t.Fatal("resumed with an unregistered plugin")
	}
}

func TestContinuousWatchKeepsTaskScansAliveWhileAIGenerates(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "agent-started")
	agent, _ := json.Marshal([]string{"sh", "-c", `printf started > "$1"; sleep 30`, "engage-test", marker})
	code, response, _ := invoke(t, dir, "", "engage", "create", "--agent-json", string(agent), "--at", "+0s", "--timeout", "30s", "--yes")
	if code != 0 {
		t.Fatal(response)
	}
	plan := decodeEngagement(t, response)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() {
		done <- Execute(ctx, []string{"--state-dir", dir, "watch", "--interval", "10ms", "--json"}, strings.NewReader(""), io.Discard, io.Discard, "test")
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("AI command did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	store := state.Store{Dir: dir}
	before, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	id := armForTest(t, dir, `["sh","-c","exit 1"]`)
	if code, _, _ := invoke(t, dir, "", "verify", id); code != 3 {
		t.Fatal("expected unmet test task")
	}
	for {
		st, err := store.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if st.WatchdogAt.After(before.WatchdogAt) && st.Tasks[id].Incident != nil && len(st.Tasks[id].Incident.Attempts) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("slow AI blocked task monitoring or watchdog liveness")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if code, _, _ := invoke(t, dir, "", "end"); code != 0 {
		t.Fatal("end failed")
	}
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("watch exited %d", code)
		}
		done <- code // Keep cleanup non-blocking after the assertion.
	case <-time.After(3 * time.Second):
		t.Fatal("watch waited for slow AI after end")
	}
	st, err := store.Snapshot()
	if err != nil || len(st.Engagements[plan.ID].Rounds) != 1 || st.Engagements[plan.ID].Rounds[0].State != "skipped" {
		t.Fatalf("in-flight invitation survived end: %+v err=%v", st, err)
	}
}
