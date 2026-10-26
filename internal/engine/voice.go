package engine

// incidentMessage uses only the redacted event category and durable send-attempt
// number. A retry says nothing about delivery, reading, or the human's activity.
func incidentMessage(reason string, attempt int) string {
	var message string
	switch reason {
	case "verification_unmet":
		message = "验收没过，先别急着散会。"
	case "verification_unknown":
		message = "验收还没给出结论。庆功宴先别订。"
	case "verification_lost":
		message = "验收结果失联了。连成绩单都能鸽。"
	case "waiting_for_input":
		message = "流程跑到你这里，开始阻塞了。"
	case "heartbeat_lost":
		message = "Agent 失联了。它没留辞职信。"
	case "deadline_exceeded":
		message = "截止时间到了，完成证据没到。"
	case "run_failed":
		message = "Agent 异常退场。谢幕倒是挺早。"
	default:
		message = "有事需要你处理。别光围观。"
	}
	if attempt > 1 {
		asides := [...]string{"又是我。", "这条通知有续集。", "还没散场。"}
		message = asides[(attempt-2)%len(asides)] + message
	}
	return message
}
