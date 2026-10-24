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

## 持续质疑对话

实际对话的加载方式、示例与退出指令见 [质疑模式指南](challenge.md)。本节约定 CLI 的输入与返回值。

```sh
hell challenge '这个产品真的有人需要吗？'
hell challenge '这个产品真的有人需要吗？' --json
hell challenge
```

`challenge [TOPIC]` 返回 `hell-challenge` skill 的完整规则与可选话题，`data` 字段如下：

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| `skill` | string | 固定为 `hell-challenge`。 |
| `topic` | string | 原样保留传入的话题；省略时为空字符串。 |
| `instructions` | string | 随 CLI 嵌入的完整 `SKILL.md`，含 frontmatter。 |
| `host_required` | boolean | 固定为 `true`，提问与后续回答由宿主 Agent 执行。 |

请当前 Agent 加载 `instructions`，把 `topic` 作为待讨论的用户主张，开始追问。省略话题时，Agent 沿用当前对话，确实没有话题时才询问。

普通输出适合复制给 Agent，JSON 适合由 Agent 工具读取。话题作为单个参数传入，含空格时使用引号；显式传入空白话题或多个位置参数返回退出码 2。

普通输出包含加载提示、同一份 skill 原文和可选话题。命令不调用模型、启动交互聊天或创建状态目录，`--state-dir` 对该命令无影响。对话上下文由宿主保留。

