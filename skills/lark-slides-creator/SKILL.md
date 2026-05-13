---
name: lark-slides-creator
version: 1.0.0
description: "飞书幻灯片创作工作流：从自然语言需求创建、重构、美化完整 PPT，覆盖规划、模板选择、视觉风格、素材规划和创建后验证。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli slides --help"
---

# slides creator workflow

> 执行 XML/API 前必须读取 ../lark-slides/SKILL.md 和对应 reference。

This skill is the natural-language entry point for creating polished presentations. It owns planning, design, template, asset, and quality-validation workflows. It delegates all XML/API execution to [`../lark-slides/SKILL.md`](../lark-slides/SKILL.md).

## When To Use

Use this skill when the user asks for:

- A new complete presentation from a topic, notes, outline, document, meeting, or rough prompt.
- Beautification, restructuring, major rewrite, or formal-report polishing.
- Template selection or a deck based on a theme, scene, industry, or visual style.
- Visual direction, palette, typography, layout system, or executive-ready presentation quality.
- Asset planning, image search/download/upload planning, or deciding where visuals belong.
- Creation-time and post-creation validation for content completeness and visual quality.

For a narrow raw XML/API operation, use `lark-slides` directly.

## Required Execution Dependency

Before running any `lark-cli slides` command or writing final XML:

1. Read [`../lark-slides/SKILL.md`](../lark-slides/SKILL.md).
2. Read [`../lark-slides/references/xml-schema-quick-ref.md`](../lark-slides/references/xml-schema-quick-ref.md).
3. Read the relevant execution reference, such as `lark-slides-create.md`, `lark-slides-media-upload.md`, `lark-slides-replace-slide.md`, or an `xml_presentation.*` API reference.

Use the execution skill's lint tool from here when XML is available:

```bash
python3 skills/lark-slides-creator/scripts/layout_lint.py --input /tmp/presentation.xml
```

## Workflow

1. Understand the deck goal.
   Capture topic, audience, page count, source material, language, formality, delivery setting, and any brand/style constraints. If the user gives enough information, proceed with explicit assumptions instead of blocking on questions.

2. Choose template or custom direction.
   If the request mentions templates, style, theme, or a common deck scenario, search templates first:

   ```bash
   python3 skills/lark-slides-creator/scripts/template_tool.py search --query "<用户需求原文>" --limit 3
   ```

   Offer 2-3 concise candidates when user choice matters. If one template is clearly best for a lightweight request, state the default and continue unless the user asked to choose.

3. Plan the deck.
   Build a page-by-page outline with title, role, key message, and intended layout for each slide. For formal reports, make the argument flow explicit: context, evidence, analysis, recommendation, next steps.

4. Design the visual system.
   Define palette, typography hierarchy, spacing, page rhythm, chart/table treatment, and recurring elements. Keep slides visual and low-density; do not produce document-like pages.

5. Plan assets.
   Decide which pages need screenshots, photos, diagrams, icons, or charts. External images must become local files first, then execution uses `+media-upload` or `@./path` placeholders as described in `lark-slides`.

6. Generate XML and execute through `lark-slides`.
   Use template summaries or extracted page slices when helpful, but rewrite all placeholder copy into the user's real content. For complex decks, prefer the two-step create flow from `lark-slides`.

7. Validate after creation.
   Read the created presentation XML with `xml_presentations get`, confirm page count and expected content, run lint when possible, then fix issues with `+replace-slide` or raw slide APIs.

## Template Workflow

Template assets live in this skill:

- [`references/template-catalog.md`](references/template-catalog.md)
- [`references/template-index.json`](references/template-index.json)
- [`assets/templates/`](assets/templates/)
- [`scripts/template_tool.py`](scripts/template_tool.py)

Machine-first commands:

```bash
python3 skills/lark-slides-creator/scripts/template_tool.py search --query "工作汇报" --tone light --limit 3
python3 skills/lark-slides-creator/scripts/template_tool.py summarize --template office--work_report --label 内容
python3 skills/lark-slides-creator/scripts/template_tool.py extract --template office--work_report --label 封面 --out /tmp/work-report-cover.xml
```

Rules:

- Search using the user's original wording.
- Show only 2-3 candidate templates unless the user asks for the full catalog.
- Summarize a target page type before extracting XML.
- Do not read entire template XML files by default.
- Reuse theme, spacing, and structure; do not copy placeholder text.

## References

| Reference | Purpose |
| --- | --- |
| [planning-layer.md](references/planning-layer.md) | Deck planning and outline workflow. |
| [visual-planning.md](references/visual-planning.md) | Visual style and layout design guidance. |
| [asset-planning.md](references/asset-planning.md) | Asset selection, local-file, and upload planning. |
| [template-catalog.md](references/template-catalog.md) | Template matching catalog. |
| [slide-templates.md](references/slide-templates.md) | Copyable slide XML patterns for creation. |
| [validation-checklist.md](references/validation-checklist.md) | Creation quality and post-create validation checklist. |
