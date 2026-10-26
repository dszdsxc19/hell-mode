# 通知插件

> 仅供娱乐。插件只在本人授权的设备和策略下运行。

v1 扩展点是通知通道：核心负责目标、验收、状态、次数与调度，插件提供 describe、notify、cancel 三个方法。插件可使用任意语言，通过 stdin/stdout 的 JSON 与 Go 核心交互。

## 注册与发现

插件必须显式注册。`plugin list` 和 `schema` 不执行插件；`plugin add --dry-run` 只校验 manifest 并解析可执行文件，`--yes` 才执行 describe 并保存注册。

```json
{
  "protocol_version": "hell-mode/v1",
  "name": "demo",
  "command": ["/tmp/hell-plugin-demo"],
  "timeout_ms": 10000,
  "env": []
}
```

command 是 argv，首项在注册时解析为绝对路径，不隐式通过 shell 执行。name 使用小写字母开头，允许小写字母、数字和短横线，最长 64 字符；log 与 pushover 为内置保留名称。超时范围 1–30000 毫秒，省略或填 0 时使用默认 10000。

环境默认仅传 PATH 与 SystemRoot，额外变量在 env 中列出名称；manifest 不承载密钥值。插件以当前操作系统用户权限运行，进程分离不构成安全沙箱。只注册本人明确信任的程序，不扫描目录自动加载，也不自动下载或升级程序。

## 一次调用

每次调用启动一个进程，stdin 写入一个请求对象并关闭，stdout 必须只有一个响应对象。stderr 可用于诊断。请求和响应均使用 protocol_version 与 id 关联，版本或标识不匹配会被拒绝。

```json
{
  "protocol_version": "hell-mode/v1",
  "id": "req_example",
  "method": "notify",
  "notification": {
    "task_id": "task_example",
    "incident_id": "incident_example",
    "attempt_id": "attempt_example",
    "title": "hell-mode：你有事了",
    "body": "验收没过，先别急着散会。\n任务 task_example · verification_unmet\n第 1 次通知尝试。详情：hell status task_example",
    "expires_at": "2026-10-23T10:00:00Z"
  }
}
```

```json
{
  "protocol_version": "hell-mode/v1",
  "id": "req_example",
  "result": {"state": "queued", "receipt": "provider-receipt"}
}
```

| 方法 | 请求 | 成功响应 result |
| --- | --- | --- |
| describe | 无 notification。 | name 必须匹配 manifest，capabilities 必须包含 notify 和 cancel。 |
| notify | 含任务、告警、尝试标识与到期时间。 | 非空 receipt，state 为 queued 或 simulated。 |
| cancel | notification 中包含已有 receipt。 | state 为 cancelled，表示后续重复动作已停止。 |

响应必须且只能包含 result 或 error。错误格式为 `{"code":"provider_unavailable","message":"..."}`；核心显示错误 code，避免把插件诊断中的凭据抄入输出。

stdout 上限为 1 MiB，stderr 捕获上限为 64 KiB。超时会终止进程；macOS / Linux 同时终止其进程组。插件须尊重 expires_at，并将 attempt_id 用作供应商支持的幂等键。取消应幂等，已经停止的回执可继续返回 cancelled。

主动邀谈复用同一协议：`task_id` 表示邀谈计划，`incident_id` 表示轮次，并附带 `priority: "normal"`。插件须把它作为一次普通通知处理，不启用重复响铃或紧急升级；成功时也可返回 `{"state":"sent"}`，无需回执。正文只含通用提示与本地查看命令，个性化内容保存在本地。任务告警省略该字段，沿用原有策略。

核心在调用前持久化发送尝试。崩溃或超时造成结果不明时，记录 unknown 并计入次数上限，后续按策略执行新的尝试；不承诺跨任意供应商的严格恰好一次。queued 只表示供应商受理，不表示设备收到或本人清醒。

## 示例插件

Go 协议类型位于 [pkg/protocol](../pkg/protocol/protocol.go)，完整演练插件位于 [examples/hell-plugin-demo](../examples/hell-plugin-demo/main.go)。它声明 notify / cancel 能力，返回 simulated 回执，不发送真实通知。

```sh
go build -o /tmp/hell-plugin-demo ./examples/hell-plugin-demo
hell plugin add --manifest demo-plugin.json --dry-run --json
hell plugin add --manifest demo-plugin.json --yes --json
hell plugin list --json
```

demo-plugin.json 使用本文开头的 manifest。注册后在布防时使用 `--channel demo`。同名注册拒绝覆盖；移除前须停止引用该通道的任务、暂停邀谈计划，并处理未取消的回执或在途发送。`plugin remove demo` 仅移除注册，不删除程序。
