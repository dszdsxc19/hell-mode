// hell-plugin-demo demonstrates the complete subprocess protocol without sending notifications.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/dszdsxc19/hell-mode/pkg/protocol"
)

func main() {
	var request protocol.Request
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 1<<20)).Decode(&request); err != nil {
		fmt.Fprintln(os.Stderr, "invalid request")
		os.Exit(1)
	}
	response := protocol.Response{ProtocolVersion: protocol.Version, ID: request.ID}
	if request.ProtocolVersion != protocol.Version {
		response.Error = &protocol.Error{Code: "unsupported_protocol", Message: "expected hell-mode/v1"}
	} else {
		switch request.Method {
		case "describe":
			response.Result = &protocol.Result{Name: "demo", Capabilities: []string{"notify", "cancel"}}
		case "notify":
			if request.Notification == nil {
				response.Error = &protocol.Error{Code: "invalid_request", Message: "notification required"}
			} else {
				response.Result = &protocol.Result{State: "simulated", Receipt: "demo:" + request.Notification.AttemptID}
			}
		case "cancel":
			response.Result = &protocol.Result{State: "cancelled"}
		default:
			response.Error = &protocol.Error{Code: "unknown_method", Message: "unsupported method"}
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
		os.Exit(1)
	}
}
