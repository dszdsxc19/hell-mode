// Package notify dispatches the same notification contract to built-ins or process plugins.
package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/dszdsxc19/hell-mode/internal/plugins"
	"github.com/dszdsxc19/hell-mode/internal/state"
	"github.com/dszdsxc19/hell-mode/pkg/protocol"
)

type Dispatcher struct {
	Store    state.Store
	Client   *http.Client
	Endpoint string // Tests can substitute a local HTTP server; the CLI always uses the official endpoint.
}

// Call keeps provider receipts separate from goal and takeover state.
func (d Dispatcher) Call(ctx context.Context, channel, method string, n protocol.Notification) (*protocol.Result, error) {
	if channel == "log" {
		if method == "cancel" {
			return &protocol.Result{State: "cancelled", Receipt: n.Receipt}, nil
		}
		return &protocol.Result{State: "simulated", Receipt: "log:" + n.AttemptID}, nil
	}
	if channel == "pushover" {
		return d.pushover(ctx, method, n)
	}
	st, err := d.Store.Snapshot()
	if err != nil {
		return nil, err
	}
	m, ok := st.Plugins[channel]
	if !ok {
		return nil, fmt.Errorf("notification plugin %q is not registered", channel)
	}
	return plugins.Call(ctx, m, method, &n)
}

// pushover uses environment-only credentials and bounded emergency-priority retries.
func (d Dispatcher) pushover(ctx context.Context, method string, n protocol.Notification) (*protocol.Result, error) {
	token, user := os.Getenv("HELL_PUSHOVER_APP_TOKEN"), os.Getenv("HELL_PUSHOVER_USER_KEY")
	if token == "" || user == "" {
		return nil, fmt.Errorf("set HELL_PUSHOVER_APP_TOKEN and HELL_PUSHOVER_USER_KEY")
	}
	base := d.Endpoint
	if base == "" {
		base = "https://api.pushover.net"
	}
	values := url.Values{"token": {token}}
	endpoint := base + "/1/messages.json"
	if method == "cancel" {
		endpoint = base + "/1/receipts/" + url.PathEscape(n.Receipt) + "/cancel.json"
	} else {
		expire := int(time.Until(n.ExpiresAt).Seconds())
		if expire < 1 {
			return nil, fmt.Errorf("notification window expired")
		}
		if expire > 10800 {
			expire = 10800
		}
		values.Set("user", user)
		values.Set("title", n.Title)
		values.Set("message", n.Body)
		values.Set("priority", "2")
		values.Set("retry", "30")
		values.Set("expire", fmt.Sprint(expire))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := d.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pushover transport failed (delivery may be unknown)")
	}
	defer response.Body.Close()
	var body struct {
		Status  int    `json:"status"`
		Receipt string `json:"receipt"`
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pushover returned HTTP %d", response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&body); err != nil || body.Status != 1 {
		return nil, fmt.Errorf("pushover returned an invalid or rejected response")
	}
	if method == "cancel" {
		return &protocol.Result{State: "cancelled", Receipt: n.Receipt}, nil
	}
	if body.Receipt == "" {
		return nil, fmt.Errorf("pushover accepted response has no receipt; delivery is unknown")
	}
	return &protocol.Result{State: "queued", Receipt: body.Receipt}, nil
}
