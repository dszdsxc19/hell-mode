// Package plugins loads explicitly registered subprocess notification adapters.
package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"time"

	"github.com/dszdsxc19/hell-mode/internal/model"
	"github.com/dszdsxc19/hell-mode/internal/process"
	"github.com/dszdsxc19/hell-mode/pkg/protocol"
)

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// Validate resolves executables at registration; a manifest cannot become an implicit shell command.
func Validate(m protocol.Manifest) (protocol.Manifest, error) {
	if m.ProtocolVersion != protocol.Version {
		return m, fmt.Errorf("plugin protocol must be %s", protocol.Version)
	}
	if !namePattern.MatchString(m.Name) || m.Name == "log" || m.Name == "pushover" {
		return m, fmt.Errorf("invalid or reserved plugin name")
	}
	if len(m.Command) == 0 || m.Command[0] == "" {
		return m, fmt.Errorf("plugin command is required")
	}
	path, err := exec.LookPath(m.Command[0])
	if err != nil {
		return m, fmt.Errorf("resolve plugin executable: %w", err)
	}
	m.Command[0], err = filepath.Abs(path)
	if err != nil {
		return m, err
	}
	if m.TimeoutMS == 0 {
		m.TimeoutMS = 10000
	}
	if m.TimeoutMS < 1 || m.TimeoutMS > 30000 {
		return m, fmt.Errorf("plugin timeout_ms must be 1..30000")
	}
	for _, key := range m.Env {
		if key == "" || regexp.MustCompile(`[^A-Za-z0-9_]`).MatchString(key) {
			return m, fmt.Errorf("invalid environment variable name")
		}
	}
	return m, nil
}

// Call exchanges one bounded JSON request; plugin diagnostics never pollute CLI stdout.
func Call(ctx context.Context, m protocol.Manifest, method string, n *protocol.Notification) (*protocol.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(m.TimeoutMS)*time.Millisecond)
	defer cancel()
	req := protocol.Request{ProtocolVersion: protocol.Version, ID: model.ID("req"), Method: method, Notification: n}
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	cmd := process.Command(ctx, m.Command)
	cmd.Stdin = bytes.NewReader(raw)
	cmd.Env = []string{}
	for _, key := range slices.Concat([]string{"PATH", "SystemRoot"}, m.Env) {
		if value, ok := os.LookupEnv(key); ok {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	stdout := &process.Buffer{Limit: 1 << 20}
	stderr := &process.Buffer{Limit: 64 << 10}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("plugin %s execution failed: %w", m.Name, err)
	}
	if stdout.Truncated {
		return nil, fmt.Errorf("plugin response exceeds 1 MiB")
	}
	var response protocol.Response
	decoder := json.NewDecoder(&stdout.Buffer)
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("invalid plugin response: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("plugin stdout must contain exactly one JSON object")
	}
	if response.ID != req.ID || response.ProtocolVersion != protocol.Version {
		return nil, fmt.Errorf("plugin response identity/version mismatch")
	}
	if (response.Result == nil) == (response.Error == nil) {
		return nil, fmt.Errorf("plugin response needs exactly one result or error")
	}
	if response.Error != nil {
		return nil, fmt.Errorf("plugin %s reported %s", m.Name, response.Error.Code)
	}
	if method == "describe" && (response.Result.Name != m.Name || !slices.Contains(response.Result.Capabilities, "notify") || !slices.Contains(response.Result.Capabilities, "cancel")) {
		return nil, fmt.Errorf("plugin must describe matching name with notify and cancel capabilities")
	}
	normalSent := n != nil && n.Priority == "normal" && response.Result.State == "sent"
	if method == "notify" && !normalSent && (response.Result.Receipt == "" || !slices.Contains([]string{"queued", "simulated"}, response.Result.State)) {
		return nil, fmt.Errorf("plugin notify needs a receipt and queued or simulated state")
	}
	if method == "cancel" && response.Result.State != "cancelled" {
		return nil, fmt.Errorf("plugin did not confirm cancellation")
	}
	return response.Result, nil
}
