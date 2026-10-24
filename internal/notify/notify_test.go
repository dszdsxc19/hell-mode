package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dszdsxc19/hell-mode/pkg/protocol"
)

func TestPushoverEmergencyAndCancellation(t *testing.T) {
	t.Setenv("HELL_PUSHOVER_APP_TOKEN", "test-app")
	t.Setenv("HELL_PUSHOVER_USER_KEY", "test-user")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" {
			t.Error(r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("token") != "test-app" {
			t.Error("missing token")
		}
		if r.URL.Path == "/1/messages.json" {
			if r.Form.Get("priority") != "2" || r.Form.Get("retry") != "30" || r.Form.Get("user") != "test-user" {
				t.Error(r.Form)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": 1, "receipt": "test-receipt"})
		} else if r.URL.Path == "/1/receipts/test-receipt/cancel.json" {
			_ = json.NewEncoder(w).Encode(map[string]int{"status": 1})
		} else {
			t.Error(r.URL.Path)
		}
	}))
	defer server.Close()
	d := Dispatcher{Endpoint: server.URL, Client: server.Client()}
	result, err := d.Call(context.Background(), "pushover", "notify", protocol.Notification{ExpiresAt: time.Now().Add(time.Minute)})
	if err != nil || result.Receipt != "test-receipt" {
		t.Fatalf("%+v %v", result, err)
	}
	result, err = d.Call(context.Background(), "pushover", "cancel", protocol.Notification{Receipt: result.Receipt})
	if err != nil || result.State != "cancelled" || calls != 2 {
		t.Fatalf("%+v %v calls=%d", result, err, calls)
	}
}

func TestPushoverRejectsHTTPAndInvalidReceipt(t *testing.T) {
	t.Setenv("HELL_PUSHOVER_APP_TOKEN", "test-app")
	t.Setenv("HELL_PUSHOVER_USER_KEY", "test-user")
	for _, status := range []int{http.StatusTooManyRequests, http.StatusOK} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"status":1}`))
			}))
			defer server.Close()
			d := Dispatcher{Endpoint: server.URL, Client: server.Client()}
			if _, err := d.Call(context.Background(), "pushover", "notify", protocol.Notification{ExpiresAt: time.Now().Add(time.Minute)}); err == nil {
				t.Fatal("ambiguous notification accepted")
			}
		})
	}
}

func TestPushoverPersonalInvitationUsesNormalPriorityWithoutRetries(t *testing.T) {
	t.Setenv("HELL_PUSHOVER_APP_TOKEN", "test-app")
	t.Setenv("HELL_PUSHOVER_USER_KEY", "test-user")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("priority") != "0" || r.Form.Has("retry") || r.Form.Has("expire") {
			t.Errorf("personal invitation must not create an emergency: %v", r.Form)
		}
		if r.Form.Get("message") != "a new invitation is ready" {
			t.Error("missing invitation notice")
		}
		_, _ = w.Write([]byte(`{"status":1}`))
	}))
	defer server.Close()
	d := Dispatcher{Endpoint: server.URL, Client: server.Client()}
	result, err := d.Call(context.Background(), "pushover", "notify", protocol.Notification{Priority: "normal", Body: "a new invitation is ready", ExpiresAt: time.Now().Add(time.Minute)})
	if err != nil || result.State != "sent" || result.Receipt != "" || calls != 1 {
		t.Fatalf("result=%+v err=%v calls=%d", result, err, calls)
	}
}
