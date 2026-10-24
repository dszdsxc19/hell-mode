package cli

// inputSchemas documents the strict JSON inputs alongside introspected command flags.
func inputSchemas() map[string]any {
	str := map[string]any{"type": "string", "minLength": 1}
	argv := map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "string"}}
	object := func(required []string, properties map[string]any) map[string]any {
		return map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "additionalProperties": false, "required": required, "properties": properties}
	}
	return map[string]any{
		"engage_agent_response": map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "required": []string{"kind", "basis"}, "properties": map[string]any{"kind": map[string]any{"type": "string", "enum": []string{"question", "task", "skip"}}, "message": map[string]any{"type": "string", "description": "Required and nonblank for question/task; optional for skip"}, "basis": str}, "description": "One JSON object on agent stdout; at most 32 KiB. question/task require nonblank message and basis."},
		"engage_create":         object([]string{"agent"}, map[string]any{"focus": map[string]any{"type": "string"}, "every": map[string]any{"type": "string", "default": "4h"}, "at": map[string]any{"type": "string", "description": "First invitation: RFC3339 or +Go-duration; default now + every"}, "agent": argv, "workdir": map[string]any{"type": "string", "default": "."}, "timeout": map[string]any{"type": "string", "default": "2m", "description": "Positive Go duration, at most 5m"}, "channel": map[string]any{"type": "string", "default": "log"}}),
		"arm":                   object([]string{"goal", "verify", "deadline"}, map[string]any{"goal": str, "verify": argv, "deadline": map[string]any{"type": "string", "description": "RFC3339 with timezone or +Go-duration"}, "workdir": str, "heartbeat_timeout": str, "verify_timeout": str, "channel": str, "interval": str, "window": str, "max_attempts": map[string]any{"type": "integer", "minimum": 1}}),
		"event":                 object([]string{"id", "task_id", "run_id", "type", "source"}, map[string]any{"id": str, "task_id": str, "run_id": str, "source": str, "type": map[string]any{"type": "string", "enum": []string{"heartbeat", "exited", "failed", "waiting", "turn_completed"}}, "exit_code": map[string]any{"type": "integer"}, "background_work": map[string]any{"type": "boolean"}}),
		"plugin_manifest":       object([]string{"protocol_version", "name", "command"}, map[string]any{"protocol_version": map[string]any{"const": "hell-mode/v1"}, "name": map[string]any{"type": "string", "pattern": "^[a-z][a-z0-9-]{0,63}$", "description": "log and pushover are reserved"}, "command": argv, "timeout_ms": map[string]any{"type": "integer", "minimum": 0, "maximum": 30000, "default": 10000, "description": "0 selects the default timeout"}, "env": map[string]any{"type": "array", "items": str}}),
	}
}
