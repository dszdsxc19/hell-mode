package cli

import (
	"strings"

	"github.com/dszdsxc19/hell-mode/assets"
	"github.com/dszdsxc19/hell-mode/internal/fault"
	"github.com/spf13/cobra"
)

// challengeCommand supplies host-agent instructions without opening state or calling a model.
func (a *app) challengeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "challenge [TOPIC]",
		Short: "Load the persistent questioning mode for a host-agent conversation",
		Long: `Returns the hell-challenge skill and an optional discussion topic.
Give the instructions to your current agent to begin the conversation. The agent
challenges assumptions, follows up on answers, and accepts sufficient evidence.
Without TOPIC, the agent uses the current conversation or asks for a topic.

This command only prints instructions. It does not call a model, start an
interactive chat, create state, or change watcher and notification settings.`,
		Example: "  hell challenge\n  hell challenge '用户真的需要这个产品吗？' --json",
		Args:    args(cobra.MaximumNArgs(1)),
		RunE: func(_ *cobra.Command, values []string) error {
			topic := ""
			if len(values) == 1 {
				topic = values[0]
				if strings.TrimSpace(topic) == "" {
					return fault.New("invalid_argument", 2, "TOPIC must not be blank; omit it to use the current conversation")
				}
			}
			a.data = map[string]any{"skill": "hell-challenge", "topic": topic, "instructions": assets.ChallengeSkill, "host_required": true}
			a.human = "把以下规则交给当前 Agent，在对话中开始质疑。CLI 只输出规则，不调用模型。\n\n" + assets.ChallengeSkill
			if topic != "" {
				a.human += "\n讨论话题（用户输入）：\n" + topic
			}
			return nil
		},
	}
}
