package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dszdsxc19/hell-mode/internal/fault"
	"github.com/dszdsxc19/hell-mode/internal/model"
	"github.com/spf13/cobra"
)

type engagementInput struct {
	Focus   string   `json:"focus"`
	Every   string   `json:"every"`
	At      string   `json:"at"`
	Agent   []string `json:"agent"`
	Workdir string   `json:"workdir"`
	Timeout string   `json:"timeout"`
	Channel string   `json:"channel"`
}

func (a *app) engageCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "engage",
		Short: "Let AI invite the human to answer a question or try a small task",
		Long: `Schedule personal conversations using saved memory and previous replies.
The independent hell watch process calls your explicitly chosen AI command.
Each invitation contains one question or small task and the reason for asking.
Unanswered invitations wait; they do not generate repeated reminders.

Memory is shared by engagements in this state directory. The selected AI
receives that memory and recent conversation history. Generated tasks are for
the human to consider, never commands for hell to execute.
vanish/end pause engagements; back leaves them paused until explicit resume.`,
		Example: "  hell engage memory --file memory.md\n  hell engage create --file engagement.json --dry-run --json\n  hell engage list\n  hell engage show ENGAGEMENT_ID",
		Args:    args(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(a.engageMemoryCommand(), a.engageCreateCommand(), a.engageListCommand(), a.engageShowCommand(), a.engageReplyCommand(false), a.engageReplyCommand(true), a.engageActiveCommand(false), a.engageActiveCommand(true))
	return cmd
}

func (a *app) engageMemoryCommand() *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "memory",
		Short: "Read personal memory, or replace it from a text file",
		Long: `Read the current memory without creating state. --file FILE replaces it
with UTF-8 text; - reads stdin. An empty file clears saved memory. Include
confirmed interests, ongoing work and relevant preferences; label guesses as
uncertain. History is retained separately. Only save information the human
has authorized for the chosen AI command.`,
		Example: "  hell engage memory\n  hell engage memory --file memory.md --json\n  hell engage memory --file -",
		Args:    args(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := a.engine()
			var memory model.PersonalMemory
			var err error
			if cmd.Flags().Changed("file") {
				var text string
				text, err = a.readEngagementText(file)
				if err != nil {
					return err
				}
				memory, err = e.SetMemory(text)
			} else {
				memory, err = e.Memory()
			}
			a.data = memory
			a.human = memory.Text
			if memory.Text == "" {
				a.human = "还没有保存个人记忆。"
			}
			return err
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "Replace personal memory with text from FILE, or - for stdin (empty file clears)")
	return cmd
}

