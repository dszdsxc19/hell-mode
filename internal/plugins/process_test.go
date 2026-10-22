package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dszdsxc19/hell-mode/pkg/protocol"
)

// TestPluginProcess is launched in a separate process, exercising the actual stdio boundary.
func TestPluginProcess(t *testing.T) {
	if os.Getenv("HELL_PLUGIN_TEST") != "1" {
		return
	}
	var request protocol.Request
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		os.Exit(2)
	}
	mode := os.Args[len(os.Args)-1]
	if mode == "timeout" {
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	if mode == "huge" {
		fmt.Print(strings.Repeat("x", (1<<20)+1))
		os.Exit(0)
	}
	response := protocol.Response{ProtocolVersion: protocol.Version, ID: request.ID, Result: &protocol.Result{Name: "test", Capabilities: []string{"notify", "cancel"}, Receipt: "receipt", State: "queued"}}
	if request.Method == "cancel" {
		response.Result.State = "cancelled"
	}
	if mode == "mismatch" {
		response.ID = "wrong"
	}
	if mode == "env" && os.Getenv("HELL_SECRET_SENTINEL") != "" {
		os.Exit(7)
	}
	_ = json.NewEncoder(os.Stdout).Encode(response)
	if mode == "extra" {
		fmt.Print("{}")
	}
	os.Exit(0)
}

func TestPluginBoundedProtocolAndEnvironment(t *testing.T) {
	t.Setenv("HELL_PLUGIN_TEST", "1")
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	t.Setenv("HELL_SECRET_SENTINEL", "must-not-leak")
	for _, mode := range []string{"valid", "env", "mismatch", "extra", "huge", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			timeout := 2000
			if mode == "timeout" {
				timeout = 50
			}
			manifest, err := Validate(protocol.Manifest{ProtocolVersion: protocol.Version, Name: "test", Command: []string{os.Args[0], "-test.run=^TestPluginProcess$", "--", mode}, TimeoutMS: timeout, Env: []string{"HELL_PLUGIN_TEST", "GORACE"}})
			if err != nil {
				t.Fatal(err)
			}
			result, err := Call(context.Background(), manifest, "describe", nil)
			if mode == "valid" || mode == "env" {
				if err != nil || result.Name != "test" {
					t.Fatalf("result=%+v err=%v", result, err)
				}
			} else if err == nil {
				t.Fatal("invalid plugin response accepted")
			}
		})
	}
}
