package cli

import (
	"github.com/dszdsxc19/hell-mode/internal/fault"
	"github.com/dszdsxc19/hell-mode/internal/plugins"
	"github.com/dszdsxc19/hell-mode/internal/state"
	"github.com/dszdsxc19/hell-mode/pkg/protocol"
	"github.com/spf13/cobra"
)

// pluginCommand registers explicit manifests rather than auto-executing names found on PATH.
func (a *app) pluginCommand() *cobra.Command {
	root := &cobra.Command{Use: "plugin", Short: "Manage trusted notification subprocess plugins", Long: "Plugins run with your OS user's permissions; this is process isolation, not a sandbox. Use only trusted executables. No downloads or package manager is invoked."}
	root.AddCommand(&cobra.Command{Use: "list", Short: "List built-ins and registered manifests without executing plugins", Args: args(cobra.NoArgs), RunE: func(_ *cobra.Command, _ []string) error {
		st, err := a.engine().Store.Snapshot()
		if err != nil {
			return err
		}
		a.data = map[string]any{"builtins": []string{"log", "pushover"}, "registered": st.Plugins}
		return nil
	}})
	var file string
	var yes, dry bool
	add := &cobra.Command{Use: "add", Short: "Validate and register a manifest (--yes executes describe)", Long: "A dry run only validates the manifest and resolves the executable. Registration executes its describe method with a timeout and checks notify/cancel capabilities. Existing names are never overwritten.", Example: "  hell plugin add --manifest plugin.json --dry-run --json\n  hell plugin add --manifest plugin.json --yes --json", Args: args(cobra.NoArgs), RunE: func(cmd *cobra.Command, _ []string) error {
		if file == "" {
			return fault.New("invalid_argument", 2, "--manifest is required")
		}
		var manifest protocol.Manifest
		if err := a.decodeInput(file, &manifest); err != nil {
			return err
		}
		manifest, err := plugins.Validate(manifest)
		if err != nil {
			return fault.New("invalid_argument", 2, "%s", err)
		}
		if dry {
			a.data = map[string]any{"dry_run": true, "manifest": manifest, "executes_plugin": false}
			return nil
		}
		if !yes {
			return fault.New("confirmation_required", 2, "pass --yes to trust this executable and run describe")
		}
		if _, err := plugins.Call(cmd.Context(), manifest, "describe", nil); err != nil {
			return err
		}
		err = a.engine().Store.Update(func(st *state.State) error {
			if _, exists := st.Plugins[manifest.Name]; exists {
				return fault.New("conflict", 5, "plugin name already registered")
			}
			st.Plugins[manifest.Name] = manifest
			return nil
		})
		a.data = manifest
		return err
	}}
	add.Flags().StringVar(&file, "manifest", "", "Plugin manifest JSON path, or - for stdin")
	add.Flags().BoolVar(&yes, "yes", false, "Trust the executable, run describe and persist registration")
	add.Flags().BoolVar(&dry, "dry-run", false, "Validate without executing the plugin or writing state")
	root.AddCommand(add)
	root.AddCommand(&cobra.Command{Use: "remove NAME", Short: "Unregister a plugin; active tasks and outstanding receipts prevent removal", Example: "  hell plugin remove desktop --json", Args: args(cobra.ExactArgs(1)), RunE: func(_ *cobra.Command, values []string) error {
		return a.engine().Store.Update(func(st *state.State) error {
			if _, exists := st.Plugins[values[0]]; !exists {
				return fault.New("not_found", 4, "plugin is not registered")
			}
			for _, t := range st.Tasks {
				if t.Policy.Channel != values[0] {
					continue
				}
				if t.Armed {
					return fault.New("conflict", 5, "disarm tasks using this plugin first")
				}
				if t.Incident != nil {
					for _, attempt := range t.Incident.Attempts {
						if attempt.Receipt != "" && !attempt.Cancelled {
							return fault.New("conflict", 5, "plugin still has outstanding notification receipts")
						}
					}
				}
			}
			delete(st.Plugins, values[0])
			a.data = map[string]any{"removed": values[0], "executable_deleted": false}
			return nil
		})
	}})
	return root
}