func (a *app) engageCreateCommand() *cobra.Command {
	input := engagementInput{}
	var file, agentJSON string
	var yes, dry bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Preview or create a recurring personal conversation",
		Long: `Use --file for a strict JSON contract, or individual flags. --agent-json
is a trusted executable argv, with no implicit shell. The AI receives a JSON
request on stdin and returns {"kind":"question|task|skip","message":"...","basis":"..."}.
The default channel log simulates delivery; show reads the local invitation.
--dry-run validates without invoking AI or writing state. --yes saves the plan.
No background process or service is installed; run hell watch separately.`,
		Example: `  hell engage create --file engagement.json --dry-run --json
  hell engage create --focus '正在验证的产品想法' --every 4h --agent-json '["my-ai-adapter"]' --yes
  hell engage create --file - --yes --json`,
		Args: args(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("file") && file == "" {
				return fault.New("invalid_argument", 2, "file path must not be empty; use - for stdin")
			}
			if file != "" {
				for _, name := range []string{"focus", "every", "at", "agent-json", "workdir", "timeout", "channel"} {
					if cmd.Flags().Changed(name) {
						return fault.New("invalid_argument", 2, "--file cannot be combined with --%s", name)
					}
				}
				if err := a.decodeInput(file, &input); err != nil {
					return err
				}
			} else {
				var value any
				if err := json.Unmarshal([]byte(agentJSON), &value); err != nil || hasNull(value) {
					return fault.New("invalid_argument", 2, "--agent-json must be a JSON argv array without null")
				}
				if err := json.Unmarshal([]byte(agentJSON), &input.Agent); err != nil {
					return fault.New("invalid_argument", 2, "--agent-json must be a JSON argv array")
				}
			}
			every, err := time.ParseDuration(input.Every)
			if err != nil {
				return fault.New("invalid_argument", 2, "every must be a Go duration, e.g. 4h")
			}
			timeout, err := time.ParseDuration(input.Timeout)
			if err != nil {
				return fault.New("invalid_argument", 2, "timeout must be a Go duration, e.g. 2m")
			}
			nextAt := time.Now().UTC().Add(every)
			if input.At != "" {
				nextAt, err = parseTime(input.At)
				if err != nil {
					return err
				}
			}
			plan := model.Engagement{Focus: input.Focus, Every: every, NextAt: nextAt, Agent: input.Agent, Workdir: input.Workdir, Timeout: timeout, Channel: input.Channel}
			e := a.engine()
			if err := e.ValidateEngagement(&plan); err != nil {
				return err
			}
			if dry {
				a.data = map[string]any{"dry_run": true, "contract": plan}
				return nil
			}
			if !yes {
				return fault.New("confirmation_required", 2, "inspect with --dry-run, then pass --yes to confirm this engagement")
			}
			created, err := e.CreateEngagement(plan)
			a.data = created
			if err == nil {
				a.human = fmt.Sprintf("主动邀谈已保存：%s\n启用：%t\n下次时间：%s\n使用 hell watch 执行调度。", created.ID, created.Active, created.NextAt.Local().Format(time.RFC3339))
			}
			return err
		},
	}
	f := cmd.Flags()
	f.StringVar(&file, "file", "", "Read JSON configuration from FILE, or - for stdin (cannot mix configuration flags)")
	f.StringVar(&input.Focus, "focus", "", "Optional focus; otherwise follow current interests from memory and replies")
	f.StringVar(&input.Every, "every", "4h", "Interval between conversations; one unresolved invitation at a time")
	f.StringVar(&input.At, "at", "", "First invitation: RFC3339 or +duration (default now + every; +0s starts now)")
	f.StringVar(&agentJSON, "agent-json", "", "Explicit AI command argv as JSON; reads one JSON request on stdin")
	f.StringVar(&input.Workdir, "workdir", ".", "Existing working directory for the AI command")
	f.StringVar(&input.Timeout, "timeout", "2m", "AI command timeout (positive, at most 5m)")
	f.StringVar(&input.Channel, "channel", "log", "log simulation, normal-priority pushover, or a registered notification plugin")
	f.BoolVar(&yes, "yes", false, "Confirm the supplied AI command, memory use and conversation schedule")
	f.BoolVar(&dry, "dry-run", false, "Validate and print the plan without invoking AI or writing state")
	return cmd
}

func (a *app) engageListCommand() *cobra.Command {
	return &cobra.Command{Use: "list", Short: "List personal conversation schedules", Args: args(cobra.NoArgs), RunE: func(_ *cobra.Command, _ []string) error {
		plans, err := a.engine().ListEngagements()
		a.data = plans
		var out strings.Builder
		for _, p := range plans {
			fmt.Fprintf(&out, "%s  启用=%t  下次=%s  %s\n", p.ID, p.Active, p.NextAt.Local().Format(time.RFC3339), p.Focus)
		}
		a.human = strings.TrimSpace(out.String())
		if a.human == "" {
			a.human = "还没有主动邀谈计划。"
		}
		return err
	}}
}

