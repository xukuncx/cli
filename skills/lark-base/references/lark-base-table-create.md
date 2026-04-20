# base +table-create

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

创建数据表；可选地继续创建字段和视图。

## 推荐命令

```bash
lark-cli base +table-create \
  --base-token app_xxx \
  --name "客户名单"

lark-cli base +table-create \
  --base-token app_xxx \
  --name "项目管理" \
  --fields '[{"name":"项目名称","type":"text"}]' \
  --view '[{"name":"默认表格","type":"grid"}]' 
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--base-token <token>` | 是 | Base Token |
| `--name <name>` | 是 | 新表名称 |
| `--fields <json>` | 否 | 字段 JSON 数组 |
| `--view <json>` | 否 | 视图 JSON 对象或数组 |

## API 入参详情

**HTTP 方法和路径：**

```
POST /open-apis/base/v3/bases/:base_token/tables
```

- 如果传了 `--fields`，CLI 会继续调用字段接口。
- 如果传了 `--view`，CLI 会继续调用视图接口。

## 返回重点

- 至少返回 `table`。
- 传了 `--fields` / `--view` 时，还会附带 `fields` / `views`。

> [!IMPORTANT]
> 如果这次是在刚创建的新 Base 里新增非默认表，返回建表成功后，必须主动澄清默认空表是否保留，例如：`下一步如果你不打算保留初始默认表，我可以继续帮你删掉；要我现在继续吗？`

## 工作流


1. 先只传 `--name` 建空表。
2. 字段或视图参数较复杂时，先精简到最小必需字段，再以内联 JSON 传参。

## 坑点

- ⚠️ 这是写入操作，执行前必须确认。
- ⚠️ CLI 会用 `--fields` 的第一个元素更新系统默认首列，后续元素才是新增字段。
- ⚠️ 不要并行改同一张表，避免状态竞争。

## 参考

- [lark-base-table.md](lark-base-table.md) — table 索引页
- [lark-base-field-create.md](lark-base-field-create.md) — 建字段
- [lark-base-view-create.md](lark-base-view-create.md) — 建视图
