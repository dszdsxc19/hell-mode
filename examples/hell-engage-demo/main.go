// hell-engage-demo demonstrates the engagement adapter protocol without a model or network call.
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	var request struct {
		ProtocolVersion string `json:"protocol_version"`
		Focus           string `json:"focus"`
		Memory          struct {
			Text string `json:"text"`
		} `json:"memory"`
		History []struct {
			Answer string `json:"answer"`
		} `json:"history"`
	}
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		fmt.Fprintln(os.Stderr, "expected an engagement JSON request:", err)
		os.Exit(1)
	}
	if request.ProtocolVersion != "hell-mode/engage-v1" {
		fmt.Fprintln(os.Stderr, "unsupported engagement protocol")
		os.Exit(1)
	}
	result := map[string]string{"kind": "question", "message": "关于你最近关注的事情，哪一个判断最值得用一次小实验验证？", "basis": "协议演示：读取了已保存的个人记忆；正式使用时由你的 AI 生成针对性问题。"}
	if request.Memory.Text == "" && request.Focus == "" && len(request.History) == 0 {
		result = map[string]string{"kind": "skip", "basis": "还没有个人记忆、关注方向或历史回答。"}
	}
	for i := len(request.History) - 1; i >= 0; i-- {
		if request.History[i].Answer != "" {
			result = map[string]string{"kind": "task", "message": "用五分钟为上次回答里的判断，写下一项可观察的验证结果。", "basis": "协议演示：本轮接收到了上一轮的真实回答。"}
			break
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
