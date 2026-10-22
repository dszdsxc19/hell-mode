# CLI 契约

> 仅供娱乐。演练使用本人设备，安排在白天。

命令名为 `hell`，Go 最低版本为 1.23。所有命令都提供 `--help`；传入 `--help --json` 获得对应命令的结构化说明。`hell schema --json` 返回整个命令树、参数默认值、JSON 输入 schema 与退出码，不创建状态目录。

## Agent 调用约定

有限命令在 `--json` 模式下向 stdout 写入一个 `hell-mode/v1` JSON 对象：

```json
{
  "schema_version": "hell-mode/v1",
  "ok": false,
  "command": "hell verify",
  "data": {"id": "task_example", "verdict": "unmet"},
  "error": {
    "code": "goal_unmet",
    "message": "verification did not meet the goal; inspect task checks",
    "next_actions": ["hell verify --help", "hell schema --json"]
  }
}
```

`data` 随命令变化，可能为 null。`ok` 表示命令操作是否成功，任务目标以 `data.verdict` 或 `data.task.verdict` 为准。`status` 成功读取一个未达成任务时仍返回 `ok: true`。

`watch --json` 是持续命令，每个扫描周期输出一行相同信封格式的 NDJSON；`watch --once --json` 返回单个结果。`run --json` 的子进程输出写入私有日志，不混入 stdout。宿主调用的 `hook claude-code` 默认成功时不输出内容，避免向宿主注入控制决定；显式加 `--json` 才输出结果。

所有命令均不弹交互问题。`--no-input` 同时断开被包装子进程的 stdin。`arm`、`plugin add` 的写入需要 `--yes`；`setup` 未传 `--yes` 时只预览。`--yes` 由调用方确认当前授权，skill 不据此推断用户授权。

| 退出码 | 含义 |
| --- | --- |
| 0 | 操作成功；查询中的目标状态可以为 unmet / unknown。 |
| 1 | 操作失败、中断或无法确认执行结果。 |
| 2 | 参数、JSON 或确认条件不满足。 |
| 3 | 验收未达成或被包装进程失败；分别查看验收结论与 child_exit_code。 |
| 4 | 任务或插件不存在。 |
| 5 | 状态冲突或事件凭据不匹配。 |

出错后先检查 `error.code` 与返回的任务标识；重新查询已有任务，避免重复创建任务或重发结果不明的通知。

## 创建目标契约

`arm` 接受独立 flags 或 `--file FILE`，`--file -` 读取 stdin。两种输入方式互斥，未知 JSON 字段、null 值和尾随对象会被拒绝；可选字段直接省略。验收命令为 argv 数组；需要 shell 语义时显式传入 shell。

```json
{
  "goal": "所有 Go 测试通过",
  "verify": ["go", "test", "./..."],
  "workdir": ".",
  "deadline": "+10m",
  "heartbeat_timeout": "30s",
  "verify_timeout": "2m",
  "channel": "log",
  "interval": "1m",
  "window": "10m",
  "max_attempts": 3
}
```

必填字段为 goal、verify 和 deadline，其余字段默认值如上。时间接受含时区的 RFC3339 或相对 `+duration`；duration 使用 Go 格式，例如 `30s`、`5m`。输出中的持续时间以带 `_ns` 后缀的整数纳秒表示，时间戳统一为 UTC。

```sh
hell arm --file contract.json --dry-run --json
hell arm --file contract.json --yes --json
hell run TASK_ID --json --no-input -- COMMAND ARG
hell verify TASK_ID --json
hell status TASK_ID --json
hell snooze TASK_ID --until +5m --json
hell takeover TASK_ID --json
hell disarm TASK_ID --json
```

`arm` 保存契约，调度由另一个进程中的 `hell watch` 执行。停止后的任务可通过 `run --rearm` 显式开启新执行，沿用既有契约和截止时间；旧通知正在发送或仍等待取消时拒绝开启新执行。验收通过、接管和解除布防分别记录，历次告警与验收证据保留。

## 状态与进程

状态目录由 `--state-dir`、环境变量 `HELL_STATE_DIR` 或系统用户配置目录下的 `hell-mode` 确定。状态使用 bbolt 单文件事务；CLI 与看守须共用目录。目录和运行日志按当前用户私有权限创建，日志位于 `runs/RUN_ID.log`。

`doctor --json` 仅检查本地状态、看守最近活动时间与凭据是否存在；不会发送通知，也不证明手机可达。真实通知通道的 `run` 要求看守在最近五秒内有过活动，演练 log 通道允许离线运行；这仍无法保证看守之后持续存活。

`watch` 在前台运行，不自行后台化或安装系统服务。当前实现仅支持本地共享状态，远程接入和整机故障覆盖尚未实现。macOS / Linux 的进程取消覆盖子进程组；Windows 仅有编译检查，后代进程清理尚未验收。

## Hooks 与事件权限

`hell run` 向子进程传递 HELL_TASK_ID、HELL_RUN_ID、HELL_EVENT_TOKEN 和绝对路径 HELL_STATE_DIR。事件令牌的哈希保存在状态库，明文不进入任务查询结果。`hell event --file - --json` 从环境变量读取令牌；输入字段见 `schema` 中的 event schema。

事件类型为 heartbeat、exited、failed、waiting、turn_completed。事件标识用于去重，旧执行事件不能改变新执行；一轮结束带有 background_work 时继续监看。结束事件将验收请求持久化，由看守执行，事件入口和 hook 回调不等待耗时验收。没有 hell run 上下文的宿主回调安静忽略，不影响普通宿主会话。

验收在有时限的执行租约内进行，多个看守不会重复领取同一请求。验收进程结果丢失时记录 unknown。当前每个看守串行处理验收和发送，长检查会延后该进程的后续扫描；首版以单任务演练为主。

`hell setup --output DIR --dry-run --json` 预览 Claude Code 配置片段与 skill，`--yes` 才导出文件。导出文件不会覆盖已有文件，也不会合并现有 `.claude/settings.json`。使用者须审阅并合并片段；真实宿主验收尚未完成。

当前事件令牌限制的是事件入口，管理命令依赖操作系统用户权限。同一用户下的 Agent 可以访问该用户文件，这不是安全沙箱；独立管理授权尚未实现。

## 手机通道

默认通道为 log，只保存模拟结果。Pushover 使用 `--channel pushover`，从 HELL_PUSHOVER_APP_TOKEN、HELL_PUSHOVER_USER_KEY 读取凭据；CLI 不提供密钥参数，也不保存密钥。

Pushover 每次应用发送采用 Emergency 通知，内部重复间隔为 30 秒，有效期不超过下次应用发送或告警到期。`max_attempts` 限制应用 API 尝试总数，不是设备响铃次数。发送超时或回执不明记为 unknown 并计入上限；取消失败会保留错误，最多尝试五次。

该通道仅通过本地 HTTP 测试，真实手机、专注模式、静音、网络和供应商限制尚待本人实机验证。供应商回执不会自动完成目标或代替本人接管。
