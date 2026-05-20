# 评测集 schema + 拉取方式

## 位置

评测集存在飞书多维表格（**live 数据源**，PM 持续更新）：

- base_token: 由本地 `EVAL_SEARCH_BASE_TOKEN` 或命令行 `--base-token` 提供
- table_id: 由本地 `EVAL_SEARCH_TABLE_ID` 或命令行 `--table-id` 提供
- view_id: 由本地 `EVAL_SEARCH_VIEW_ID` 或命令行 `--view-id` 提供
- URL: 不写入仓库；保存在本地运行记录或私有上下文中

> **污染警告**：这个 base 本身会被 `drive +search` 命中。harness 必须把账号拆成两个 profile：loader profile 只用于读取这个 base 并生成 `dataset.jsonl`；executor profile 只用于盲测搜索，**不可**加入该 base 的查看权限，否则评测结果被自答污染。详见 [`pollution-preflight.md`](pollution-preflight.md)。

## 原始字段（字段 id → 含义）

| 字段名 | 类型 | 说明 |
|--------|------|------|
| `Query内容` | text | 自然语言问题；Executor 唯一可见输入 |
| `是否采纳` | single-select | 只运行包含 `采纳` 的记录；空值、`待定`、`不采纳` 都不进入评测 |
| `类别` | select | 例如 `搜索+总结类`；Judge / report 可见，Executor 不可见 |
| `涉及筛选项` | text | 逗号分隔的过滤字段，如 `creator_ids,is_completed,due_time`；deterministic collector 可用于盲测路由，Executor prompt 不注入 |
| `涉及实体` | multi-select | 任务、联系人、消息、妙记、视频会议、邮箱、文档、日程等；用于实体路由和 Judge 归因，Executor prompt 不注入 |
| `预期结果` | text | Judge 独占使用，**Executor 不可见** |
| `数据信息` | text（可能含 markdown 链接） | expected source URLs / entity 线索；Judge 独占使用，**Executor 不可见** |
| `关键信息` | text | Judge 独占使用，写入 `expected.critical_info`，并拼入 `expected.key_points` 作为主要评分要点 |

> **新列提示**：runner 会把 Base 返回但尚未映射的字段写入 `meta.json.unhandled_fields` / `summary.json.dataset_warnings`，并在 stdout 里打印 `unhandled_fields`。看到新列时先问用户是否需要处理；用户确认后再把它加入转换逻辑。

## 拉取命令

推荐用确定性 setup runner 拉取并转换：

```bash
node --experimental-strip-types tests/eval-search/eval-search-run.ts \
  --loader-profile <base-reader> \
  --executor-profile <blind-runner> \
  --subset 3
```

如果只有一个账号，可以拆成两步：

```bash
# 账号仍有评测 Base 权限时，只拉本地快照
node --experimental-strip-types tests/eval-search/eval-search-run.ts \
  --snapshot-only \
  --loader-profile <base-reader>

# 移除该账号的评测 Base 权限后，从本地快照继续盲测 setup
node --experimental-strip-types tests/eval-search/eval-search-run.ts \
  --dataset-file tests/eval-search/runs/<snapshot-run>/dataset.jsonl \
  --executor-profile <blind-runner>
```

只看原始 Base 拉取时，用 loader profile 执行：

```bash
lark-cli --profile <base-reader> base +record-list \
  --as user \
  --base-token "$EVAL_SEARCH_BASE_TOKEN" \
  --table-id "$EVAL_SEARCH_TABLE_ID" \
  --view-id "$EVAL_SEARCH_VIEW_ID" \
  --limit 100
```

返回形如：
```json
{
  "ok": true,
  "data": {
    "data": [ [value_of_query, value_of_len, ...], ... ],
    "field_id_list": ["fldh3DHP53", ...],
    "fields": ["Query内容", "是否采纳", "类别", "涉及筛选项", "涉及实体", "预期结果", "数据信息", "关键信息"],
    "record_id_list": ["recvg4qIXMSU6K", ...],
    "has_more": true
  }
}
```

若 `has_more=true`，用 `--offset` 翻页直到全部拉完。

## 转换为 harness 内部 schema

主 agent 把每一行转成一个 case 对象，拼成 `dataset.jsonl`（jsonl，一行一个 case）：

```json
{
  "case_id": "case_001",
  "record_id": "recvg4qIXMSU6K",
  "query": "帮我总结一下分配给我的所有任务，按紧急程度排个序，看看哪些需要优先处理",
  "has_knowledge": true,
  "expected": {
    "key_points": "关键信息 + 预期结果原文",
    "critical_info": "关键信息原文",
    "critical_info_warnings": [],
    "expected_result": "预期结果原文",
    "aux_info": "类别 / 涉及筛选项 / 涉及实体 / 数据信息",
    "rubric_notes": {}
  },
  "category": ["搜索+总结类"],
  "filter_keys": ["assignee_ids", "due_time", "is_completed"],
  "involved_entities": ["任务"],
  "source_info": "数据信息原文",
  "source_urls": [
    "https://applink.feishu.cn/client/todo/detail?guid=..."
  ],
  "source_refs": [
    {"type": "open_id", "value": "ou_xxx"},
    {"type": "message_id", "value": "om_xxx"}
  ]
}
```

### 转换要点

1. **case_id 编号**：按 record_id 在返回里的顺序分配 `case_001, case_002, ...`。同一次 run 内稳定，跨 run 不保证（PM 在 base 里插新行会错位）。如需跨 run 追踪，用 `record_id`
2. **filter `是否采纳`**：只保留包含 `采纳` 的记录。空值不是默认采纳；如果 Base 全表有 100+ 行但 `采纳` 只有 19 行，本轮评测就是 19 条
3. **解析 `涉及实体` / `涉及筛选项`**：保留到 `involved_entities` / `filter_keys`，用于 deterministic collector 实体路由和 Judge 归因；不要注入给 AI Executor prompt
4. **解析 `关键信息` / `预期结果` / `数据信息`**：`关键信息` 写入 `expected.critical_info`，`预期结果` 写入 `expected.expected_result`，两者按"关键信息 → 预期结果"顺序拼入 `expected.key_points`；`数据信息` 写入 `source_info`
5. **解析 evidence 线索**：从 `预期结果` / `关键信息` / `数据信息` 中提取 URL 到 `source_urls`；同时提取非 URL 标识到 `source_refs`，例如 `open_id`、`message_id`、`chat_id`、`thread_id`、`task_guid`。某些消息 / 联系人 / 邮箱 case 没有 URL，Judge 必须结合 `source_refs`、`source_info` 和 key points 评分
6. **`关键信息` 质量诊断**：runner 会把空值、过短、硬编码日期、主观阈值、括号/引号不配对、常见错别字等写到 `expected.critical_info_warnings`，并在 `meta.json.critical_info_warnings` 汇总。该诊断只提示人工优化，不直接影响分数
7. **空 query 过滤**：`Query内容` 字段为空或纯空白的记录跳过

## Pilot 样本：只跑前 3 条冒烟

`/eval-search run --subset 3` 只拉前 3 条 `是` 类 case 跑。用于：
- 第一次落地 harness，验证端到端能跑通
- auto-PR 流程的 dry-run（改完 skill 跑 3 条看趋势）

## 频率 / 数据漂移

PM 在 base 里编辑 case 是常态。harness 不做 snapshot 冻结（v0.1 范围外），每次 `run` 拉最新。

**代价**：v_n 和 v_{n+1} 的分数差会混入 dataset 变化。在 PR description 里强制标注 `dataset_size / first_run_of_records` 两个字段，reviewer 自己判断。
