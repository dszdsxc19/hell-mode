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
  "仅供娱乐本地模拟Agent没干完你也别想睡它宣布结束开始验收未通过独立看守重复提醒人工接管停止提醒目标保留未达成实际结果时间经过剪辑";

export const HellMode: React.FC = () => {
  const frame = useCurrentFrame();
  const [fontHandle] = useState(() =>
    delayRender("Load self-hosted Noto Sans SC"),
  );
  useEffect(() => {
    Promise.all([
      document.fonts.load('500 40px "Noto Sans SC"', chinese),
      document.fonts.load('700 70px "Noto Sans SC"', chinese),
    ])
      .then(() => continueRender(fontHandle))
      .catch(cancelRender);
  }, [fontHandle]);

  const stage = frame < 90 ? 0 : frame < 180 ? 1 : frame < 330 ? 2 : 3;
  const starts = [0, 90, 180, 330];
  const attemptCount =
    stage < 2 ? 0 : frame < 260 ? 1 : capture.attempts.length;
  const headline = [
    "Agent 没干完，",
    "它宣布结束。",
    "验收未通过。",
    "人工接管。",
  ][stage];
  const subtitle = [
    "你也别想睡。",
    "hell-mode 开始验收。",
    "独立看守，重复提醒。",
    "停止提醒，目标保留未达成。",
  ][stage];
  const lines = [
    { at: 90, text: "$ hell run TASK_ID -- agent", color: muted },
    { at: 112, text: `  ${capture.agent_output}`, color: ink },
    { at: 135, text: `  agent exit ${capture.child_exit_code}`, color: green },
    { at: 180, text: `  verdict: ${capture.verdict}`, color: orange },
    {
      at: 202,
      text: `  hell run exit ${capture.run_exit_code}`,
      color: orange,
    },
    ...capture.attempts.map((attempt, index) => ({
      at: 230 + index * 30,
      text: `  watch → log [${attempt.number}/${capture.max_attempts}] ${attempt.state}`,
      color: orange,
    })),
    { at: 330, text: "$ hell takeover TASK_ID", color: ink },
    { at: 355, text: `  incident: ${capture.takeover.incident}`, color: green },
    { at: 375, text: `  verdict: ${capture.takeover.verdict}`, color: orange },
  ];

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
            {String(stage + 1).padStart(2, "0")} / 04
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
                  color: stage >= 2 ? orange : muted,
                  marginTop: 9,
                }}
              >
                {stage >= 2 ? capture.verdict : "pending"}
              </div>
            </div>
            <div style={{ width: 1, backgroundColor: "#30383b" }} />
            <div>
              <div style={{ fontSize: 16, color: muted, letterSpacing: 2 }}>
                REMINDERS
              </div>
              <div style={{ fontSize: 26, marginTop: 9 }}>
                {attemptCount} / {capture.max_attempts}{" "}
                {stage === 3 ? "· stopped" : ""}
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
                '"SFMono-Regular", Consolas, "Liberation Mono", monospace',
              fontSize: 18,
              lineHeight: 1.8,
            }}
          >
            {frame < 90 ? (
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
            width: `${interpolate(frame, [0, 449], [0, 100])}%`,
          }}
        />
      </div>
    </AbsoluteFill>
  );
};
