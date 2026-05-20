#!/usr/bin/env node

const fs = require("node:fs");
const path = require("node:path");
const { spawnSync } = require("node:child_process");

function usage() {
  console.log(`Usage:
  node --experimental-strip-types tests/eval-search/eval-search-judge.ts --run-dir <dir>

Reads dataset.jsonl, trajectories/*.json, and preflight.json, then writes:
  verdicts.json
  summary.json

This is the automatic Judge stage for the local harness. It scores only cases
that have expected/source/critical information. Cases without gold data stay in
dataset_size but are excluded from scored totals.`);
}

function parseArgs(argv) {
  const out: any = { runDir: "" };
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

function readJson(file, fallback = null) {
  if (!fs.existsSync(file)) {
    return fallback;
  }
  return JSON.parse(fs.readFileSync(file, "utf8"));
}

function writeJson(file, value) {
  fs.writeFileSync(file, `${JSON.stringify(value, null, 2)}\n`);
}

function loadDataset(file) {
  return fs
    .readFileSync(file, "utf8")
    .split(/\r?\n/)
    .filter((line) => line.trim())
    .map((line, index) => {
      try {
        return JSON.parse(line);
      } catch (err) {
        throw new Error(`cannot parse ${file}:${index + 1}: ${err.message}`);
      }
    });
}

function normalizeText(value) {
  return String(value || "")
    .replace(/[“”]/g, '"')
    .replace(/[‘’]/g, "'")
    .replace(/<[^>]+>/g, " ")
    .replace(/\s+/g, " ")
    .trim();
}

function normalizeNeedle(value) {
  return normalizeText(value).toLowerCase();
}

function addRef(refs, type, value) {
  const text = String(value || "").trim();
  if (!text) {
    return;
  }
  refs.push({ type, value: text });
}

function extractUrlTokens(text) {
  const out = [];
  for (const url of String(text || "").match(/https?:\/\/[^\s)）\]"'<>}]+/g) || []) {
    addRef(out, "url", url.replace(/[.,;，。；|]+$/g, ""));
    const token = url.match(/\/(?:docx|docs|wiki|base|sheets|file|folder|minutes)\/([^/?#]+)/);
    if (token) {
      addRef(out, "token", token[1]);
    }
    const guid = url.match(/[?&]guid=([0-9a-f-]{36})/i);
    if (guid) {
      addRef(out, "task_guid", guid[1]);
    }
  }
  return out;
}

function extractInlineRefs(text) {
  const normalized = normalizeText(text);
  const refs = [];
  const patterns = [
    ["open_id", /\bou_[A-Za-z0-9_]+\b/g],
    ["message_id", /\bom_[A-Za-z0-9_]+\b/g],
    ["chat_id", /\boc_[A-Za-z0-9_]+\b/g],
    ["thread_id", /\bomt_[A-Za-z0-9_]+\b/g],
    ["task_guid", /\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b/gi],
  ];
  for (const [type, pattern] of patterns) {
    for (const match of normalized.matchAll(pattern)) {
      addRef(refs, type, match[0]);
    }
  }
  return refs;
}

function uniqueRefs(refs) {
  const seen = new Set();
  const out = [];
  for (const ref of refs) {
    const key = `${ref.type}:${ref.value}`;
    if (!seen.has(key)) {
      seen.add(key);
      out.push(ref);
    }
  }
  return out;
}

function expectedRefs(item) {
  return uniqueRefs([
    ...(item.source_urls || []).flatMap((url) => extractUrlTokens(url)),
    ...(item.source_refs || []),
    ...extractUrlTokens(item.source_info || ""),
    ...extractInlineRefs(item.source_info || ""),
    ...extractInlineRefs(item.expected?.critical_info || ""),
    ...extractInlineRefs(item.expected?.expected_result || ""),
  ]);
}

function walkStrings(value, out) {
  if (typeof value === "string") {
    out.push(value);
    return;
  }
  if (Array.isArray(value)) {
    for (const item of value) {
      walkStrings(item, out);
    }
    return;
  }
  if (value && typeof value === "object") {
    for (const item of Object.values(value)) {
      walkStrings(item, out);
    }
  }
}

function observedRefs(trajectory) {
  const strings = [];
  walkStrings(
    {
      rounds: trajectory?.rounds || [],
      observed_top_results: trajectory?.observed_top_results || [],
      evidence_top_results: trajectory?.evidence_top_results || [],
      referenced_urls: trajectory?.referenced_urls || [],
      answer: trajectory?.answer || "",
    },
    strings,
  );
  const joined = strings.join("\n");
  return uniqueRefs([...extractUrlTokens(joined), ...extractInlineRefs(joined)]);
}

function refMatch(expected, observed, haystack) {
  const expectedValue = normalizeNeedle(expected.value);
  if (!expectedValue) {
    return false;
  }
  if (observed.some((item) => normalizeNeedle(item.value) === expectedValue)) {
    return true;
  }
  return haystack.includes(expectedValue);
}

function recallScore(expected, observed, trajectory) {
  const evidenceCount = (trajectory?.evidence_top_results || []).length;
  if (expected.length === 0) {
    return { score: evidenceCount > 0 ? 2 : 0, matched: [] };
  }
  const haystackValues = [];
  walkStrings(trajectory || {}, haystackValues);
  const haystack = normalizeNeedle(haystackValues.join("\n"));
  const matched = expected.filter((item) => refMatch(item, observed, haystack));
  const ratio = matched.length / expected.length;
  if (ratio >= 1) return { score: 5, matched };
  if (ratio > 0.5) return { score: 4, matched };
  if (matched.length > 0) return { score: 3, matched };
  if (evidenceCount > 0) return { score: 1, matched };
  return { score: 0, matched };
}

function splitKeyPoints(text) {
  const normalized = normalizeText(text);
  if (!normalized) {
    return [];
  }
  const candidates = normalized
    .split(/[\n。；;]|(?:\d+[.、])/)
    .map((item) => item.trim())
    .filter((item) => item.length >= 4);
  const out = candidates.length > 0 ? candidates : [normalized];
  return out.slice(0, 20);
}

function coverageScore(points, answer) {
  if (points.length === 0) {
    return 0;
  }
  const haystack = normalizeNeedle(answer);
  let matched = 0;
  for (const point of points) {
    const terms = normalizeNeedle(point)
      .split(/\s+/)
      .filter((item) => item.length >= 2);
    const numeric = point.match(/[0-9]+(?:\.[0-9]+)?%?|20[0-9]{2}/g) || [];
    const needles = [...new Set([...terms, ...numeric])].slice(0, 8);
    if (needles.length === 0) {
      continue;
    }
    const hit = needles.filter((item) => haystack.includes(item)).length;
    if (hit >= Math.max(1, Math.ceil(needles.length * 0.35))) {
      matched += 1;
    }
  }
  const ratio = matched / points.length;
  if (ratio >= 0.8) return { score: 5, matched };
  if (ratio >= 0.6) return { score: 4, matched };
  if (ratio >= 0.4) return { score: 3, matched };
  if (ratio >= 0.2) return { score: 2, matched };
  return { score: 0, matched };
}

const DOMAIN_BY_ENTITY = [
  ["任务", "task", "skills/lark-task/references/lark-task-search.md"],
  ["任务清单", "task", "skills/lark-task/references/lark-task-search.md"],
  ["联系人", "contact", "skills/lark-contact/SKILL.md"],
  ["消息", "im", "skills/lark-im/references/lark-im-messages-search.md"],
  ["Bot", "im", "skills/lark-im/references/lark-im-messages-search.md"],
  ["妙记", "minutes", "skills/lark-minutes/SKILL.md"],
  ["视频会议", "vc", "skills/lark-vc/references/lark-vc-search.md"],
  ["邮箱", "mail", "skills/lark-mail/references/lark-mail-triage.md"],
  ["日程", "calendar", "skills/lark-calendar/SKILL.md"],
  ["文档", "drive", "skills/lark-drive/references/lark-drive-search.md"],
  ["云空间", "drive", "skills/lark-drive/references/lark-drive-search.md"],
  ["知识库", "wiki", "skills/lark-wiki/SKILL.md"],
];

function targetInfo(item) {
  for (const entity of item.involved_entities || []) {
    const found = DOMAIN_BY_ENTITY.find((row) => row[0] === entity);
    if (found) {
      return { domain: found[1], file: found[2] };
    }
  }
  return { domain: "drive", file: "skills/lark-drive/references/lark-drive-search.md" };
}

function usedDomains(trajectory) {
  const out = new Set();
  for (const round of trajectory?.rounds || []) {
    const kind = round.tool_kind || "";
    if (kind) {
      out.add(kind);
    }
    const cmd = round.cmd || "";
    for (const [, domain] of DOMAIN_BY_ENTITY) {
      if (cmd.includes(` ${domain} `) || cmd.includes(` ${domain} +`)) {
        out.add(domain);
      }
    }
  }
  return out;
}

function isScorable(item) {
  return Boolean(
    normalizeText(item.expected?.critical_info) ||
      normalizeText(item.expected?.expected_result) ||
      normalizeText(item.expected?.key_points) ||
      normalizeText(item.source_info) ||
      (item.source_urls || []).length > 0 ||
      (item.source_refs || []).length > 0,
  );
}

function judgeCase(item, trajectory, preflight) {
  const target = targetInfo(item);
  if (!isScorable(item)) {
    return {
      case_id: item.case_id,
      record_id: item.record_id || "",
      query: item.query,
      status: "unscored_missing_expected",
      target_domain: target.domain,
      scores: {
        recall: 0,
        accuracy: 0,
        completeness: 0,
        contamination_penalty: 0,
        total: 0,
      },
      rationale: {
        recall: "Skipped: this row has query but no expected/source/critical information.",
        accuracy: "Skipped for missing gold data.",
        completeness: "Skipped for missing gold data.",
      },
      improvement: { tool_capability: [], search_strategy: [], skill_prompts: [] },
      contamination: {
        risk_flagged: Boolean(preflight?.contamination_risk),
        tainted_tokens_fetched: [],
        penalty_applied: 0,
      },
      excluded_from_totals: true,
    };
  }

  const expected = expectedRefs(item);
  const observed = observedRefs(trajectory);
  const recall = recallScore(expected, observed, trajectory || {});
  const answer = normalizeText(trajectory?.answer || "");
  const points = splitKeyPoints(
    item.expected?.critical_info || item.expected?.expected_result || item.expected?.key_points || "",
  );
  const coverage = coverageScore(points, answer);
  const accuracy = Math.min(5, Math.max(coverage.score, recall.score >= 4 ? 4 : recall.score));
  const completeness = coverage.score;
  const taintedFetched = [];
  const fetchedText = normalizeNeedle(JSON.stringify(trajectory?.rounds || []));
  for (const token of preflight?.tainted_tokens || []) {
    if (token && fetchedText.includes(normalizeNeedle(token))) {
      taintedFetched.push(token);
    }
  }
  const contaminationPenalty = taintedFetched.length > 0 ? -1 : 0;
  const total = recall.score + accuracy + completeness + contaminationPenalty;
  const improvements: any = { tool_capability: [], search_strategy: [], skill_prompts: [] };
  const domains = usedDomains(trajectory);
  if ((item.involved_entities || []).length > 0 && !domains.has(target.domain)) {
    improvements.skill_prompts.push(
      `${item.case_id} involved_entities=${(item.involved_entities || []).join(",")}；trajectory 未使用 ${target.domain} domain shortcut。${target.file} 应补充实体路由和自然语言过滤映射。`,
    );
  }
  if (recall.score <= 2) {
    improvements.search_strategy.push(
      `${item.case_id} recall=${recall.score}；应在 ${target.domain} 搜索中优先使用 query 的时间/人员/状态过滤，并用非污染 evidence_top_results 验证目标资源。`,
    );
  }
  if ((trajectory?.evidence_top_results || []).length === 0) {
    improvements.tool_capability.push(
      `${item.case_id} 没有可用 evidence_top_results；${target.domain} search shortcut 需要返回更稳定的标题、id/token、时间和摘要字段。`,
    );
  }

  return {
    case_id: item.case_id,
    record_id: item.record_id || "",
    query: item.query,
    status: "scored",
    target_domain: target.domain,
    target_file: target.file,
    scores: {
      recall: recall.score,
      accuracy,
      completeness,
      contamination_penalty: contaminationPenalty,
      total,
    },
    rationale: {
      recall: expected.length
        ? `Matched ${recall.matched.length}/${expected.length} expected refs from source_urls/source_refs/source_info.`
        : "No explicit source refs; scored from non-tainted evidence availability.",
      accuracy: `Answer/key-point overlap covered ${coverage.matched}/${points.length || 0} key point(s).`,
      completeness: `Completeness follows critical_info/key_points coverage: ${coverage.matched}/${points.length || 0}.`,
    },
    improvement: improvements,
    contamination: {
      risk_flagged: Boolean(preflight?.contamination_risk),
      tainted_tokens_fetched: taintedFetched,
      penalty_applied: contaminationPenalty,
    },
    expected_ref_count: expected.length,
    observed_ref_count: observed.length,
  };
}

function targetFileFor(domain, bucket) {
  const row = DOMAIN_BY_ENTITY.find((item) => item[1] === domain);
  if (!row) {
    return bucket === "tool_capability" ? `shortcuts/${domain}` : "skills/lark-drive/references/lark-drive-search.md";
  }
  return bucket === "tool_capability" ? `shortcuts/${domain}` : row[2];
}

function aggregateFindings(verdicts) {
  const findings = new Map();
  for (const verdict of verdicts) {
    if (verdict.excluded_from_totals) {
      continue;
    }
    for (const bucket of ["tool_capability", "search_strategy", "skill_prompts"]) {
      for (const suggestion of verdict.improvement?.[bucket] || []) {
        const targetFile = targetFileFor(verdict.target_domain, bucket);
        const normalized = suggestion.replace(/^case_\d+\s*/, "").slice(0, 180);
        const key = `${bucket}:${verdict.target_domain}:${targetFile}:${normalized}`;
        if (!findings.has(key)) {
          findings.set(key, {
            finding_id: `F-${String(findings.size + 1).padStart(3, "0")}`,
            bucket,
            target_domain: verdict.target_domain,
            target_file: targetFile,
            suggestion: normalized,
            driving_cases: [],
            priority: "low",
          });
        }
        findings.get(key).driving_cases.push(verdict.case_id);
      }
    }
  }
  return [...findings.values()]
    .map((item) => ({
      ...item,
      priority:
        item.driving_cases.length >= 3 && item.bucket !== "tool_capability"
          ? "high"
          : item.driving_cases.length >= 2 || item.bucket === "tool_capability"
            ? "medium"
            : "low",
    }))
    .sort((a, b) => {
      const rank = { high: 0, medium: 1, low: 2 };
      return rank[a.priority] - rank[b.priority] || b.driving_cases.length - a.driving_cases.length;
    });
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  if (args.help) {
    usage();
    return;
  }
  const root = repoRoot();
  const runDir = path.isAbsolute(args.runDir) ? args.runDir : path.join(root, args.runDir);
  const dataset = loadDataset(path.join(runDir, "dataset.jsonl"));
  const preflightRows = readJson(path.join(runDir, "preflight.json"), []);
  const preflightByCase = new Map((preflightRows || []).map((item) => [item.case_id, item]));
  const oldSummary = readJson(path.join(runDir, "summary.json"), {});
  const verdicts = dataset.map((item) => {
    const trajectory = readJson(path.join(runDir, "trajectories", `${item.case_id}.json`), null);
    return judgeCase(item, trajectory, preflightByCase.get(item.case_id));
  });
  writeJson(path.join(runDir, "verdicts.json"), verdicts);

  const scored = verdicts.filter((item) => !item.excluded_from_totals);
  const totals = scored.reduce(
    (acc, item) => {
      acc.sum += item.scores.total;
      acc.recall += item.scores.recall;
      acc.accuracy += item.scores.accuracy;
      acc.completeness += item.scores.completeness;
      return acc;
    },
    { sum: 0, recall: 0, accuracy: 0, completeness: 0 },
  );
  const findings = aggregateFindings(verdicts);
  const bucketCounts = findings.reduce((acc, item) => {
    acc[item.bucket] = (acc[item.bucket] || 0) + item.driving_cases.length;
    return acc;
  }, {});
  const primaryBottleneck =
    Object.entries(bucketCounts).sort((a: any, b: any) => b[1] - a[1])[0]?.[0] || null;
  const contaminationFetched = verdicts.filter((item) => item.contamination?.tainted_tokens_fetched?.length > 0).length;
  const summary = {
    ...oldSummary,
    run_id: oldSummary.run_id || path.basename(runDir),
    status: "scored",
    dataset_size: dataset.length,
    scored: scored.length,
    unscored_missing_expected: verdicts
      .filter((item) => item.excluded_from_totals)
      .map((item) => item.case_id),
    contaminated_fetched: contaminationFetched,
    contaminated_skipped: oldSummary.contaminated_skipped || 0,
    parse_error_cases: oldSummary.parse_error_cases || [],
    primary_bottleneck: primaryBottleneck,
    totals: {
      sum: totals.sum,
      max: scored.length * 15,
      percent: scored.length ? Number(((totals.sum / (scored.length * 15)) * 100).toFixed(1)) : null,
      per_dim: {
        recall: scored.length ? Number((totals.recall / scored.length).toFixed(2)) : null,
        accuracy: scored.length ? Number((totals.accuracy / scored.length).toFixed(2)) : null,
        completeness: scored.length ? Number((totals.completeness / scored.length).toFixed(2)) : null,
      },
    },
    findings,
    blockers: [],
  };
  writeJson(path.join(runDir, "summary.json"), summary);
  console.log(
    JSON.stringify(
      {
        run_id: summary.run_id,
        status: summary.status,
        run_dir: path.relative(root, runDir),
        dataset_size: summary.dataset_size,
        scored: summary.scored,
        unscored_missing_expected: summary.unscored_missing_expected.length,
        total_percent: summary.totals.percent,
        findings: summary.findings.length,
        primary_bottleneck: summary.primary_bottleneck,
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
