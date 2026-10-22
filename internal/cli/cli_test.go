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

	"github.com/dszdsxc19/hell-mode/internal/engine"
	"github.com/dszdsxc19/hell-mode/internal/model"
	"github.com/dszdsxc19/hell-mode/internal/notify"
	"github.com/dszdsxc19/hell-mode/internal/state"
)

type envelope struct {
	OK      bool            `json:"ok"`
	Version string          `json:"schema_version"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func invoke(t *testing.T, dir, input string, arguments ...string) (int, envelope, string) {
	t.Helper()
	var out, stderr bytes.Buffer
	argv := append([]string{"--state-dir", dir, "--json"}, arguments...)
	code := Execute(context.Background(), argv, strings.NewReader(input), &out, &stderr, "test")
	var result envelope
	decoder := json.NewDecoder(&out)
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("exit=%d stdout=%q stderr=%q decode=%v", code, out.String(), stderr.String(), err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatal("stdout had extra data")
	}
	if result.Version != "hell-mode/v1" {
		t.Fatal(result)
	}
	return code, result, stderr.String()
}

func armForTest(t *testing.T, dir string, verify string) string {
	t.Helper()
	code, response, _ := invoke(t, dir, "", "arm", "--goal", "test goal", "--verify-json", verify, "--deadline", "+10m", "--yes")
	if code != 0 {
		t.Fatalf("arm failed: %s", response.Data)
	}
	var task model.Task
	if err := json.Unmarshal(response.Data, &task); err != nil {
		t.Fatal(err)
	}
	return task.ID
}

func TestHelpSchemaAndDryRunDoNotCreateState(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	for _, argv := range [][]string{{"--help"}, {"arm", "--help"}, {"schema"}, {"arm", "--goal", "goal", "--verify-json", `["sh","-c","exit 0"]`, "--deadline", "+1m", "--dry-run"}} {
		code, result, _ := invoke(t, dir, "", argv...)
		if code != 0 || !result.OK {
			t.Fatalf("%v: %+v", argv, result)
		}
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("read-only discovery created state")
	}
	code, result, _ := invoke(t, dir, "", "schema")
	if code != 0 {
		t.Fatal(code)
	}
	var schema map[string]json.RawMessage
	_ = json.Unmarshal(result.Data, &schema)
	if _, ok := schema["input_schemas"]; !ok {
		t.Fatal("missing machine input contracts")
	}
}

func TestMachineErrorsAndNoImplicitConfirmation(t *testing.T) {
	dir := t.TempDir()
	for _, test := range []struct {
		argv      []string
		code      int
		errorCode string
	}{
		{[]string{"status", "missing"}, 4, "not_found"},
		{[]string{"arm", "--goal", "goal", "--verify-json", `["sh","-c","exit 0"]`, "--deadline", "+1m"}, 2, "confirmation_required"},
		{[]string{"arm", "--not-a-flag"}, 2, "invalid_argument"},
		{[]string{"not-a-command"}, 2, "invalid_argument"},
		{[]string{"event", "--file", "-"}, 2, "invalid_argument"},
		{[]string{"arm", "--goal", "goal", "--verify-json", `["sh","-c","exit 0"]`, "--deadline", "+1m", "--max-attempts", "0", "--yes"}, 2, "invalid_argument"},
	} {
		code, response, _ := invoke(t, dir, `{"unexpected":true}`, test.argv...)
		if code != test.code || response.OK || response.Error == nil || response.Error.Code != test.errorCode {
			t.Fatalf("argv=%v code=%d response=%+v", test.argv, code, response)
		}
	}
}

func TestContractJSONDefaultsAndNullRejection(t *testing.T) {
	dir := t.TempDir()
	code, result, _ := invoke(t, dir, `{"goal":"goal","verify":["sh","-c","exit 0"],"deadline":"+1m"}`, "arm", "--file", "-", "--yes")
	if code != 0 {
		t.Fatalf("minimal file contract rejected: %+v", result)
	}
	var task model.Task
	_ = json.Unmarshal(result.Data, &task)
	if task.Policy.MaxAttempts != 3 || task.Policy.Channel != "log" {
		t.Fatal(task)
	}
	for _, input := range []string{`null`, `{"goal":"goal","verify":["sh",null],"deadline":"+1m"}`} {
		code, result, _ := invoke(t, dir, input, "arm", "--file", "-", "--yes")
		if code != 2 || result.OK {
			t.Fatal("null accepted")
		}
	}
}

func TestRunnerJSONFailureWatchAndTakeover(t *testing.T) {
	dir := t.TempDir()
	id := armForTest(t, dir, `["sh","-c","echo check-output; exit 1"]`)
	code, result, stderr := invoke(t, dir, "", "run", id, "--no-input", "--", "sh", "-c", "echo child-stdout; echo child-stderr >&2; exit 0")
	if code != 3 || result.OK || stderr != "" {
		t.Fatalf("code=%d response=%+v stderr=%q", code, result, stderr)
	}
	var run struct {
		Task model.Task `json:"task"`
		Log  string     `json:"log_path"`
	}
	if err := json.Unmarshal(result.Data, &run); err != nil {
		t.Fatal(err)
	}
	if run.Task.Verdict != "unmet" {
		t.Fatal(run.Task)
	}
	logs, err := os.ReadFile(run.Log)
	if err != nil || !strings.Contains(string(logs), "child-stdout") || !strings.Contains(string(logs), "child-stderr") {
		t.Fatalf("%s %v", logs, err)
	}
	if code, _, _ := invoke(t, dir, "", "watch", "--once"); code != 0 {
		t.Fatal(code)
	}
	if code, _, _ := invoke(t, dir, "", "takeover", id); code != 0 {
		t.Fatal(code)
	}
	code, status, _ := invoke(t, dir, "", "status", id)
	if code != 0 {
		t.Fatal(code)
	}
	var task model.Task
	_ = json.Unmarshal(status.Data, &task)
	if task.Verdict != "unmet" || task.Incident.State != "taken_over" || len(task.Incident.Attempts) != 1 || !task.Incident.Attempts[0].Cancelled {
		t.Fatal(task)
	}
}

func TestRunnerPassAndLaunchFailureAreRecorded(t *testing.T) {
	dir := t.TempDir()
	id := armForTest(t, dir, `["sh","-c","exit 0"]`)
	code, _, _ := invoke(t, dir, "", "run", id, "--", "sh", "-c", "echo output")
	if code != 0 {
		t.Fatal(code)
	}
	id = armForTest(t, dir, `["sh","-c","exit 1"]`)
	code, _, _ = invoke(t, dir, "", "run", id, "--", "/does/not/exist")
	if code != 1 {
		t.Fatal(code)
	}
	_, response, _ := invoke(t, dir, "", "status", id)
	var task model.Task
	_ = json.Unmarshal(response.Data, &task)
	if task.Verdict != "unknown" || task.CurrentRun().State != "exited" || task.Incident == nil {
		t.Fatal(task)
	}
}

func TestHookStaysSilentAndVerifiesCurrentRun(t *testing.T) {
	dir := t.TempDir()
	id := armForTest(t, dir, `["sh","-c","exit 0"]`)
	store := state.Store{Dir: dir}
	e := engine.Engine{Store: store, Notifier: notify.Dispatcher{Store: store}}
	running, token, err := e.StartRun(id, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HELL_TASK_ID", id)
	t.Setenv("HELL_RUN_ID", running.CurrentRunID)
	t.Setenv("HELL_EVENT_TOKEN", token)
	var out, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"--state-dir", dir, "hook", "claude-code"}, strings.NewReader(`{"hook_event_name":"Stop","session_id":"example"}`), &out, &stderr, "test")
	if code != 0 || out.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), stderr.String())
	}
	got, err := e.Get(id)
	if err != nil || got.Verdict != "pending" || !got.PendingVerification {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := e.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err = e.Get(id)
	if err != nil || got.Verdict != "met" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestUnscopedHookIsSilentAndDoesNotCreateState(t *testing.T) {
	t.Setenv("HELL_TASK_ID", "")
	t.Setenv("HELL_RUN_ID", "")
	t.Setenv("HELL_EVENT_TOKEN", "")
	dir := filepath.Join(t.TempDir(), "absent")
	var out, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"--state-dir", dir, "hook", "claude-code"}, strings.NewReader(`{"hook_event_name":"Stop"}`), &out, &stderr, "test")
	if code != 0 || out.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("unscoped hook created state")
	}
}

func TestClaudeLifecycleMapping(t *testing.T) {
	for _, test := range []struct {
		name, payload, reason string
		pending               bool
	}{
		{"stop", `{"hook_event_name":"Stop"}`, "", true},
		{"background", `{"hook_event_name":"Stop","background_tasks":[{"id":"job"}]}`, "", false},
		{"failure", `{"hook_event_name":"StopFailure"}`, "run_failed", true},
		{"permission", `{"hook_event_name":"Notification","notification_type":"permission_prompt"}`, "waiting_for_input", false},
		{"idle", `{"hook_event_name":"Notification","notification_type":"idle_prompt"}`, "waiting_for_input", false},
		{"other", `{"hook_event_name":"Notification","notification_type":"other"}`, "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			id := armForTest(t, dir, `["sh","-c","exit 1"]`)
			store := state.Store{Dir: dir}
			e := engine.Engine{Store: store, Notifier: notify.Dispatcher{Store: store}}
			running, token, err := e.StartRun(id, false)
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("HELL_TASK_ID", id)
			t.Setenv("HELL_RUN_ID", running.CurrentRunID)
			t.Setenv("HELL_EVENT_TOKEN", token)
			code, _, _ := invoke(t, dir, test.payload, "hook", "claude-code")
			if code != 0 {
				t.Fatal(code)
			}
			got, err := e.Get(id)
			if err != nil {
				t.Fatal(err)
			}
			if got.PendingVerification != test.pending {
				t.Fatal(got)
			}
			if test.reason == "" {
				if got.Incident != nil {
					t.Fatal(got)
				}
			} else if got.Incident == nil || got.Incident.Reason != test.reason {
				t.Fatal(got)
			}
		})
	}
}

func TestSetupPreviewRefusesOverwriteAndSymlink(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(t.TempDir(), "generated")
	code, _, _ := invoke(t, dir, "", "setup", "--output", output, "--dry-run")
	if code != 0 {
		t.Fatal(code)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("preview wrote files")
	}
	code, _, _ = invoke(t, dir, "", "setup", "--output", output, "--yes")
	if code != 0 {
		t.Fatal(code)
	}
	code, result, _ := invoke(t, dir, "", "setup", "--output", output, "--yes")
	if code != 5 || result.OK {
		t.Fatal("overwrote existing setup")
	}
	unsafeRoot := t.TempDir()
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(unsafeRoot, ".claude")); err != nil {
		t.Skip(err)
	}
	code, result, _ = invoke(t, dir, "", "setup", "--output", unsafeRoot, "--yes")
	if code != 5 || result.OK {
		t.Fatal("followed symlink")
	}
	entries, _ := os.ReadDir(target)
	if len(entries) != 0 {
		t.Fatal("wrote outside output root")
	}
}
