package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dszdsxc19/hell-mode/assets"
)

func TestChallengeReturnsHostInstructionsWithoutState(t *testing.T) {
	for _, topic := range []string{"", "我要做一个产品\n条件：保持本地存储，保留 \"原始输入\""} {
		t.Run(topic, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "absent")
			argv := []string{"challenge"}
			if topic != "" {
				argv = append(argv, topic)
			}
			code, response, stderr := invoke(t, dir, "", argv...)
			if code != 0 || !response.OK || stderr != "" {
				t.Fatalf("code=%d response=%+v stderr=%s", code, response, stderr)
			}
			var result struct {
				Skill        string `json:"skill"`
				Topic        string `json:"topic"`
				Instructions string `json:"instructions"`
				HostRequired bool   `json:"host_required"`
			}
			if err := json.Unmarshal(response.Data, &result); err != nil {
				t.Fatal(err)
			}
			if result.Skill != "hell-challenge" || result.Topic != topic || result.Instructions == "" || result.Instructions != assets.ChallengeSkill || !result.HostRequired {
				t.Fatalf("unexpected host contract: %+v", result)
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatalf("challenge created state: %v", err)
			}
		})
	}
}

func TestChallengeDoesNotRequireStateDirectory(t *testing.T) {
	code, response, _ := invoke(t, "", "", "challenge")
	if code != 0 || !response.OK {
		t.Fatalf("read-only instructions require state: %+v", response)
	}
}

func TestChallengeInvalidArguments(t *testing.T) {
	for _, argv := range [][]string{{"challenge", ""}, {"challenge", " \t\n"}, {"challenge", "one", "two"}} {
		code, response, _ := invoke(t, filepath.Join(t.TempDir(), "absent"), "", argv...)
		if code != 2 || response.OK || response.Error == nil || response.Error.Code != "invalid_argument" {
			t.Fatalf("argv=%v code=%d response=%+v", argv, code, response)
		}
	}
}

func TestChallengeHumanOutput(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	topic := "这个计划的前提成立吗？"
	var out, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"--state-dir", dir, "challenge", topic}, strings.NewReader(""), &out, &stderr, "test")
	if code != 0 || stderr.Len() != 0 || !strings.Contains(out.String(), assets.ChallengeSkill) || !strings.Contains(out.String(), topic) {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("human output created state")
	}
}

func TestChallengeDiscovery(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	code, response, _ := invoke(t, dir, "", "challenge", "--help")
	if code != 0 || !response.OK {
		t.Fatal(response)
	}
	var help struct {
		Path  string `json:"path"`
		Usage string `json:"usage"`
	}
	if err := json.Unmarshal(response.Data, &help); err != nil {
		t.Fatal(err)
	}
	if help.Path != "hell challenge" || !strings.Contains(help.Usage, "[TOPIC]") {
		t.Fatalf("missing challenge help: %s", response.Data)
	}
	code, response, _ = invoke(t, dir, "", "schema")
	if code != 0 || !response.OK {
		t.Fatal(response)
	}
	var schema struct {
		Commands struct {
			Children []struct {
				Path string `json:"path"`
			} `json:"subcommands"`
		} `json:"commands"`
	}
	if err := json.Unmarshal(response.Data, &schema); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, command := range schema.Commands.Children {
		found = found || command.Path == "hell challenge"
	}
	if !found {
		t.Fatalf("challenge missing from schema: %s", response.Data)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("discovery created state")
	}
}

func TestSetupExportsBothSkills(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	output := filepath.Join(t.TempDir(), "generated")
	code, response, _ := invoke(t, dir, "", "setup", "--output", output)
	if code != 0 || !response.OK {
		t.Fatal(response)
	}
	var preview struct {
		DryRun               bool              `json:"dry_run"`
		Files                map[string]string `json:"files"`
		LiveSettingsModified bool              `json:"live_settings_modified"`
	}
	if err := json.Unmarshal(response.Data, &preview); err != nil {
		t.Fatal(err)
	}
	if !preview.DryRun || preview.LiveSettingsModified || len(preview.Files) != 3 || preview.Files[".agents/skills/hell-challenge/SKILL.md"] != assets.ChallengeSkill || preview.Files[".agents/skills/hell-mode/SKILL.md"] != assets.Skill {
		t.Fatalf("unexpected setup preview: %+v", preview)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("preview created files")
	}
	code, response, _ = invoke(t, dir, "", "setup", "--output", output, "--yes")
	if code != 0 || !response.OK {
		t.Fatal(response)
	}
	for name, want := range preview.Files {
		got, err := os.ReadFile(filepath.Join(output, name))
		if err != nil || string(got) != want {
			t.Fatalf("export mismatch for %s: %v", name, err)
		}
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("setup created watcher state")
	}
}

func TestSetupPreservesExistingChallengeSkill(t *testing.T) {
	output := t.TempDir()
	path := filepath.Join(output, ".agents", "skills", "hell-challenge", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("user customization"), 0600); err != nil {
		t.Fatal(err)
	}
	code, response, _ := invoke(t, t.TempDir(), "", "setup", "--output", output, "--yes")
	if code != 5 || response.OK {
		t.Fatal("setup did not reject existing challenge skill")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "user customization" {
		t.Fatal("setup changed existing challenge skill")
	}
	for _, name := range []string{".claude/settings.hell-mode.json", ".agents/skills/hell-mode/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(output, name)); !os.IsNotExist(err) {
			t.Fatalf("setup partially wrote %s", name)
		}
	}
}
