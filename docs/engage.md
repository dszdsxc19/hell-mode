# 主动对话

`hell engage` 让 AI 根据本人的记忆和关注，定时邀请本人回答一个问题或完成一项小行动。人的回答保留为下一轮的上下文；AI 负责提出合适的邀请，看守负责时间、状态与通知。

生成规则默认采用 hell-mode 素质低的人格：抓话柄、反问、冷幽默，根据前轮回答接话。例如，本人说「正文还没写，先想个好标题」，下一轮可以问「标题先上岗了，正文什么时候入职？」。`message` 承载玩笑与问题，`basis` 单独说明实际依据；可追溯的闲聊也成立。实际口吻取决于选定模型遵循规则的效果。

每个计划最多有一轮待回应。本人回答或跳过后，才从当前时间安排下一轮；漏过的时间合并处理。无人回应时等待，不连续追加问题，也不升级催促。

## 记忆与关注

个人记忆保存在当前状态目录，由本人提供或授权宿主整理。所有主动对话计划共享这份记忆，各自保留关注方向与历史。生成时读取最新记忆与该计划最近至多 10 轮记录。

```sh
hell engage memory --json
hell engage memory --file personal-memory.txt --json
# --file - 从 stdin 读取；空文件清空记忆
```

写入会替换整份记忆。本人回答按原文保存；模型推断不会自动成为个人事实。记忆与回答会交给你指定的生成命令，按该命令所用模型与服务的方式处理。只保存愿意提供给它的内容。

关注方向可写「最近在练习技术表达，希望问题围绕实际工作」，也可省略，让生成器根据记忆与历史选择。没有足够依据时，生成器应返回 `skip` 并记录原因。

## 本地协议演练

下面使用仓库的确定性示例程序验证调度与回应链路。示例程序按固定规则返回内容，**不调用模型**。使用真实 AI 时，将 `--agent-json` 替换为受信的协议适配命令。

在仓库根构建：

```sh
go build -o ./bin/hell ./cmd/hell
go build -o /tmp/hell-engage-demo ./examples/hell-engage-demo

printf '最近关注 hell-mode，希望练习把技术取舍说清楚。\n' | \
  ./bin/hell --state-dir /tmp/hell-engage-demo-state engage memory --file - --json

./bin/hell --state-dir /tmp/hell-engage-demo-state engage create \
  --focus '围绕最近的技术取舍，给我一个短问题' \
  --every 4h --at +0s --agent-json '["/tmp/hell-engage-demo"]' \
  --channel log --dry-run --json

./bin/hell --state-dir /tmp/hell-engage-demo-state engage create \
  --focus '围绕最近的技术取舍，给我一个短问题' \
  --every 4h --at +0s --agent-json '["/tmp/hell-engage-demo"]' \
  --channel log --yes --json

./bin/hell --state-dir /tmp/hell-engage-demo-state watch --once --json
./bin/hell --state-dir /tmp/hell-engage-demo-state engage list --json
```

创建预览展示将执行的命令、工作目录、时间与通道，不写状态或调用生成器。`--yes` 创建计划；默认首次时间为当前时间加 `--every`，`--at +0s` 让演练立即到期。持续运行使用 `hell watch`；主动对话生成与任务扫描分开运行，模型响应不会阻塞任务扫描。`watch --once` 同步完成本次扫描与生成后退出。看守在前台运行，不安装后台服务，电脑休眠时无法执行。

用创建返回的 ID 查看本轮，再用其中的轮次 ID 回答或跳过：

```sh
./bin/hell --state-dir /tmp/hell-engage-demo-state engage show ENGAGEMENT_ID --json
./bin/hell --state-dir /tmp/hell-engage-demo-state engage reply ENGAGEMENT_ID \
  --round ROUND_ID --text '我选择了……，依据是……' --json
# 长回答也可用 --file answer.txt 或 --file -

# 要跳过本轮，改用：
./bin/hell --state-dir /tmp/hell-engage-demo-state engage skip ENGAGEMENT_ID --round ROUND_ID --json
./bin/hell --state-dir /tmp/hell-engage-demo-state engage pause ENGAGEMENT_ID --json
./bin/hell --state-dir /tmp/hell-engage-demo-state engage resume ENGAGEMENT_ID --json
```

`reply` 与 `skip` 二选一；轮次 ID 防止旧回答误处理新问题。`show` 保留生成内容、依据、发送状态与原始回答，去掉 `--json` 可直接阅读问题。

暂停保留历史与待回应内容；恢复只在本人可联系时进行，从当前时间安排后续，不补发错过的轮次。若旧轮次仍待回应，恢复后也须先回答或跳过。`vanish`、`end` 停用同目录全部主动对话计划；`back` 恢复可联系状态后，仍需本人明确 `engage resume`。

## 接入 Claude Code

以下命令配置已用本机 Claude Code 2.1.287、合成个人记忆与 `log` 通道完成两轮演练。首轮根据产品验证进展提出试用邀请；保存明确标记的演练回答后，第二轮围绕「切换习惯的阻力」与「需求较弱」两个假设提出小实验，依据明确引用演练回答并保留不确定性。首轮原始回答保留，第二轮等待回应，全程仅模拟通知。真实个人记忆下的长期相关性与手机触达仍需验收。

