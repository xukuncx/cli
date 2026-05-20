#!/usr/bin/env node

const path = require("node:path");
const fs = require("node:fs");
const { spawnSync } = require("node:child_process");

function usage() {
  console.log(`Usage:
  node --experimental-strip-types tests/eval-search/eval-search-cycle.ts --run-dir <dir> [options]

Options:
  --skip-optimizer                  run Judge only
  --skip-pr                         do not push/create PR
  --skip-gate                       skip make unit-test in PR worktree
  --optimizer-mode <codex|draft-only>

Runs the post-executor stages: Judge -> Optimizer -> draft PR.`);
}

function parseArgs(argv) {
  const out: any = {
    runDir: "",
    skipOptimizer: false,
    skipPr: false,
    skipGate: false,
    optimizerMode: "codex",
  };
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    const next = () => {
      if (i + 1 >= argv.length) {
        throw new Error(`missing value for ${arg}`);
      }
      i += 1;
      return argv[i];
    };
    if (arg === "--help" || arg === "-h") {
      out.help = true;
    } else if (arg === "--run-dir") {
      out.runDir = next();
    } else if (arg === "--skip-optimizer") {
      out.skipOptimizer = true;
    } else if (arg === "--skip-pr") {
      out.skipPr = true;
    } else if (arg === "--skip-gate") {
      out.skipGate = true;
    } else if (arg === "--optimizer-mode") {
      out.optimizerMode = next();
    } else {
      throw new Error(`unknown option ${arg}`);
    }
  }
  if (!out.help && !out.runDir) {
    throw new Error("--run-dir is required");
  }
  return out;
}

function repoRoot() {
  const result = spawnSync("git", ["rev-parse", "--show-toplevel"], {
    encoding: "utf8",
  });
  if (result.status !== 0) {
    throw new Error("must run inside a git worktree");
  }
  return result.stdout.trim();
}

function runNode(script, args, root) {
  const result = spawnSync(
    "node",
    ["--experimental-strip-types", script, ...args],
    {
      cwd: root,
      encoding: "utf8",
      maxBuffer: 128 * 1024 * 1024,
      timeout: 120 * 60 * 1000,
      stdio: ["ignore", "pipe", "pipe"],
    },
  );
  process.stdout.write(result.stdout || "");
  process.stderr.write(result.stderr || "");
  if (result.status !== 0) {
    throw new Error(`${script} exited ${result.status}`);
  }
}

function readJson(file, fallback = null) {
  if (!fs.existsSync(file)) {
    return fallback;
  }
  return JSON.parse(fs.readFileSync(file, "utf8"));
}

function writeJson(file, value) {
  fs.writeFileSync(file, `${JSON.stringify(value, null, 2)}\n`);
}

function updateCycle(runDir, patch) {
  const file = path.join(runDir, "cycle.json");
  const current = readJson(file, {});
  writeJson(file, { ...current, ...patch, updated_at: new Date().toISOString() });
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  if (args.help) {
    usage();
    return;
  }
  const root = repoRoot();
  const runDir = path.isAbsolute(args.runDir) ? args.runDir : path.join(root, args.runDir);
  updateCycle(runDir, { status: "judge_running" });
  runNode("tests/eval-search/eval-search-judge.ts", ["--run-dir", runDir], root);
  updateCycle(runDir, { status: "judge_finished" });

  if (!args.skipOptimizer) {
    updateCycle(runDir, { status: "optimizer_running" });
    const proposeArgs = ["--run-dir", runDir, "--optimizer-mode", args.optimizerMode];
    if (args.skipPr) {
      proposeArgs.push("--skip-pr");
    }
    if (args.skipGate) {
      proposeArgs.push("--skip-gate");
    }
    runNode("tests/eval-search/eval-search-propose-pr.ts", proposeArgs, root);
    updateCycle(runDir, { status: "finished" });
  } else {
    updateCycle(runDir, { status: "finished", optimizer_skipped: true });
    const summaryFile = path.join(runDir, "summary.json");
    const summary = readJson(summaryFile, {});
    writeJson(summaryFile, { ...summary, pr_status: "optimizer_skipped", pr_urls: [] });
  }

  const summary = readJson(path.join(runDir, "summary.json"), {});
  const prStatus = readJson(path.join(runDir, "pr-draft", "status.json"), {});
  const prStatusValue = args.skipOptimizer
    ? summary.pr_status || "optimizer_skipped"
    : prStatus.status || summary.pr_status || "not_requested";
  console.log(
    JSON.stringify(
      {
        status: "finished",
        run_id: summary.run_id || path.basename(runDir),
        run_dir: path.relative(root, runDir),
        dataset_size: summary.dataset_size,
        scored: summary.scored,
        total_percent: summary.totals?.percent,
        findings: summary.findings?.length || 0,
        pr_status: prStatusValue,
        pr_urls: summary.pr_urls || [],
      },
      null,
      2,
    ),
  );
}

try {
  main();
} catch (err) {
  console.error(err.stack || err.message);
  process.exitCode = 1;
}
