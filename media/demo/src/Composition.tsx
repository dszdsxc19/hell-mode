import { useEffect, useState } from "react";
import {
  AbsoluteFill,
  cancelRender,
  continueRender,
  delayRender,
  interpolate,
  useCurrentFrame,
} from "remotion";
import "@fontsource/noto-sans-sc/500.css";
import "@fontsource/noto-sans-sc/700.css";
import capture from "./capture.json";

const ink = "#eef0ec";
const muted = "#929b9e";
const orange = "#ff784f";
const green = "#a4d6b3";
const chinese =
  "仅供娱乐本地模拟Agent没干完你也别想睡验收未通过独立看守重复提醒番茄失踪人类暂不提供服务约定时间已到人类尚未返回是否回来由你决定你回来了任务和问题保存在这里需要人的事情请保存并等待实际结果时间经过剪辑";

export const HellMode: React.FC = () => {
  const frame = useCurrentFrame();
  const [fontHandle] = useState(() =>
    delayRender("Load self-hosted Noto Sans SC"),
  );
  useEffect(() => {
    Promise.all([
      document.fonts.load(
        '500 40px "Noto Sans SC"',
        chinese + capture.end.message,
      ),
      document.fonts.load(
        '700 70px "Noto Sans SC"',
        chinese + capture.end.message,
      ),
    ])
      .then(() => continueRender(fontHandle))
      .catch(cancelRender);
  }, [fontHandle]);

  const stage =
    frame < 75 ? 0 : frame < 195 ? 1 : frame < 285 ? 2 : frame < 375 ? 3 : 4;
  const starts = [0, 75, 195, 285, 375];
  const attemptCount =
    frame < 150 ? 0 : frame < 175 ? 1 : capture.attempts.length;
  const headline = [
    "Agent 没干完，",
    "验收未通过。",
    "番茄失踪。",
    "时间到了。",
    "你回来了。",
  ][stage];
  const subtitle = [
    "你也别想睡。",
    "独立看守，重复提醒。",
    "人类暂不提供服务。",
    "是否回来，由你决定。",
    "任务和问题保存在这里。",
  ][stage];
  const lines = [
    { at: 75, text: "$ hell run TASK_ID -- agent", color: muted },
    { at: 90, text: `  ${capture.agent_output}`, color: ink },
    { at: 105, text: `  agent exit ${capture.child_exit_code}`, color: green },
    { at: 120, text: `  verdict: ${capture.verdict}`, color: orange },
    {
      at: 135,
      text: `  hell run exit ${capture.run_exit_code}`,
      color: orange,
    },
    ...capture.attempts.map((attempt, index) => ({
      at: 150 + index * 25,
      text: `  watch → log [${attempt.number}/${capture.max_attempts}] ${attempt.state}`,
      color: orange,
    })),
    { at: 195, text: "$ hell vanish --for 100ms", color: ink },
    { at: 212, text: "  番茄失踪已开启。", color: ink },
    { at: 227, text: "  人类暂不提供服务。", color: ink },
    { at: 242, text: "  需要人的事情，请保存并等待。", color: muted },
    { at: 285, text: "$ hell presence --json", color: ink },
    { at: 300, text: `  mode: ${capture.after_timer.mode}`, color: ink },
    { at: 315, text: "  人类尚未返回。", color: muted },
    { at: 375, text: "$ hell back", color: ink },
    { at: 390, text: "  你回来了。", color: ink },
    { at: 405, text: "  任务和问题保存在这里。", color: muted },
    { at: 420, text: `  quiet: ${capture.back.quiet}`, color: muted },
    {
      at: 435,
      text: `  reminders: ${capture.back.reminders_after_wait}`,
      color: muted,
    },
    { at: 450, text: "$ hell end", color: ink },
  ];

  if (frame >= 465) {
    return (
      <AbsoluteFill
        style={{
          backgroundColor: "#101416",
          color: ink,
          fontFamily: '"Noto Sans SC", sans-serif',
          justifyContent: "center",
          padding: "80px 128px",
          fontSize: 48,
          fontWeight: 500,
          lineHeight: 1.7,
        }}
      >
        {capture.end.message.split("\n\n").map((line) => (
          <div key={line} style={{ marginBottom: 34 }}>
            {line}
          </div>
        ))}
      </AbsoluteFill>
    );
  }

  return (
    <AbsoluteFill
      style={{
        backgroundColor: "#101416",
        color: ink,
        fontFamily: '"Noto Sans SC", sans-serif',
        padding: "52px 64px 38px",
      }}
    >
      <div style={{ display: "flex", alignItems: "center", gap: 14 }}>
        <svg width="28" height="34" viewBox="0 0 28 34" fill="none">
          <path
            d="M16 1C18 12 27 13 27 22A13 13 0 0 1 1 22C1 16 5 12 9 9C8 15 10 17 12 18C16 14 16 8 16 1Z"
            fill={orange}
          />
        </svg>
        <span style={{ fontSize: 32, fontWeight: 700, letterSpacing: -1 }}>
          hell-mode
        </span>
        <span
          style={{
            marginLeft: "auto",
            fontSize: 20,
            color: muted,
            border: "1px solid #394144",
            padding: "8px 14px",
            borderRadius: 5,
          }}
        >
          仅供娱乐 · 本地模拟
        </span>
      </div>
      <div style={{ display: "flex", gap: 40, flex: 1, marginTop: 64 }}>
        <div style={{ width: 470, flexShrink: 0 }}>
          <div style={{ color: orange, fontSize: 18, letterSpacing: 2 }}>
            {String(stage + 1).padStart(2, "0")} / 05
          </div>
          <div
            style={{
              fontSize: 64,
              lineHeight: 1.35,
              fontWeight: 700,
              marginTop: 24,
              letterSpacing: -2,
              opacity: interpolate(
                frame,
                [starts[stage], starts[stage] + 10],
                [stage === 0 ? 1 : 0, 1],
                { extrapolateLeft: "clamp", extrapolateRight: "clamp" },
              ),
            }}
          >
            {headline}
          </div>
          <div
            style={{
              fontSize: stage === 0 ? 60 : 34,
              lineHeight: 1.6,
              marginTop: 8,
              color: stage === 0 ? ink : muted,
              fontWeight: stage === 0 ? 700 : 500,
            }}
          >
            {subtitle}
          </div>
          <div style={{ display: "flex", gap: 28, marginTop: 54 }}>
            <div>
              <div style={{ fontSize: 16, color: muted, letterSpacing: 2 }}>
                GOAL
              </div>
              <div
                style={{
                  fontSize: 26,
                  color: frame >= 120 ? orange : muted,
                  marginTop: 9,
                }}
              >
                {frame >= 120 ? capture.verdict : "pending"}
              </div>
            </div>
            <div style={{ width: 1, backgroundColor: "#30383b" }} />
            <div>
              <div style={{ fontSize: 16, color: muted, letterSpacing: 2 }}>
                {stage >= 2 ? "HUMAN" : "REMINDERS"}
              </div>
              <div style={{ fontSize: 26, marginTop: 9 }}>
                {stage >= 2
                  ? stage === 4
                    ? capture.back.mode
                    : capture.vanish.mode
                  : `${attemptCount} / ${capture.max_attempts}`}
              </div>
            </div>
          </div>
        </div>
        <div
          style={{
            flex: 1,
            minWidth: 0,
            border: "1px solid #343d41",
            borderRadius: 10,
            backgroundColor: "#161c1f",
            overflow: "hidden",
            alignSelf: "flex-start",
          }}
        >
          <div
            style={{
              padding: "15px 20px",
              borderBottom: "1px solid #343d41",
              fontSize: 15,
              color: muted,
              display: "flex",
              gap: 8,
              alignItems: "center",
            }}
          >
            {["#ff784f", "#d6b86d", "#86ac96"].map((color) => (
              <span
                key={color}
                style={{
                  width: 8,
                  height: 8,
                  borderRadius: "50%",
                  backgroundColor: color,
                }}
              />
            ))}
            <span style={{ marginLeft: 8 }}>hell / local rehearsal</span>
          </div>
          <div
            style={{
              padding: "24px 20px",
              height: 380,
              fontFamily:
                '"SFMono-Regular", Consolas, "Liberation Mono", "Noto Sans SC", monospace',
              fontSize: 18,
              lineHeight: 1.8,
            }}
          >
            {frame < 75 ? (
              <>
                <div style={{ color: muted }}>$ hell arm --channel log</div>
                <div style={{ color: green, marginTop: 12 }}>
                  {" "}
                  armed · max attempts {capture.max_attempts}
                </div>
                <div style={{ color: muted }}> watch running independently</div>
                <div style={{ marginTop: 24, color: orange }}>
                  {" "}
                  waiting for the agent_
                </div>
              </>
            ) : (
              lines
                .filter((line) => frame >= line.at)
                .slice(-10)
                .map((line) => (
                  <div
                    key={line.at}
                    style={{
                      color: line.color,
                      whiteSpace: "pre",
                      opacity: interpolate(
                        frame,
                        [line.at, line.at + 6],
                        [0, 1],
                        { extrapolateLeft: "clamp", extrapolateRight: "clamp" },
                      ),
                    }}
                  >
                    {line.text}
                  </div>
                ))
            )}
          </div>
        </div>
      </div>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          fontSize: 16,
          color: muted,
          marginTop: 32,
        }}
      >
        <span>github.com/dszdsxc19/hell-mode</span>
        <span style={{ marginLeft: "auto" }}>实际 CLI 结果 · 时间经过剪辑</span>
      </div>
      <div style={{ height: 3, marginTop: 22, backgroundColor: "#30383b" }}>
        <div
          style={{
            height: 3,
            backgroundColor: orange,
            width: `${interpolate(frame, [0, 464], [0, 100])}%`,
          }}
        />
      </div>
    </AbsoluteFill>
  );
};
