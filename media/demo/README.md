# README 演示

20 秒的连续终端场景：Agent 正常退出，验收失败，独立看守产生两次 `log` 模拟提醒。人开启「番茄失踪」，时间到后仍然安静；主动回来也不补发旧提醒。最后执行 `end`，仅留下 CLI 实际输出的三行文字，停留 4.5 秒。演练部分标注「仅供娱乐 · 本地模拟」。

## 复现

需要 Go 1.23+、Node 24+ 与 npm。从本目录运行：

```sh
npm ci
npm run capture
npm run lint
npm run dev
```

`capture.mjs` 在临时目录构建并调用真实 `hell`，启动独立看守，执行故意失败的验收。演练用 `vanish --for 100ms` 压缩默认 25 分钟的安静期，越过计时后验证人仍为 `away`。执行 `back` 后等待一个提醒间隔，断言旧任务仍保持 quiet、没有补发。最后捕获 `end` 的人类可读输出，验证看守自行以 0 退出、任务仍为 unmet，再清理临时状态。全程使用 `log` 通道，不需要通知凭据。

捕获结果写入 `src/capture.json`，仅保留稳定事实；任务 ID、令牌、绝对路径与时间戳不进入公开素材。画面压缩等待时间，并以摘要形式呈现 JSON；终端中的 `TASK_ID` 和 `agent` 是可读占位符，实际命令见捕获脚本。末尾三行直接使用捕获文本，不附加总结或下一步。该动画验证的是本地模拟链路，宿主和手机实机验证情况见 [README](../../README.md)。

## 输出

```sh
npm run capture
npm run render:gif
npm run poster
npm run render:mp4
```

GIF 为 1200 × 760、15 fps，通过 `--number-of-gif-loops=0` 只播放一次，在结尾停住。PNG 是末尾三行的静态替代图，两者提交到 `docs/assets/`。MP4 为 30 fps，写入被忽略的 `out/`，用于分享。首次渲染可能需要 Remotion 下载 Chrome Headless Shell。

若默认 Headless Shell 在加载阶段超时，可指定本机 Chrome。本次 macOS 导出使用以下方式，其他输出命令同样支持该参数：

```sh
npm run render:gif -- --browser-executable='/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
```

动画由 `useCurrentFrame()` 驱动；字体通过 npm 自托管 Noto Sans SC，不依赖渲染机器的中文系统字体。字体遵循其包内 SIL Open Font License，Remotion 的使用条件见 [官方许可](https://www.remotion.dev/license)。

CI 重新执行捕获并比较事实、检查 TypeScript 和 lint、构建演示 bundle。修改行为或剧情后重新捕获、检查关键画面，再更新 GIF 与 PNG；CI 不会自动覆盖仓库中的媒体。