`hell setup` 可导出同一份 `.agents/skills/hell-challenge/SKILL.md`；导出后的宿主安装与更新见 [安装说明](install.md#安装质疑-skill)。

## 导出集成文件

```sh
hell setup --output ./integration --dry-run --json
hell setup --output ./integration --yes --json
```

`--output` 必填。未传 `--yes` 或显式传入 `--dry-run` 时，仅返回导出计划。`--dry-run` 优先于 `--yes`，预览不创建目录。

JSON 的 `data` 包含绝对路径 `output`、布尔值 `dry_run`、路径到文件内容的映射 `files`，以及固定为 `false` 的 `live_settings_modified`。文件清单固定为：

| 相对输出路径 | 用途 |
| --- | --- |
| `.claude/settings.hell-mode.json` | Claude Code 监看 hooks 的待合并片段。 |
| `.agents/skills/hell-mode/SKILL.md` | 任务监看 skill。 |
| `.agents/skills/hell-challenge/SKILL.md` | 质疑对话 skill。 |

实际导出时，任何目标文件已存在则返回冲突与退出码 5，不覆盖文件；写入路径中的子目录为符号链接时也会拒绝。预览不检查这些写入条件。`setup` 只导出文件，宿主发现目录和已有设置合并由安装步骤处理；它不会全局安装 skill 或自动修改 `.claude/settings.json`。只使用质疑模式时，可仅安装对应 skill。

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

## 番茄失踪与今天结束

```sh
hell vanish --for 25m
hell presence --json
hell back
hell end
```

这些动作作用于同一状态目录的全部任务与 hell-mode 通知，不拦截宿主独立发出的通知或权限弹窗。`vanish` 默认 25 分钟，只接受正的 Go duration。它保存全局 `away` 状态，将现有任务设为 `quiet: true`，取消已知回执；运行与验收可以继续，新的等待和失败证据继续保存。失踪期间新建的任务也保持静默。

到达 `until` 后仍是 `away`，没有闹铃、自动返回或补发。只有本人明确执行 `back`，全局状态才恢复为 `available`。旧任务的 `quiet` 保留；本人查看 `status` 后，可明确执行 `run TASK_ID --rearm -- COMMAND...` 开启新一轮监看，沿用原契约与截止时间。回来本身不表示接管任何告警。

`end` 保存 `ended`，使当前看守在下一次状态检查时静默退出。它保留任务和真实验收结论，不终止 Agent 进程，不在第二天自动恢复。之后若要监看，先由本人执行 `back`，再显式启动 `watch`。`watch --once --json` 在结束状态只重试取消清理，返回 `stopped: true`。看守即使错过快速的 end → back，也会通过持久停止代数退出。

`presence` 只读，未初始化时返回 `available`，不创建状态目录。返回字段为 `mode`、`since`、`until` 和 `watch_epoch`；`until` 表示约定的时间，不代表人已回来。`vanish` 与 `end` 的 JSON 另带 `pending_cancellations`，表示已知在途通知或尚未确认取消的回执数。普通输出不追问原因，也不附加下一步建议。

人类边界操作始终以本人指令为依据，Agent 不得因为时间到了、任务失败或看到了任务输出而执行 `back`、`end` 或解除静默。单任务 `snooze` 仍按原约定到期恢复；全局失踪优先于它。

通知请求已在途时，外部供应商仍可能完成发送；收到迟到回执后会立即取消。取消失败保留记录，`watch` 或 `watch --once` 可在已有次数上限内重试。长验收会延后该进程的退出检查，但不会解除持久静默。

## 状态与进程

状态目录由 `--state-dir`、环境变量 `HELL_STATE_DIR` 或系统用户配置目录下的 `hell-mode` 确定。状态使用 bbolt 单文件事务；CLI 与看守须共用目录。目录和运行日志按当前用户私有权限创建，日志位于 `runs/RUN_ID.log`。当前状态版本为 2，首次写入自动迁移 v1；旧程序会拒绝 v2，避免丢失人的静默边界。

`doctor --json` 仅检查本地状态、看守最近活动时间与凭据是否存在；不会发送通知，也不证明手机可达。启用真实通知的 `run` 要求看守在最近五秒内有过活动；静默任务和 log 演练允许离线运行。看守最近活动仍无法保证之后持续存活。

`watch` 在前台运行，不自行后台化或安装系统服务。当前实现仅支持本地共享状态，远程接入和整机故障覆盖尚未实现。macOS / Linux 的进程取消覆盖子进程组；Windows 仅有编译检查，后代进程清理尚未验收。

## Hooks 与事件权限

`hell run` 向子进程传递 HELL_TASK_ID、HELL_RUN_ID、HELL_EVENT_TOKEN 和绝对路径 HELL_STATE_DIR。事件令牌的哈希保存在状态库，明文不进入任务查询结果。`hell event --file - --json` 从环境变量读取令牌；输入字段见 `schema` 中的 event schema。

事件类型为 heartbeat、exited、failed、waiting、turn_completed。事件标识用于去重，旧执行事件不能改变新执行；一轮结束带有 background_work 时继续监看。结束事件将验收请求持久化，由看守执行，事件入口和 hook 回调不等待耗时验收。没有 hell run 上下文的宿主回调安静忽略，不影响普通宿主会话。

验收在有时限的执行租约内进行，多个看守不会重复领取同一请求。验收进程结果丢失时记录 unknown。当前每个看守串行处理验收和发送，长检查会延后该进程的后续扫描；首版以单任务演练为主。

`hell setup` 返回的 hooks 片段属于 [导出集成文件](#导出集成文件)。使用监看 hooks 时，使用者须审阅并合并片段；真实宿主 hooks 验收尚未完成。

当前事件令牌限制的是事件入口，管理命令依赖操作系统用户权限。同一用户下的 Agent 可以访问该用户文件，这不是安全沙箱；独立管理授权尚未实现。

## 手机通道

默认通道为 log，只保存模拟结果。Pushover 使用 `--channel pushover`，从 HELL_PUSHOVER_APP_TOKEN、HELL_PUSHOVER_USER_KEY 读取凭据；CLI 不提供密钥参数，也不保存密钥。

Pushover 每次应用发送采用 Emergency 通知，内部重复间隔为 30 秒，有效期不超过下次应用发送或告警到期。`max_attempts` 限制应用 API 尝试总数，不是设备响铃次数。发送超时或回执不明记为 unknown 并计入上限；取消失败会保留错误，最多尝试五次。

该通道仅通过本地 HTTP 测试，真实手机、专注模式、静音、网络和供应商限制尚待本人实机验证。供应商回执不会自动完成目标或代替本人接管。
