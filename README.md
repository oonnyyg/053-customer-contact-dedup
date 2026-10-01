# 客户通讯录去重工具

本地通讯录去重命令行工具：逐行 JSON（JSON Lines）导入不同渠道的联系人，
计算可能重复的候选对，人工确认后生成保留字段来源的合并档案。
仅使用 Go 标准库，数据保存在指定本地目录，正常退出后重开可继续使用。

## 运行

```
go run ./cmd/contacts -dir ./contacts-data < requests.jsonl
go build -o contacts ./cmd/contacts && ./contacts -dir /path/to/data
```

- `-dir`：数据目录（默认 `./contacts-data`），不存在会自动创建。
- 数据落盘为目录内的 `contacts-db.json`，采用临时文件 + 原子替换写入。
- 标准输入每行一条请求，标准输出每行一条响应，请求与响应一一对应。

## 请求格式

所有请求都带 `op` 字段。

### import（批量导入）

```json
{"op":"import","records":[
  {"source":"crm","id":"1001","name":" Alice  Wang ","company":" Acme Corp","email":"Alice@X.com","phone":"+86 138-0000-1234"}
]}
```

- `source`（来源）、`id`（来源内编号）、`name`（姓名）必填；
  `company`、`email`、`phone` 可空。
- `source` 与 `id` 共同唯一标识一条记录。
- 整批先完整校验，任一条不合法或与已有记录内容冲突，则整批拒绝、不写入任何记录。
- 相同标识且各原始字段完全相同的重复导入会被跳过（`skipped`），不新增记录。
- 响应：`{"ok":true,"type":"import_result","import_result":{"received":N,"added":N,"skipped":N}}`

### candidates（查找候选）

```json
{"op":"candidates","filter":{"source":"crm","min_score":60,"page":1,"page_size":20}}
```

- `source`：只返回任一侧来自该来源的候选；可省略。
- `min_score`：最低总分，低于 40 时按 40 处理。
- `page` 从 1 开始，`page_size` 默认 20；响应含筛选后 `total`。
- 响应 `type` 为 `candidates`，每项含 `left`、`right`、`total` 和 `scores`
  （每个命中字段的 `field`、`points` 及双方原值 `left_value`/`right_value`）。

### get_record（查看原文与证据）

```json
{"op":"get_record","record_id":{"source":"crm","id":"1001"}}
```

- 返回记录全部原始字段 `record`、`archived`、`profile_id`。
- 已归档记录附带 `evidence`：逐字段列出原值及是否被档案采用（`used`），
  档案编号见 `profile_id`。即使字段未被采用，原始信息仍可查询。

### merge（确认合并）

```json
{"op":"merge","merge":{
  "sources":[{"source":"crm","id":"1001"},{"source":"web","id":"77"}],
  "choices":{"name":{"source":"crm","id":"1001"},"phone":{"source":"web","id":"77"}}
}}
```

- `sources` 至少两条，且必须都是存在且尚未归档的记录；不得重复列出。
- 每个字段（`name`/`company`/`email`/`phone`）忽略空值：
  - 非空记录只有一种规范值：自动采用来源与编号字典序最小记录的原文。
  - 存在多种规范值：必须在 `choices` 中指定采用哪条该字段非空的组内源记录。
  - 全部为空：输出字段保持为空。
- 缺少选择、选择引用组外记录或引用该字段为空的记录，整次拒绝。
- 每个非空输出字段在响应 `profile.fields` 中记录来源，例如
  `"fields":{"name":{"source":"crm","id":"1001"}}`。
- 成功后源记录立即归档，不能再归入其他档案；候选也不再包含它们。
- 档案编号形如 `P0001`，在合并成功时才分配，保证唯一且单调递增。

### export（导出）

```json
{"op":"export"}
```

- 返回 `profiles`（按档案编号排序）和 `unmerged`（尚未合并联系人，按来源、编号排序）。
- 已合并的源记录不会在 `unmerged` 中重复出现。

## 字段规范化与匹配规则

规范化仅用于比较，原文始终原样保存：

- 姓名、公司：去除首尾空白、内部连续空白合并为单个空格、忽略大小写。
- 邮箱：去除首尾空白、忽略大小写（不处理内部空白）。
- 电话：只移除空白、连字符和括号，不猜测国家区号。

仅在尚未合并的记录之间，逐对比较双方都非空的规范值：

| 字段 | 得分 |
| --- | --- |
| 邮箱相同 | 60 |
| 电话相同 | 60 |
| 姓名相同 | 20 |
| 公司相同 | 20 |

- 总分达到 40 才列为候选；空字段不加分；仅姓名相同（20 分）不会成候选。
- 每对只返回一次：先把双方按来源、编号字典序排列为 left/right，
  再按总分降序、left 标识、right 标识排序。
- 候选只提示“可能重复”：甲=乙、乙=丙不会自动把三者合并，必须人工分别确认。

## 错误区分

错误响应：`{"ok":false,"error_code":"...","error":"..."}`

- `format_error`：JSON 格式错误或未知操作（缺少业务参数时也用此码）。
- `invalid_request`：请求语义不合法（必填缺失、记录不存在、选择引用组外记录等）。
- `business_rejected`：业务规则拒绝（内容冲突整批拒绝、字段多值缺选择、源记录已归档）。

## 测试

```
go test ./...
```

覆盖：格式归一化、空字段不加分、候选排序/并列排序/筛选/分页、
导入整批校验与冲突回滚、冲突字段选择、单规范值自动取字典序最小来源、
重复合并拒绝、档案编号递增、重开后导出及字段来源证据查询。
