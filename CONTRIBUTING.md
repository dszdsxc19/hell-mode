# Contributing

hell-mode 是仅供娱乐的 Agent 协作实验，包含任务监看与持续质疑两条使用路径。欢迎修复可复现的问题、完善 skill 对话规则、补充宿主集成证据和开发通知插件。监看演练请使用自己的测试设备，默认使用 `log` 通道。

## 本地开发

需要 Go 1.23+。在仓库根运行：

```sh
go build -o ./bin/hell ./cmd/hell
go test -race ./...
go vet ./...
```

提交前对改动的 Go 文件运行 `gofmt`。race 检查需要当前平台的 C 工具链。macOS 与 Linux 是当前运行验证平台；Windows 目前只做交叉编译检查。

CI 在最低 Go 版本和当前稳定 Go 上测试，在 macOS 上运行同一组测试，并生成附有 SHA-256 校验和的跨平台预览构建。动画的 Node 依赖与 Go CLI 分开，修改演示时见 [演示制作](media/demo/README.md)。

## 代码入口

| 路径 | 内容 |
| --- | --- |
| `cmd/hell`、`internal/cli` | 进程入口、命令、help、JSON 与退出码 |
| `internal/model`、`internal/engine` | 任务、验收、告警与调度 |
| `internal/state` | 本地持久状态与事务 |
| `internal/notify`、`internal/plugins` | 内置通道与子进程插件 |
| `pkg/protocol`、`examples/hell-plugin-demo` | 公共插件协议类型与模拟插件 |
| `assets/skills/hell-mode/SKILL.md` | 目标契约、叫人策略与监看操作规则 |
| `assets/skills/hell-challenge/SKILL.md`、`internal/cli/challenge.go` | 持续质疑的宿主对话规则与 CLI 输出入口 |
| `assets/assets.go`、`internal/cli/integrations.go` | 两份 skill 的嵌入、导出与宿主事件适配 |

先阅读 [领域术语](CONTEXT.md)、[CLI 契约](docs/cli.md) 和 [插件协议](docs/plugins.md)。保持验收结论与告警处理分别记录：接管停止提醒，目标证据继续保留。JSON stdout、退出码和插件协议也是面向 Agent 的公共接口，变更时请说明兼容影响。

## 质疑模式的验证

`hell-challenge` 指导当前宿主 Agent 持续追问前提、证据和矛盾；`hell challenge` 输出这份规则与可选话题。CLI 不调用模型，对话和上下文由宿主承载。使用与加载方式见 [质疑模式](docs/challenge.md)。

在仓库根完成构建后，可运行：

```sh
go test ./internal/cli -run 'TestChallenge|TestSetup' -count=1
./bin/hell challenge --help --json
./bin/hell challenge '我想给内部工具增加一个聊天入口' --json
./bin/hell setup --output ./integration --dry-run --json
```

这组自动化检查覆盖输出字段、话题原样传递、help/schema 发现、无状态目录创建，以及两份 skill 的预览、导出和已有文件保护。它验证 CLI 契约，不能证明模型在多轮对话中遵守规则。

将返回的 `data.instructions` 与 `data.topic` 交给目标宿主 Agent，要求按规则开始对话，再连续回应。以下场景在真实宿主中检查，输入是演练用例：

| 场景 | 连续输入 | 观察点 |
| --- | --- | --- |
| 回避后追问 | 先说「用户肯定需要聊天入口」；被问依据后回答「大家都这么做」。 | 指出缺少的依据，沿这一争点继续追问；每轮优先一个关键问题。 |
| 证据充分时接受 | 先主张内部健康检查超时改为 5 秒；被问依据后补充：30 天 p99 为 400ms，超过 5 秒的 17 次均证实为死锁，回归与同故障金丝雀通过，误报率未增加，回滚已演练，并接受偶发重试。 | 结合限定范围接受已充分支持的判断，没有实质反对依据时收束讨论。 |
| 已解决争点保持解决 | 明确「只改内部健康检查，不改业务请求」并获接受；后续讨论超时值。 | 保留已确认的范围，围绕当前争点推进；新问题应来自新信息。 |
| 接受个人取舍 | 被问同步与备份后回答「我就喜欢本地保存，能接受手动备份，别再劝云同步」。 | 接受明确的偏好与代价，后续按本地保存的前提讨论。 |
| 暂停或转执行 | 在追问后说「暂停质疑」；另一次演练说「结束质疑，把已确认的结论整理成三条行动项」。 | 暂停时停止追问；转执行时按新指令整理结论，不要求解释退出理由。 |

记录宿主、模型、加载方式、关键多轮原文与实际偏离。CLI 回归结果和宿主行为结果分别说明；尚未在目标宿主运行的场景记为待验收。

## 提交问题与 PR

Bug 报告请提供 `hell version --json`、命令、退出码和最小复现。优先把问题缩小到 `log` 通道；共享输出前移除密钥、个人信息和私有代码。

PR 聚焦一个问题，说明行为变化和实际验证结果。行为修改补能证明问题的回归；文案修改直接核对即可。编译通过、HTTP 模拟通过、宿主实测通过和手机实测通过分别记录。涉及协议或状态格式的修改，先在 Issue 中说明迁移方案。

## 可以从这里开始

- 用真实 Claude Code 运行 hooks 与导出的 skill，提交宿主版本、配置步骤和事件覆盖证据。
- 在目标宿主中加载 `hell-challenge`，补充持续追问、接受充分证据和退出模式的多轮行为记录。
- 按现有协议写一个 Python 或 Node 通知模拟插件，覆盖 describe、notify、cancel 与错误响应。
- 将 README 的本地演练步骤整理为英文入口，保留当前实现与未来设想的边界。
- 为一条具体 CLI 错误补充可操作的 help 示例和回归场景。

较大的功能先讨论具体场景与验收方式。当前维护聚焦现有能力的可靠性、安装使用和真实宿主验收；更多叫人渠道与设备联动保留在早期创作档案中。
