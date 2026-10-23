# Contributing

hell-mode 是仅供娱乐的 Agent 通知实验。欢迎修复可复现的问题、补充宿主集成证据和开发通知插件。请使用自己的测试设备；演练默认使用 `log` 通道。

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
| `assets/skills`、`internal/cli/integrations.go` | Skill 导出与宿主事件适配 |

先阅读 [领域术语](CONTEXT.md)、[CLI 契约](docs/cli.md) 和 [插件协议](docs/plugins.md)。保持验收结论与告警处理分别记录：接管停止提醒，目标证据继续保留。JSON stdout、退出码和插件协议也是面向 Agent 的公共接口，变更时请说明兼容影响。

## 提交问题与 PR

Bug 报告请提供 `hell version --json`、命令、退出码和最小复现。优先把问题缩小到 `log` 通道；共享输出前移除密钥、个人信息和私有代码。

PR 聚焦一个问题，说明行为变化和实际验证结果。行为修改补能证明问题的回归；文案修改直接核对即可。编译通过、HTTP 模拟通过、宿主实测通过和手机实测通过分别记录。涉及协议或状态格式的修改，先在 Issue 中说明迁移方案。

## 可以从这里开始

- 用真实 Claude Code 运行 hooks 与导出的 skill，提交宿主版本、配置步骤和事件覆盖证据。
- 按现有协议写一个 Python 或 Node 通知模拟插件，覆盖 describe、notify、cancel 与错误响应。
- 将 README 的本地演练步骤整理为英文入口，保留当前实现与未来设想的边界。
- 为一条具体 CLI 错误补充可操作的 help 示例和回归场景。

较大的功能先讨论具体场景与验收方式。当前维护聚焦现有能力的可靠性、安装使用和真实宿主验收；更多叫人渠道与设备联动保留在早期创作档案中。
