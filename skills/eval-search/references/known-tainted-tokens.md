# 已知污染文档标记清单

**维护原则**：只加，不删（除非文档被彻底销毁）。每次 v_n 迭代中新增的"评测过程记录"文档都要补进来。

## excluded_user_ids（必须排除的登录账号）

运行 harness 前，`lark-cli auth status.userOpenId` 若命中以下之一，harness 拒绝启动：

```yaml
excluded_user_ids: []
```

真实账号 ID 不提交到仓库。放到本地文件 `skills/eval-search/references/known-tainted-tokens.local.md`，格式与本文件一致；该文件被 `.gitignore` 忽略。

## tainted_tokens（搜索命中这些 token 即标记 contamination_risk）

```yaml
tainted_tokens: []
```

真实飞书 token 不提交到仓库。把当前评测集 base token、历史评测集 token、评测过程文档 token、人工答题参考文档 token 放进本地 `known-tainted-tokens.local.md`。

## 新增条目流程

1. 发现某个飞书文档在评测中被 fetch + 用来作答 → 该文档 tainted
2. 提取 token：
   - docx URL `https://xxx/docx/<token>` → `tainted_tokens` 加 `<token>`
   - wiki URL `https://xxx/wiki/<token>` → `tainted_tokens` 加 `<token>`；**另外**用 `lark-cli wiki +resolve-node --token <token>` 拿到真实 `obj_token`，也加进去
   - base URL 只加 base_token 即可
3. 提交 PR 改本文件（commit message: `chore(eval-search): mark tainted <token> - <reason>`）

## 执行侧处理规则

- Preflight 命中 tainted token 只标记风险，不阻断整轮评测。
- Executor/collector 不能因为命中本文件就隐藏结果；否则评测会被过滤规则美化，不能反映真实搜索行为。
- Executor/collector 必须把命中本文件的结果标为 `tainted=true` / `evidence_excluded=true`。这些结果可以出现在 observed search results 中，但不能进入 evidence candidates、fetch 队列、答案合成或 recall top-5 证据。
- Collector 应把命中的 token 写进 trajectory / raw evidence，保留 `tainted` 这类元数据，交给 Judge 按 RUBRIC 判定污染扣分。
- `verdicts.json` 里只对“fetch 过 tainted token 且答案受其影响”的 case 扣污染分；单纯 search 命中但未 fetch 的 case 不扣污染分，但可以作为污染风险记录。
- 新增 collector、shortcut 或搜索策略时，都要把本文件当作统一标记清单读取，避免各处散落 hard-coded 污染 token。

## 替代策略（推荐）

**不要在飞书上写"评测过程记录" / "v_n 比对分析"之类文档**。都写成本仓库 markdown：

- 评测流程/设计 → `skills/eval-search/**`（已就位）
- 某轮迭代分析 → `tests/eval-search/runs/<run-id>/*.md`（gitignored，本地查看）
- 发布用的 retrospective → PR description / GitHub wiki / release notes

这样根本不会污染飞书搜索语料，污染标记清单的维护压力也会逐渐下降。

## `/eval-search cycle` 的例外

如果用户明确要求把中间结果记录到云文档，允许使用 [`cycle.md`](cycle.md) 的云文档日志，但必须遵守：

1. 云文档只写阶段状态、分数摘要、finding 摘要、PR URL 和本地产物路径；不写标准答案、完整 trajectory、source_urls 或 key_error_snippets
2. 创建或绑定报告文档后，立刻把 doc token 写入本轮 `tests/eval-search/runs/<run-id>/cloud-doc/tainted_tokens.json`
3. 本 cycle 的 regression / after-run 必须把该 token 作为额外污染 token
4. 需要持久 blocklist 时，单独开 `chore(eval-search): blocklist cycle report <run-id>` PR；不得混进搜索策略或能力优化 PR
