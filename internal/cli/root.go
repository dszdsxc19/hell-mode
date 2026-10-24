// Package cli presents a single discoverable contract for people and agents.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/dszdsxc19/hell-mode/internal/engine"
	"github.com/dszdsxc19/hell-mode/internal/fault"
	"github.com/dszdsxc19/hell-mode/internal/notify"
	"github.com/dszdsxc19/hell-mode/internal/state"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type app struct {
	root          *cobra.Command
	in            io.Reader
	out, errOut   io.Writer
	dir           string
	json, noInput bool
	jsonOnError   bool
	data          any
	silent        bool
	human         string
}

// Execute emits exactly one JSON envelope for finite commands, including failures.
func Execute(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer, version string) int {
	a := &app{in: in, out: out, errOut: errOut}
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--json" || arg == "--json=true" {
			a.jsonOnError = true
		}
	}
	root := a.command(version)
	root.SetArgs(args)
	cmd, err := root.ExecuteContextC(ctx)
	if err != nil && a.jsonOnError {
		a.json = true
	}
	if cmd == nil {
		cmd = root
	}
	if a.silent && err == nil {
		return 0
	}
	code := 0
	var detail any
	if err != nil {
		var typed *fault.Error
		if !errors.As(err, &typed) {
			exit, errorCode := 1, "operation_failed"
			if strings.HasPrefix(err.Error(), "unknown command") {
				exit, errorCode = 2, "invalid_argument"
			}
			typed = &fault.Error{Code: errorCode, Message: err.Error(), Exit: exit}
		}
		code = typed.Exit
		detail = map[string]any{"code": typed.Code, "message": typed.Message, "next_actions": []string{cmd.CommandPath() + " --help", "hell schema --json"}}
	}
	if a.json {
		if encodeErr := json.NewEncoder(out).Encode(map[string]any{"schema_version": "hell-mode/v1", "ok": err == nil, "command": cmd.CommandPath(), "data": a.data, "error": detail}); encodeErr != nil {
			return 1
		}
	} else {
		if a.human != "" && err == nil {
			fmt.Fprintln(out, a.human)
		} else if a.data != nil {
			b, _ := json.MarshalIndent(a.data, "", "  ")
			fmt.Fprintln(out, string(b))
		}
		if err != nil {
			fmt.Fprintln(errOut, "Error:", err)
			fmt.Fprintln(errOut, "Run:", cmd.CommandPath(), "--help")
		}
	}
	return code
}

