# Judge 打分模板

**使用方式**：主 agent 切 hat 执行。Executor 全部跑完后，主 agent 逐 case 读 `trajectory + expected`，按本文件产出 verdict。

> **隔离纪律**：不要在 Executor 尚未跑完时开始 Judge（会污染 Executor 所在 reasoning 窗口）。Executor 全部完成、`trajectories/*.json` 落盘后再启动 Judge。

---

## Judge 每个 case 的输入

从磁盘读（**不要复用 Executor 的 reasoning context**）：
- `dataset.jsonl` 中该 case 的 `query / involved_entities / filter_keys / expected / source_urls / source_refs / source_info / has_knowledge / rubric_notes`
- `trajectories/<case_id>.json`（含 rounds 列表 + 最终 answer）
- `preflight.json`（看 `contamination_risk` 和 `tainted_tokens`）
- `skills/eval-search/RUBRIC.md`

## 每个 case 的打分步骤

1. **实体路由校验**：先读 `involved_entities`，确认 Executor 使用了对应 domain 的 lark-cli shortcut。任务/联系人/消息/妙记/视频会议/邮箱/日程 case 不能因为没用 `drive +search` 就扣分；反过来，非文档 case 只用 `drive +search` 且没有对应 domain 查询，应在 improvement 中归因到具体实体的 strategy/skill。
2. **recall**：扫 trajectory 里的每一条 tool_use，提取被 fetch / resolve / get / search 命中过的 token、URL、entity id、message_id、open_id、标题和时间，并读取 `evidence_top_results` / search round 里的非污染 evidence tokens。与 `source_urls` 和 `source_refs` 做交集；若没有 URL，则按 `source_refs` / `source_info` / `expected.critical_info` 中的实体名、标题、数量和时间字段判断召回。标记为 `tainted=true` 或 `evidence_excluded=true` 的 search 结果只能算污染观测，不能算 recall top-5 命中。按 RUBRIC 打分
3. **accuracy**：优先把 `answer` 和 `expected.critical_info` 逐条比对；若为空则使用 `expected.key_points`。优先应用 `expected.rubric_notes.可信无误`
4. **completeness**：数 key points 覆盖数。优先应用 `expected.【打分备注】.完整详实`
5. **contamination**：查 trajectory 是否 fetch 过 `preflight.tainted_tokens`；search-only 命中只记录风险，不扣污染分，也不作为 recall/accuracy/completeness 的证据。若有 fetch，按 RUBRIC 给 `contamination_penalty`
6. **improvement 三桶**：从 trajectory 里找失败片段，分类写进 `tool_capability / search_strategy / skill_prompts`，并标注对应实体/domain。

## improvement 填写规则

**每条建议必须满足**：
- 指向**具体文件**（skill_prompts）、**具体命令**（tool_capability）或**具体动作**（search_strategy）
- 引用 trajectory 里触发该建议的 round 序号
- 指向正确实体/domain：任务写 `skills/lark-task/**` 或 `shortcuts/task/**`，消息写 `skills/lark-im/**` 或 `shortcuts/im/**`，邮箱写 `skills/lark-mail/**` 或 `shortcuts/mail/**`，以此类推。除非 `involved_entities` 是文档/云空间/wiki，否则不要把 finding 落到 `lark-drive` 或文档搜索。
- 不写"可以更好"这种无落点的建议；写不出具体落点的建议**丢弃**，不要凑数

**示例**：

✅ 好的：
```json
"skill_prompts": [
  "case_003 involved_entities=任务；round 1 只用了 drive +search，没有调用 task +search 的 assignee/due/completed 过滤。skills/lark-task/references/lark-task-search.md 应补充自然语言任务搜索的过滤映射"
]
```

❌ 差的：
```json
"skill_prompts": [
  "搜索不够全面",
  "agent 应该更聪明地处理 wiki"
]
```

## 合并规则（主 agent 在全部 case 打完后做）

把所有 verdicts 的 `improvement` 按"改动落点文件"去重合并到 `summary.json`：

```json
{
  "run_id": "2026-04-15T10-00Z",
  "dataset_size": 14,
  "scored": 13,
  "contaminated_fetched": 1,
  "totals": {
    "sum": 132,
    "max": 195,
    "percent": 67.7,
    "per_dim": {"recall": 2.69, "accuracy": 3.92, "completeness": 3.54}
  },
  "findings": [
    {
      "finding_id": "F-001",
      "bucket": "skill_prompts",
      "target_domain": "task",
      "target_file": "skills/lark-task/references/lark-task-search.md",
      "suggestion": "补充任务自然语言搜索中 assignee/due/completed 的过滤映射",
      "driving_cases": ["case_003", "case_007", "case_011"],
      "priority": "high"
    },
    {
      "finding_id": "F-002",
      "bucket": "tool_capability",
      "target_domain": "mail",
      "target_file": "shortcuts/mail",
      "suggestion": "mail +triage 缺少按附件类型聚合的输出，agent 难以回答附件类型总结",
      "driving_cases": ["case_001", "case_005"],
      "priority": "medium"
    }
  ],
  "primary_bottleneck": "skill_prompts",
  "pollution_warnings": []
}
```

**priority 判定**：
- `high`: driving_cases ≥3 且 bucket 是 `skill_prompts` / `search_strategy`（改文档成本低、收益面广）
- `medium`: driving_cases ≥2 或 bucket 是 `tool_capability`（代码改动）
- `low`: driving_cases == 1（过拟合风险高，给 Optimizer 作参考但不强推）

## 自我校准检查（写 verdict 前自问）

- 我是不是看了 expected 才倒推 trajectory 合理性？（应该反过来：先看 trajectory 自己是否合理，再 check 是否命中 expected）
- contamination_penalty 有没有漏判？
- improvement 的三桶比例是否均衡到可疑（例如 13 个 case 全扔 `skill_prompts`，可能是判断懒）
