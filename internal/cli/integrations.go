package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dszdsxc19/hell-mode/assets"
	"github.com/dszdsxc19/hell-mode/internal/fault"
	"github.com/dszdsxc19/hell-mode/internal/model"
	"github.com/spf13/cobra"
)

// setupCommand exports reviewable integration artifacts without touching live host settings.
func (a *app) setupCommand() *cobra.Command {
	var output string
	var yes, dry bool
	cmd := &cobra.Command{Use: "setup", Short: "Export Claude Code hook configuration and an Agent Skill for review", Long: `Writes a settings fragment and .agents/skills/hell-mode/SKILL.md under the
explicit output directory. Existing files and symlinked child directories are
refused. Nothing is merged into .claude/settings.json or installed globally.
The hook requires hell on PATH and a process launched via hell run.
Without --yes (or with --dry-run), this command only prints a plan.`, Example: "  hell setup --output ./integration --dry-run --json\n  hell setup --output ./integration --yes --json", Args: args(cobra.NoArgs), RunE: func(_ *cobra.Command, _ []string) error {
		if output == "" {
			return fault.New("invalid_argument", 2, "--output is required")
		}
		root, err := filepath.Abs(output)
		if err != nil {
			return err
		}
		hook := map[string]any{"type": "command", "command": "hell hook claude-code", "timeout": 30}
		settings := map[string]any{"hooks": map[string]any{"Stop": []any{map[string]any{"hooks": []any{hook}}}, "StopFailure": []any{map[string]any{"hooks": []any{hook}}}, "Notification": []any{map[string]any{"matcher": "permission_prompt|idle_prompt", "hooks": []any{hook}}}}}
		raw, _ := json.MarshalIndent(settings, "", "  ")
		files := map[string]string{".claude/settings.hell-mode.json": string(raw) + "\n", ".agents/skills/hell-mode/SKILL.md": assets.Skill}
		a.data = map[string]any{"dry_run": dry || !yes, "output": root, "files": files, "live_settings_modified": false}
		if dry || !yes {
			return nil
		}
		// Preflight all leaves before creating any; exclusive opens also prevent overwrite races.
		for name := range files {
			if _, err := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(err) {
				return fault.New("conflict", 5, "output already exists or cannot be inspected: %s", name)
			}
		}
		created := []string{}
		for name, content := range files {
			path := filepath.Join(root, name)
			if err := safeParents(root, filepath.Dir(name)); err != nil {
				for _, p := range created {
					_ = os.Remove(p)
				}
				return err
			}
			file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				for _, p := range created {
					_ = os.Remove(p)
				}
				return err
			}
			_, err = file.WriteString(content)
			closeErr := file.Close()
			if err == nil {
				err = closeErr
			}
			created = append(created, path)
			if err != nil {
				for _, p := range created {
					_ = os.Remove(p)
				}
				return err
			}
		}
		return nil
	}}
	cmd.Flags().StringVar(&output, "output", "", "Explicit directory for reviewable generated artifacts")
	cmd.Flags().BoolVar(&yes, "yes", false, "Write the reviewed artifacts; never overwrite existing files")
	cmd.Flags().BoolVar(&dry, "dry-run", false, "Print artifact contents without creating directories")
	return cmd
}

func safeParents(root, relative string) error {
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	current := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			if err := os.Mkdir(current, 0700); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fault.New("conflict", 5, "setup refuses symlink or non-directory parent")
		}
	}
	return nil
}

// hookCommand adapts observations only; it never emits a stop-blocking host decision.
func (a *app) hookCommand() *cobra.Command {
	root := &cobra.Command{Use: "hook", Short: "Adapt host lifecycle observations to scoped run events"}
	root.AddCommand(&cobra.Command{Use: "claude-code", Short: "Read one Claude Code hook payload from stdin", Long: "Supports Stop, StopFailure and permission_prompt/idle_prompt Notification. Requires the task/run/token environment supplied by hell run. It does not read transcripts or modify Claude's continuation decisions.", Example: "  hell hook claude-code < hook-payload.json", Args: args(cobra.NoArgs), RunE: func(cmd *cobra.Command, _ []string) error {
		raw, err := io.ReadAll(io.LimitReader(a.in, (1<<20)+1))
		if err != nil {
			return err
		}
		if len(raw) > 1<<20 {
			return fault.New("invalid_argument", 2, "hook payload exceeds 1 MiB")
		}
		var payload struct {
			Event      string            `json:"hook_event_name"`
			Type       string            `json:"notification_type"`
			Background []json.RawMessage `json:"background_tasks"`
			Crons      []json.RawMessage `json:"session_crons"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return fault.New("invalid_argument", 2, "invalid hook JSON")
		}
		if os.Getenv("HELL_TASK_ID") == "" && os.Getenv("HELL_RUN_ID") == "" && os.Getenv("HELL_EVENT_TOKEN") == "" {
			a.silent = !a.json
			a.data = map[string]any{"ignored": true, "reason": "no_hell_run_context"}
			return nil
		}
		typeName := ""
		switch payload.Event {
		case "Stop":
			typeName = "turn_completed"
		case "StopFailure":
			typeName = "failed"
		case "Notification":
			if payload.Type == "permission_prompt" || payload.Type == "idle_prompt" {
				typeName = "waiting"
			}
		}
		if typeName == "" {
			a.silent = !a.json
			a.data = map[string]any{"ignored": true}
			return nil
		}
		// Random invocation identity avoids deduplicating legitimate repeated identical host payloads.
		hash := sha256.Sum256(append(raw, []byte(model.ID("hook"))...))
		event := model.Event{ID: hex.EncodeToString(hash[:]), TaskID: os.Getenv("HELL_TASK_ID"), RunID: os.Getenv("HELL_RUN_ID"), Type: typeName, Source: "claude-code", BackgroundWork: len(payload.Background) > 0 || len(payload.Crons) > 0}
		requested, err := a.engine().Event(event, os.Getenv("HELL_EVENT_TOKEN"))
		if err != nil {
			return err
		}
		a.data = map[string]any{"task_id": event.TaskID, "run_id": event.RunID, "verification_requested": requested}
		a.silent = !a.json
		return nil
	}})
	return root
}

func (a *app) doctorCommand() *cobra.Command {
	return &cobra.Command{Use: "doctor", Short: "Inspect local state, watchdog recency and configuration without sending notifications", Example: "  hell doctor --json", Args: args(cobra.NoArgs), RunE: func(_ *cobra.Command, _ []string) error {
		st, err := a.engine().Store.Snapshot()
		if err != nil {
			return err
		}
		a.data = map[string]any{"state_dir": a.dir, "watchdog_last_seen": st.WatchdogAt, "watchdog_seen_within_5s": !st.WatchdogAt.IsZero() && time.Since(st.WatchdogAt) < 5*time.Second, "pushover_credentials_present": os.Getenv("HELL_PUSHOVER_APP_TOKEN") != "" && os.Getenv("HELL_PUSHOVER_USER_KEY") != "", "registered_plugins": len(st.Plugins), "remote_monitoring": false, "default_channel": "log", "entertainment_only": true}
		return nil
	}}
}