func (a *app) engageShowCommand() *cobra.Command {
	return &cobra.Command{Use: "show ENGAGEMENT_ID", Short: "Read invitations, reasons, human answers and delivery status", Args: args(cobra.ExactArgs(1)), RunE: func(_ *cobra.Command, values []string) error {
		p, err := a.engine().GetEngagement(values[0])
		a.data = p
		if err != nil {
			return err
		}
		var out strings.Builder
		fmt.Fprintf(&out, "%s\n启用：%t\n下次时间：%s\n", p.ID, p.Active, p.NextAt.Local().Format(time.RFC3339))
		if p.Focus != "" {
			fmt.Fprintf(&out, "关注：%s\n", p.Focus)
		}
		if p.LastError != "" {
			fmt.Fprintf(&out, "最近错误：%s\n", p.LastError)
		}
		for _, round := range p.Rounds {
			fmt.Fprintf(&out, "\n%s [%s] %s\n", round.ID, round.State, round.Kind)
			if round.Error != "" {
				fmt.Fprintf(&out, "生成详情：%s\n", round.Error)
			}
			if round.Message != "" {
				fmt.Fprintln(&out, round.Message)
			}
			if round.Basis != "" {
				fmt.Fprintf(&out, "缘由：%s\n", round.Basis)
			}
			if round.Answer != "" {
				fmt.Fprintf(&out, "你的回答：%s\n", round.Answer)
			}
			if round.Delivery != nil {
				fmt.Fprintf(&out, "通知：%s\n", round.Delivery.State)
				if round.Delivery.Error != "" {
					fmt.Fprintf(&out, "通知详情：%s\n", round.Delivery.Error)
				}
			}
		}
		a.human = strings.TrimSpace(out.String())
		return nil
	}}
}

func (a *app) engageReplyCommand(skip bool) *cobra.Command {
	var roundID, answer, file string
	name, description := "reply", "Record the human's answer for a specific invitation"
	if skip {
		name, description = "skip", "Skip a specific invitation without inventing an answer"
	}
	cmd := &cobra.Command{Use: name + " ENGAGEMENT_ID", Short: description, Args: args(cobra.ExactArgs(1)), RunE: func(cmd *cobra.Command, values []string) error {
		if strings.TrimSpace(roundID) == "" {
			return fault.New("invalid_argument", 2, "--round is required; read hell engage show first")
		}
		if !skip {
			if cmd.Flags().Changed("file") == cmd.Flags().Changed("text") {
				return fault.New("invalid_argument", 2, "use exactly one of --text or --file for the human's answer")
			}
			if cmd.Flags().Changed("file") {
				var err error
				answer, err = a.readEngagementText(file)
				if err != nil {
					return err
				}
			}
			if strings.TrimSpace(answer) == "" {
				return fault.New("invalid_argument", 2, "answer must not be blank; use skip to decline this invitation")
			}
		}
		p, err := a.engine().ReplyEngagement(values[0], roundID, answer, skip)
		a.data = p
		if err == nil {
			a.human = "回答已保存，下次邀谈会参考这段对话。"
			if skip {
				a.human = "这次邀谈已跳过。"
			}
		}
		return err
	}}
	cmd.Flags().StringVar(&roundID, "round", "", "Exact invitation ID from show; prevents replying to a newer invitation by mistake")
	if !skip {
		cmd.Flags().StringVar(&answer, "text", "", "The human's answer, preserved verbatim")
		cmd.Flags().StringVar(&file, "file", "", "Read the human's answer from FILE, or - for stdin")
	}
	return cmd
}

func (a *app) engageActiveCommand(active bool) *cobra.Command {
	name, description := "pause", "Pause a conversation schedule and keep its history"
	if active {
		name, description = "resume", "Resume by human choice, scheduling the next invitation from now"
	}
	return &cobra.Command{Use: name + " ENGAGEMENT_ID", Short: description, Long: description + ".\nHuman instruction required. Resume requires available presence and never replays overdue invitations.", Args: args(cobra.ExactArgs(1)), RunE: func(_ *cobra.Command, values []string) error {
		p, err := a.engine().SetEngagementActive(values[0], active)
		a.data = p
		if err == nil {
			a.human = "主动邀谈已暂停。"
			if active {
				a.human = "主动邀谈已恢复，下次时间：" + p.NextAt.Local().Format(time.RFC3339)
			}
		}
		return err
	}}
}

// readEngagementText bounds raw memory/reply text without treating it as a JSON contract.
func (a *app) readEngagementText(path string) (string, error) {
	if path == "" {
		return "", fault.New("invalid_argument", 2, "file path must not be empty; use - for stdin")
	}
	reader := a.in
	if path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer f.Close()
		reader = f
	}
	raw, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
	if err != nil {
		return "", err
	}
	if len(raw) > 1<<20 {
		return "", fault.New("invalid_argument", 2, "text exceeds 1 MiB")
	}
	if !utf8.Valid(raw) {
		return "", fault.New("invalid_argument", 2, "text must be valid UTF-8")
	}
	return string(raw), nil
}
