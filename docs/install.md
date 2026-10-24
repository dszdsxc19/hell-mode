# 安装与本地演练

当前提供开发预览，尚无标记版本的 GitHub Release。`hell-challenge` 可直接交给 Agent 读取，无需安装 CLI；任务监看使用 `hell` CLI 和独立看守进程。两份 skill 的安装都独立于 hooks 配置。

## 获取源码

需要 Git：

```sh
git clone https://github.com/dszdsxc19/hell-mode.git
cd hell-mode
```

只使用质疑对话时，直接前往[安装质疑 skill](#安装质疑-skill)。以下构建步骤用于需要 CLI 的场景。

## 从源码构建

需要 Go 1.23+。在仓库根目录执行，以下命令使用 macOS / Linux 的 shell：

```sh
go build -trimpath -o ./bin/hell ./cmd/hell
./bin/hell version --json
./bin/hell --help
```

也可安装当前 main：

```sh
go install github.com/dszdsxc19/hell-mode/cmd/hell@main
```

可执行文件写入 `GOBIN`，未设置时写入 `$(go env GOPATH)/bin`；将该目录加入 PATH 后运行 `hell`。`@main` 随提交变化，复现特定结果时可将其替换为完整提交 SHA。源码构建的版本字符串可能仍为 `0.1.0-dev`，应同时记录 `git rev-parse HEAD`。

Windows 可从 PowerShell 构建：

```powershell
go build -trimpath -o .\bin\hell.exe ./cmd/hell
.\bin\hell.exe version --json
```

Windows 目前只验证编译，后代进程取消与完整运行链路尚未验收。

Go CLI 的安装和运行不需要 Node；Node 只用于制作 README 动画。

## 下载 CI 预览构建

打开 [CI 运行记录](https://github.com/dszdsxc19/hell-mode/actions/workflows/ci.yml)，选择目标提交的成功运行，下载 `hell-cli-<完整提交 SHA>` artifact。包含 macOS / Linux 的 amd64、arm64，以及 Windows amd64 二进制和 `SHA256SUMS`。

解压后校验文件：

```sh
# Linux
sha256sum -c SHA256SUMS
# macOS
shasum -a 256 -c SHA256SUMS
```

为本机二进制添加执行权限，例如 `chmod +x hell-darwin-arm64`，然后通过 `./hell-darwin-arm64 --help` 运行。预览产物按提交标识，遵循 GitHub artifact 的保留期限；不等同于稳定版本发布。macOS 预览未签名或公证，系统可能提示未知开发者；有疑问时自行从源码构建。

## 安装质疑 skill

当前对话可直接读取源码或通过 CLI 取得规则，具体指令见 [开始一轮对话](challenge.md#开始一轮对话)。在其他项目的会话中读取源码时，提供 `assets/skills/hell-challenge/SKILL.md` 的实际绝对路径；Agent 无法读取文件时，可交给它完整内容。源码读取无需 Go 或 CLI。

以下步骤用于长期安装到项目，质疑规则仍由宿主 Agent 执行。对话用法和退出方式见 [质疑模式指南](challenge.md)。

### 导出并安装到项目

先导出到新的临时目录，审阅后再安装；以下命令适用于 macOS / Linux，假设 `hell` 已在 PATH：

```sh
challenge_export=$(mktemp -d)
hell setup --output "$challenge_export" --dry-run --json
hell setup --output "$challenge_export" --yes --json
```

导出包含 `.agents/skills/hell-challenge/SKILL.md`、`.agents/skills/hell-mode/SKILL.md` 和 `.claude/settings.hell-mode.json`。`setup` 只写入指定目录，不会自动安装到宿主，也不会合并现有设置。仅使用质疑模式时，只安装 `hell-challenge`。

| 宿主 | 项目内安装位置 | 安装后的显式调用 |
| --- | --- | --- |
| Codex | `.agents/skills/hell-challenge/SKILL.md` | CLI / IDE 中输入 `$hell-challenge`，或通过 `/skills` 选择 |
| Claude Code | `.claude/skills/hell-challenge/SKILL.md` | 输入 `/hell-challenge` |

下面以 Codex 为例。将 `target_project` 改成目标项目的绝对路径；Claude Code 将 `challenge_skill_dir` 中的 `.agents` 改成 `.claude`：

```sh
target_project='/absolute/path/to/your/project'
challenge_skill_dir="$target_project/.agents/skills/hell-challenge"
mkdir -p "$(dirname "$challenge_skill_dir")"
mkdir "$challenge_skill_dir" && cp "$challenge_export/.agents/skills/hell-challenge/SKILL.md" "$challenge_skill_dir/SKILL.md"
```

目标目录已存在时，最后一行会停止复制；按下节升级，保留已有定制。

Codex 会自动发现 skill 变更；CLI / IDE 中未出现时重启 Codex，再查看技能列表。桌面端的选择入口随当前版本而定，当前对话可直接读取原文路径。目录与发现方式依据 [OpenAI 官方 skills 文档](https://learn.chatgpt.com/docs/build-skills#where-codex-loads-local-skills)。

Claude Code 会刷新已有技能目录内的 `SKILL.md`；若会话启动时顶层 `.claude/skills/` 尚不存在，运行 `/reload-skills`，此后该会话中再修改它仍需重新运行这条命令。目录与刷新方式见 [Claude Code 官方 skills 文档](https://code.claude.com/docs/en/skills#edit-a-skill-during-a-session)。这些是宿主文档规定的加载方式；本仓库的真实宿主安装与持续对话效果仍待验收。

### 升级已有安装

按原安装来源更新 CLI；源码安装需重新构建，`go install` 安装则重跑安装命令。`hell challenge` 和 `hell setup` 使用 CLI 内嵌规则，更新源码文件不会更新旧二进制。直接读取源码的方式使用该检出中的规则。

重新执行上面的导出步骤，生成新的 `challenge_export`，然后对比当前安装：

```sh
challenge_backup=$(mktemp)
cp "$challenge_skill_dir/SKILL.md" "$challenge_backup"
diff -u "$challenge_skill_dir/SKILL.md" "$challenge_export/.agents/skills/hell-challenge/SKILL.md"
```

`diff` 返回 1 表示存在差异。对照差异编辑已安装的 `SKILL.md`，保留本地定制，再按宿主的方式刷新并重新调用 skill；需要回退时，运行 `cp "$challenge_backup" "$challenge_skill_dir/SKILL.md"`。`setup` 实际写入时遇到同名输出文件会拒绝导出，升级时始终使用新目录。已有 `.claude/settings.json` 和其他 skill 保持原样；安装监看 skill 时还需按其规则准备 CLI、状态目录和看守进程。

## 演练与停止

[README 快速开始](../README.md#开始演练) 使用故意失败的验收和 `log` 通道，不发送真实通知。两个终端须使用同一状态目录。`run` 返回退出码 3 是这次演练的预期结果，后续 `status` 和 `takeover` 仍应执行。

`takeover TASK_ID` 停止本轮提醒并保留验收证据；`disarm TASK_ID` 解除布防；在看守终端按 Ctrl+C 停止看守进程。默认状态目录由 `hell doctor --json` 查看，也可用 `--state-dir` 或 `HELL_STATE_DIR` 指定。

`vanish --for 25m` 让同一目录的任务保持静默，到期仍等待本人主动 `back`；`end` 结束本次值班并让看守退出。复用已结束的演练目录时，由本人先执行 `back`，再启动 `watch`。旧任务保持静默，重新监看须明确 `run --rearm`。这些操作保留真实验收结论，见 [人的联系边界](cli.md#番茄失踪与今天结束)。

本地看守与任务机共享文件状态，整机休眠或断电期间无法继续执行检查和通知。手机详情入口、远程看守和电话通道尚未实现。Pushover 适配器只完成本地 HTTP 模拟验证，真实手机行为待验收。
