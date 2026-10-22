import assert from "node:assert/strict";
import { spawn, spawnSync } from "node:child_process";
import { once } from "node:events";
import { log } from "node:console";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import process from "node:process";
import { clearTimeout, setTimeout } from "node:timers";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";

// Publish stable CLI facts, never paths, tokens, IDs or timestamps.
const demo = dirname(fileURLToPath(import.meta.url));
const root = resolve(demo, "../..");
const scratch = await mkdtemp(join(tmpdir(), "hell-mode-demo-"));
const binary = join(
  scratch,
  process.platform === "win32" ? "hell.exe" : "hell",
);
let watcher;
let watcherClosed;

function cli(args, expected = 0) {
  const result = spawnSync(
    binary,
    ["--state-dir", join(scratch, "state"), "--json", "--no-input", ...args],
    { cwd: scratch, encoding: "utf8", timeout: 15_000 },
  );
  assert.equal(
    result.status,
    expected,
    `hell ${args[0]}: ${result.stderr || result.error || result.stdout}`,
  );
  const envelope = JSON.parse(result.stdout);
  assert.equal(envelope.schema_version, "hell-mode/v1");
  return envelope;
}

async function waitForAttempts(taskId, count) {
  const deadline = Date.now() + 12_000;
  while (Date.now() < deadline) {
    assert.equal(watcher.exitCode, null, "watchdog exited early");
    const task = cli(["status", taskId]).data;
    if (
      task.incident?.attempts.length >= count &&
      task.incident.attempts.every((attempt) => attempt.state === "simulated")
    )
      return task;
    await delay(50);
  }
  throw new Error("Watchdog did not produce the expected simulated attempts");
}

try {
  const build = spawnSync(
    "go",
    ["build", "-trimpath", "-o", binary, "./cmd/hell"],
    { cwd: root, encoding: "utf8", timeout: 120_000 },
  );
  assert.equal(build.status, 0, build.stderr || String(build.error));
  const task = cli([
    "arm",
    "--goal",
    "Demo: verification must fail",
    "--verify-json",
    JSON.stringify([process.execPath, "-e", "process.exit(1)"]),
    "--deadline",
    "+10m",
    "--interval",
    "2s",
    "--max-attempts",
    "3",
    "--window",
    "1m",
    "--channel",
    "log",
    "--yes",
  ]).data;
  watcher = spawn(
    binary,
    [
      "--state-dir",
      join(scratch, "state"),
      "watch",
      "--interval",
      "100ms",
      "--json",
    ],
    { cwd: scratch, stdio: "ignore" },
  );
  watcherClosed = once(watcher, "close");
  const run = cli(
    [
      "run",
      task.id,
      "--",
      process.execPath,
      "-e",
      'console.log("agent-finished")',
    ],
    3,
  );
  assert.equal(run.error.code, "goal_unmet");
  assert.equal(run.data.child_exit_code, 0);
  assert.equal(run.data.task.verdict, "unmet");
  const agentOutput = (await readFile(run.data.log_path, "utf8")).trim();
  assert.equal(agentOutput, "agent-finished");
  const alarm = await waitForAttempts(task.id, 2);
  assert.equal(alarm.incident.attempts.length, 2);
  const takeover = cli(["takeover", task.id]).data;
  assert.equal(takeover.incident.state, "taken_over");
  assert.equal(takeover.verdict, "unmet");
  assert.ok(takeover.incident.attempts.every((attempt) => attempt.cancelled));
  await delay(2300);
  const stopped = cli(["status", task.id]).data;
  assert.equal(
    stopped.incident.attempts.length,
    2,
    "takeover did not stop reminders",
  );
  const facts = {
    schema_version: "hell-mode/demo-v1",
    channel: task.policy.channel,
    max_attempts: task.policy.max_attempts,
    agent_output: agentOutput,
    child_exit_code: run.data.child_exit_code,
    run_exit_code: 3,
    verdict: run.data.task.verdict,
    attempts: alarm.incident.attempts.map((attempt, index) => ({
      number: index + 1,
      state: attempt.state,
    })),
    takeover: {
      incident: takeover.incident.state,
      verdict: takeover.verdict,
      cancelled_receipts: takeover.incident.attempts.filter(
        (attempt) => attempt.cancelled,
      ).length,
      reminders_after_wait: stopped.incident.attempts.length,
    },
  };
  await writeFile(
    join(demo, "src/capture.json"),
    JSON.stringify(facts, null, 2) + "\n",
  );
  log(
    "Captured: agent exit 0 → unmet → 2 simulated reminders → takeover → reminders stopped; verdict remains unmet.",
  );
} finally {
  if (watcher && watcher.exitCode === null && watcher.signalCode === null) {
    watcher.kill("SIGTERM");
    const timeout = setTimeout(() => watcher.kill("SIGKILL"), 3000);
    try {
      await watcherClosed;
    } finally {
      clearTimeout(timeout);
    }
  }
  await rm(scratch, { recursive: true, force: true });
}