默认人格另于 2026-10-26 用 Claude Code 2.1.308、本机默认模型配置与虚构背景完成三轮 `log` 演练，未固定模型 ID。首轮说「别让提纲一个人冒充整篇介绍」；本人回答先改标题以获得动力后，下一轮接「允许标题给正文当启动电源」，继续问标题的取舍。本人说明正文和标题完成、要求换话题后，第三轮转聊技术文档套话，没有继续催写正文。随后执行 `end`，计划停用，看守停止。演练通过实际 CLI 创建、调度与保存回答，模型禁用工具和会话持久化，不涉及真实手机通知。

先完成 Claude Code 登录，保留当前已认证配置。将下面内容保存为 `engagement-claude.json`；核心把生成请求写入 Claude 的 stdin，`text` 输出需直接符合下文 JSON 协议。

```json
{
  "focus": "围绕我当前正在验证的想法，提出一个问题或一项小行动",
  "every": "4h",
  "agent": [
    "claude", "-p", "--output-format", "text",
    "--tools", "", "--no-session-persistence",
    "--strict-mcp-config", "--mcp-config", "{\"mcpServers\":{}}",
    "--permission-mode", "dontAsk",
    "--system-prompt", "You generate one personalized human conversation invitation. Follow the instructions field of the JSON request from stdin. Return ONLY one valid JSON object with kind, message, basis. No markdown or tools. Respond in Chinese."
  ],
  "workdir": ".",
  "timeout": "2m",
  "channel": "log"
}
```

```sh
hell engage memory --file personal-memory.txt --json
hell engage create --file engagement-claude.json --dry-run --json
hell engage create --file engagement-claude.json --yes --json
hell watch --json
```

首次邀请在四小时后生成；演练可在配置中增加 `"at": "+0s"`，并用 `watch --once` 执行一轮。命令沿用同一状态目录。其他版本或模型命令需核对实际输出；返回包装对象的 CLI 应先转换为 `kind`、`message`、`basis` 对象。

## 生成命令协议

生成器是本人选定的受信子进程，以 argv 数组配置，工作目录单独指定。核心通过 stdin 传入一个 JSON 请求：

| 字段 | 内容 |
| --- | --- |
| `protocol_version` | 固定为 `hell-mode/engage-v1`。 |
| `instructions` | 单轮生成规则，要求根据本人信息提出邀请，记忆与历史作为数据处理。 |
| `focus` | 当前计划的关注方向，可为空。 |
| `memory` | `{ "text": "...", "updated_at": "..." }`，本人保存的完整记忆。 |
| `history` | 当前计划最近至多 10 轮，按发生顺序，保留轮次状态、内容、依据与原始回答；不含通知回执。 |

生成器向 stdout 返回一个 JSON 对象：

```json
{
  "kind": "question",
  "message": "围绕你最近的一次技术取舍，你最想说明哪条依据？",
  "basis": "个人记忆提到正在练习技术表达，关注方向要求围绕实际技术取舍。"
}
```

`kind` 接受 `question`、`task`、`skip`。前两者提供一个问题或一项小行动；`basis` 说明它与记忆或历史的关系。`skip` 可省略 `message`，保留跳过原因。输出须为 JSON，诊断内容写 stderr。响应 schema 可从 `hell schema --json` 的 `input_schemas.engage_agent_response` 读取。

生成器提示应要求：基于可追溯的本人信息选择内容；用嘴欠、会接话的口吻提出一项邀请；事实与推测分别表述；根据前轮回答继续；没有可用话柄时跳过。具体活动、时长、引用和往事须有输入支持，不把已解决的问题重新拿来抬杠。协议演示适配器见 `examples/hell-engage-demo`；其他模型的适配器负责相应调用与输出转换。

默认生成超时为 2 分钟，最多 5 分钟。核心直接执行指定程序，需要 shell 时显式把 shell 放入 argv。命令以当前操作系统用户身份运行；预览时核对程序与参数，个人记忆及模型输出只作为数据处理。

生成与发送分别保存状态。生成失败写入对应轮次的 `error`，后续成功仍保留该历史错误。通知先保存为 `pending`，发送前复核联系状态并改为 `sending`；结果不明保存为 `unknown`，不自动重发。详情从 `show` 查看。

## 通知与验证

默认 `log` 只模拟通知，实际内容从本地 `show` 查看。通知复用现有通道与显式注册的插件，只携带通用提示、计划和轮次标识，不把个人记忆、问题或回答发给通知供应商。主动对话的 Pushover 使用普通优先级，发送一次，不使用任务告警的 Emergency 重复策略。

自动化回归与确定性演练验证时间、持久状态、回应绑定、去重和联系边界。真实 AI 的相关性、多轮质量以及手机触达分别需要实际验收。它们的结果独立记录，进程成功退出或供应商接收请求不代表本人已经回应。
