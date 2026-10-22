# 安装与本地演练

当前提供开发预览，尚无标记版本的 GitHub Release。Go CLI 的安装和运行不需要 Node；Node 只用于制作 README 动画。

## 从源码构建

需要 Go 1.23+ 和 Git。以下命令使用 macOS / Linux 的 shell：

```sh
git clone https://github.com/dszdsxc19/hell-mode.git
cd hell-mode
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

## 演练与停止

[README 快速开始](../README.md#开始演练) 使用故意失败的验收和 `log` 通道，不发送真实通知。两个终端须使用同一状态目录。`run` 返回退出码 3 是这次演练的预期结果，后续 `status` 和 `takeover` 仍应执行。

`takeover TASK_ID` 停止本轮提醒并保留验收证据；`disarm TASK_ID` 解除布防；在看守终端按 Ctrl+C 停止看守进程。默认状态目录由 `hell doctor --json` 查看，也可用 `--state-dir` 或 `HELL_STATE_DIR` 指定。

`vanish --for 25m` 让同一目录的任务保持静默，到期仍等待本人主动 `back`；`end` 结束本次值班并让看守退出。复用已结束的演练目录时，由本人先执行 `back`，再启动 `watch`。旧任务保持静默，重新监看须明确 `run --rearm`。这些操作保留真实验收结论，见 [人的联系边界](cli.md#番茄失踪与今天结束)。

本地看守与任务机共享文件状态，整机休眠或断电期间无法继续执行检查和通知。手机详情入口、远程看守和电话通道尚未实现。Pushover 适配器只完成本地 HTTP 模拟验证，真实手机行为待验收。