func (a *app) command(version string) *cobra.Command {
	root := &cobra.Command{Use: "hell", Short: "Agent 没干完，你也别想睡 — entertainment-only agent task watcher", Long: `hell-mode is an entertainment-only task watcher written in Go.

Freeze a goal and verification argv, run an agent, then let a separate watchdog
record failures and schedule bounded notifications. The default log channel
simulates delivery. No command prompts interactively. arm/setup/plugin add
require --yes for writes; use --dry-run to inspect the plan first.

Agent contract: --json returns hell-mode/v1 envelopes, schema describes commands,
stdout contains only results, and process diagnostics go to stderr or run logs.
Exit codes: 0 success; 1 operational/unknown outcome; 2 invalid input;
3 goal unmet or child failed; 4 not found; 5 conflict/invalid event authority.
Task status reads return 0 even when the recorded goal is unmet.

hell challenge prints a persistent questioning skill for your current agent.
The conversation runs in the host agent; the CLI does not call a model.

Human boundaries: vanish --for 25m makes this state directory quiet until the
human explicitly returns with back. Timer expiry never calls the human back.
back does not replay old alerts. end stops watchdogs and keeps unfinished work.
These actions require the human's instruction, never an agent's initiative.

Use only for voluntary, bounded daytime experiments. Do not rely on this for
critical alerts or interrupt real sleep.`, Example: `  hell schema --json
  hell challenge "这个产品真的有人需要吗？" --json
  hell arm --goal "tests pass" --verify-json '["go","test","./..."]' --deadline +10m --dry-run --json
  hell watch --json
  hell status --json
  hell vanish --for 25m
  hell presence --json
  hell back
  hell end`, SilenceErrors: true, SilenceUsage: true}
	a.root = root
	root.CompletionOptions.DisableDefaultCmd = true
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if cmd == root || cmd.Name() == "schema" || cmd.Name() == "version" || cmd.Name() == "challenge" {
			return nil
		}
		if a.dir == "" {
			return fault.New("invalid_argument", 2, "state-dir must not be empty")
		}
		absolute, err := filepath.Abs(a.dir)
		if err != nil {
			return err
		}
		a.dir = absolute
		return nil
	}
	defaultDir := os.Getenv("HELL_STATE_DIR")
	if defaultDir == "" {
		base, err := os.UserConfigDir()
		if err == nil {
			defaultDir = filepath.Join(base, "hell-mode")
		}
	}
	root.PersistentFlags().StringVar(&a.dir, "state-dir", defaultDir, "State and run-log directory (env HELL_STATE_DIR); shared by CLI and watchdog")
	root.PersistentFlags().BoolVar(&a.json, "json", false, "Emit versioned JSON; watch streams newline-delimited envelopes")
	root.PersistentFlags().BoolVar(&a.noInput, "no-input", false, "Never prompt; also disconnect child stdin for hell run")
	root.SetOut(a.out)
	root.SetErr(a.errOut)
	root.SetIn(a.in)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return fault.New("invalid_argument", 2, "%s", err) })
	originalHelp := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if a.json {
			a.data = commandSchema(cmd)
			return
		}
		originalHelp(cmd, args)
		a.silent = true
	})
	root.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return fault.New("invalid_argument", 2, "unexpected arguments")
		}
		return cmd.Help()
	}
	root.AddCommand(&cobra.Command{Use: "schema", Short: "Describe every command and flag without touching state", Args: args(cobra.NoArgs), RunE: func(cmd *cobra.Command, _ []string) error {
		a.data = map[string]any{"protocol_version": "hell-mode/v1", "commands": commandSchema(root), "input_schemas": inputSchemas(), "exit_codes": map[string]string{"0": "success", "1": "operational_or_unknown", "2": "invalid_input", "3": "goal_unmet_or_child_failed", "4": "not_found", "5": "conflict_or_unauthorized"}}
		return nil
	}})
	root.AddCommand(&cobra.Command{Use: "version", Short: "Print CLI and protocol versions", Args: args(cobra.NoArgs), RunE: func(_ *cobra.Command, _ []string) error {
		a.data = map[string]string{"version": version, "protocol_version": "hell-mode/v1", "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH}
		return nil
	}})
	root.AddCommand(a.armCommand(), a.runCommand(), a.statusCommand(), a.verifyCommand(), a.eventCommand(), a.watchCommand(), a.actionCommand("takeover"), a.actionCommand("snooze"), a.actionCommand("disarm"), a.pluginCommand(), a.setupCommand(), a.hookCommand(), a.doctorCommand())
	root.AddCommand(a.presenceCommand(), a.vanishCommand(), a.backCommand(), a.endCommand())
	root.AddCommand(a.challengeCommand())
	root.AddCommand(a.engageCommand())
	return root
}

// commandSchema derives machine documentation from the same definitions as human help.
func commandSchema(cmd *cobra.Command) map[string]any {
	flags := []map[string]any{}
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		flags = append(flags, map[string]any{"name": f.Name, "type": f.Value.Type(), "default": f.DefValue, "description": f.Usage})
	})
	cmd.InheritedFlags().VisitAll(func(f *pflag.Flag) {
		flags = append(flags, map[string]any{"name": f.Name, "type": f.Value.Type(), "default": f.DefValue, "description": f.Usage, "inherited": true})
	})
	children := []map[string]any{}
	for _, child := range cmd.Commands() {
		if !child.Hidden {
			children = append(children, commandSchema(child))
		}
	}
	return map[string]any{"path": cmd.CommandPath(), "usage": cmd.UseLine(), "summary": cmd.Short, "description": cmd.Long, "examples": cmd.Example, "flags": flags, "subcommands": children}
}

func args(validate cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, values []string) error {
		if err := validate(cmd, values); err != nil {
			return fault.New("invalid_argument", 2, "%s", err)
		}
		return nil
	}
}

func (a *app) engine() engine.Engine {
	s := state.Store{Dir: a.dir}
	return engine.Engine{Store: s, Notifier: notify.Dispatcher{Store: s}}
}

// decodeInput bounds stdin/file payloads and rejects trailing JSON and unknown contract fields.
func (a *app) decodeInput(path string, result any) error {
	reader := a.in
	if path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		reader = f
	}
	raw, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
	if err != nil {
		return err
	}
	if len(raw) > 1<<20 {
		return fault.New("invalid_argument", 2, "input exceeds 1 MiB")
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return fault.New("invalid_argument", 2, "invalid JSON input: %s", err)
	}
	if hasNull(generic) {
		return fault.New("invalid_argument", 2, "JSON inputs do not accept null; omit optional fields instead")
	}
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	if err := d.Decode(result); err != nil {
		return fault.New("invalid_argument", 2, "invalid JSON input: %s", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fault.New("invalid_argument", 2, "expected exactly one JSON object")
	}
	return nil
}

func hasNull(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case []any:
		for _, child := range v {
			if hasNull(child) {
				return true
			}
		}
	case map[string]any:
		for _, child := range v {
			if hasNull(child) {
				return true
			}
		}
	}
	return false
}
