package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dszdsxc19/hell-mode/internal/model"
)

func TestPresenceDiscoveryDoesNotCreateState(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	for _, command := range []string{"presence", "vanish", "back", "end"} {
		code, response, _ := invoke(t, dir, "", command, "--help")
		if code != 0 || !response.OK {
			t.Fatalf("%s: %d %+v", command, code, response)
		}
	}
	code, response, _ := invoke(t, dir, "", "presence")
	if code != 0 || !bytes.Contains(response.Data, []byte(`"mode":"available"`)) {
		t.Fatal(code, string(response.Data))
	}
	code, _, _ = invoke(t, dir, "", "vanish", "--for", "0s")
	if code != 2 {
		t.Fatal(code)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("read-only discovery or rejected input created state")
	}
}

func TestVanishExpiryBackAndExplicitRearm(t *testing.T) {
	dir := t.TempDir()
	id := armForTest(t, dir, `["sh","-c","exit 1"]`)
	if code, _, _ := invoke(t, dir, "", "verify", id); code != 3 {
		t.Fatal(code)
	}
	if code, _, _ := invoke(t, dir, "", "vanish", "--for", "100ms"); code != 0 {
		t.Fatal(code)
	}
	time.Sleep(120 * time.Millisecond)
	code, response, _ := invoke(t, dir, "", "presence")
	if code != 0 || !bytes.Contains(response.Data, []byte(`"mode":"away"`)) {
		t.Fatal(code, string(response.Data))
	}
	for _, argv := range [][]string{{"watch", "--once"}, {"back"}, {"watch", "--once"}} {
		if code, _, _ := invoke(t, dir, "", argv...); code != 0 {
			t.Fatal(argv, code)
		}
	}
	_, response, _ = invoke(t, dir, "", "status", id)
	var task model.Task
	_ = json.Unmarshal(response.Data, &task)
	if !task.Quiet || task.Verdict != "unmet" || len(task.Incident.Attempts) != 0 {
		t.Fatal(task)
	}
	if code, _, _ := invoke(t, dir, "", "run", id, "--rearm", "--no-input", "--", "sh", "-c", "exit 0"); code != 3 {
		t.Fatal(code)
	}
	if code, _, _ := invoke(t, dir, "", "watch", "--once"); code != 0 {
		t.Fatal(code)
	}
	_, response, _ = invoke(t, dir, "", "status", id)
	_ = json.Unmarshal(response.Data, &task)
	if task.Quiet || len(task.Incident.Attempts) != 1 {
		t.Fatal(task)
	}
}

func TestEndKeepsUnfinishedWorkAndHasAFinalLine(t *testing.T) {
	dir := t.TempDir()
	id := armForTest(t, dir, `["sh","-c","exit 1"]`)
	_, _, _ = invoke(t, dir, "", "verify", id)
	var out, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"--state-dir", dir, "end"}, strings.NewReader(""), &out, &stderr, "test")
	if code != 0 || stderr.Len() != 0 || out.String() != "任务未完成。\n\n你决定今天到此为止。\n\n看守已停止。\n" {
		t.Fatal(code, out.String(), stderr.String())
	}
	out.Reset()
	code = Execute(context.Background(), []string{"--state-dir", dir, "watch", "--json"}, strings.NewReader(""), &out, &stderr, "test")
	if code != 0 || out.Len() != 0 {
		t.Fatal("ended watch did not exit silently", code, out.String())
	}
	code, response, _ := invoke(t, dir, "", "watch", "--once")
	if code != 0 || !bytes.Contains(response.Data, []byte(`"stopped":true`)) {
		t.Fatal(code, string(response.Data))
	}
	_, response, _ = invoke(t, dir, "", "status", id)
	var task model.Task
	_ = json.Unmarshal(response.Data, &task)
	if task.Verdict != "unmet" || !task.Quiet {
		t.Fatal(task)
	}
}
