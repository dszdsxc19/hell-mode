package cli

import (
	"fmt"
	"time"

	"github.com/dszdsxc19/hell-mode/internal/fault"
	"github.com/dszdsxc19/hell-mode/internal/model"
	"github.com/spf13/cobra"
)

func (a *app) presenceCommand() *cobra.Command {
	return &cobra.Command{Use: "presence", Short: "Read whether the human is available, away, or done for today", Long: `Read the human boundary for this state directory without creating state.
An elapsed away timer still means away. No command infers that the human has returned.
Quiet tasks remain quiet after back until an explicitly requested run --rearm.`, Example: "  hell presence --json", Args: args(cobra.NoArgs), RunE: func(_ *cobra.Command, _ []string) error {
		p, err := a.engine().Presence()
		if err != nil {
			return err
		}
		a.data = p
		switch p.Mode {
		case "away":
			if !time.Now().Before(p.Until) {
				a.human = "约定时间已到。\n人类尚未返回。"
			} else {
				a.human = "人类暂不提供服务。\n约定结束时间：" + p.Until.Local().Format(time.RFC3339) + "\n是否回来，由你决定。"
			}
		case "ended":
			a.human = "今天已结束。\n看守已停止。"
		default:
			a.human = "你在这里。"
		}
		return nil
	}}
}

func (a *app) vanishCommand() *cobra.Command {
	var duration time.Duration
	cmd := &cobra.Command{Use: "vanish", Short: "番茄失踪：ask this system to leave the human alone", Long: `Human instruction required. Make all tasks in this state directory quiet.
Running work and verification may continue; known notification receipts are cancelled.
The timer is an agreed quiet period, not a return-to-work deadline: after it
elapses, the system remains quiet until the human explicitly uses back.
No reason, score, timer alarm, or automatic return is recorded.`, Example: "  hell vanish --for 25m\n  hell vanish --for 25m --json", Args: args(cobra.NoArgs), RunE: func(cmd *cobra.Command, _ []string) error {
		if duration <= 0 {
			return fault.New("invalid_argument", 2, "--for must be a positive Go duration, e.g. 25m")
		}
		p, err := a.engine().SetPresence(cmd.Context(), "away", time.Now().UTC().Add(duration))
		a.data = p
		if err != nil {
			return err
		}
		a.human = "番茄失踪已开启。\n行，先消失。需要你的事情我攒着。"
		return a.cancellationNotice(p)
	}}
	cmd.Flags().DurationVar(&duration, "for", 25*time.Minute, "Agreed quiet period; expiry never resumes notifications")
	return cmd
}

func (a *app) backCommand() *cobra.Command {
	return &cobra.Command{Use: "back", Short: "Return by human choice; keep old tasks quiet", Long: `Human instruction required, even if an away timer has expired.
Restore availability without replaying alerts or unmuting existing tasks.
Inspect saved work with status. To monitor an old task again, the human must
explicitly request run --rearm. Ended watchdog processes do not restart.`, Example: "  hell back\n  hell back --json", Args: args(cobra.NoArgs), RunE: func(cmd *cobra.Command, _ []string) error {
		p, err := a.engine().SetPresence(cmd.Context(), "available", time.Time{})
		a.data = p
		if err != nil {
			return err
		}
		a.human = "回来了？事情也还在。\n旧提醒不补发，要继续盯哪件事你再说。"
		return nil
	}}
}

func (a *app) endCommand() *cobra.Command {
	return &cobra.Command{Use: "end", Short: "今天到此为止：stop watching without declaring success", Long: `Human instruction required. Make this state directory quiet and stop its
watchdog loops at their next state check. Keep tasks, logs and real verdicts.
This does not kill agent processes or mark their goals met. No automatic
next-day restart occurs. back restores availability; start watch explicitly
when monitoring again. External requests already in flight may finish and
their receipts are cancelled when available.`, Example: "  hell end\n  hell end --json", Args: args(cobra.NoArgs), RunE: func(cmd *cobra.Command, _ []string) error {
		p, err := a.engine().SetPresence(cmd.Context(), "ended", time.Time{})
		a.data = p
		if err != nil {
			return err
		}
		tasks, err := a.engine().List()
		if err != nil {
			return err
		}
		first := "任务记录已保留。"
		parting := "行，今天先散会。"
		for _, task := range tasks {
			if task.Verdict != "met" {
				first = "任务未完成。"
				parting = "气势倒是下班了。"
				break
			}
		}
		a.human = first + "\n\n" + parting + "\n\n看守已停止。"
		return a.cancellationNotice(p)
	}}
}

// A local stop cannot promise that a provider has already cancelled an in-flight request.
func (a *app) cancellationNotice(p model.Presence) error {
	tasks, err := a.engine().List()
	if err != nil {
		return err
	}
	pending := 0
	for _, task := range tasks {
		if task.Incident == nil {
			continue
		}
		for _, attempt := range task.Incident.Attempts {
			if (attempt.State == "sending" && time.Now().Before(attempt.LeaseUntil)) || (attempt.Receipt != "" && !attempt.Cancelled && time.Now().Before(task.Incident.ExpiresAt)) {
				pending++
			}
		}
	}
	if pending > 0 {
		a.human += fmt.Sprintf("\n\n仍有 %d 条在途通知或取消结果待确认；可用 watch --once 重试取消。", pending)
	}
	a.data = struct {
		model.Presence
		PendingCancellations int `json:"pending_cancellations"`
	}{p, pending}
	return nil
}
