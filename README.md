# 客户通讯录去重工具

本地命令行工具，把不同渠道导入的联系人整理成可人工审阅的重复候选，确认
后生成保留字段来源的合并档案。仅使用 Go 标准库。

## 运行

```sh
go run ./cmd/contacts -dir ./data
```

`-dir` 为必填的本地数据目录，数据保存在其中的 `contacts_data.json`
（先写临时文件再原子替换）。进程正常退出后重新以同一目录启动即可继续使用。

## 请求/响应格式

标准输入按行读取 JSON，每行一条请求；每行对应一条 JSON 响应。

请求：

```json
{"op": "<操作名>", "payload": { ... }}
```

成功响应：

```json
{"status": "ok", "data": { ... }}
```

错误响应按类型区分：

- `format_error`：JSON 无法解析、缺少字段、参数类型错误。
- `unknown_op`：操作名不存在。
- `rejected`：请求格式正确但被业务规则拒绝（冲突、缺选择、已归档等）。

```json
{"status": "error", "error": "rejected", "message": "..."}
```

## 操作

### import 批量导入

```json
{"op":"import","payload":{"records":[
  {"source":"crm","id":"1001","name":" Alice  Smith ","company":"Acme Corp",
   "email":" Alice@Example.COM ","phone":"+86 138-0000-(1234)"}
]}}
```

- `source`（来源）、`id`（来源内编号）、`name`（姓名）必填；
  `company`、`email`、`phone` 可空。
- `source` + `id` 共同唯一标识一条记录。
- 整批先做完整校验：任一条不合规或同一标识出现字段冲突，整批拒绝、不写入。
- 相同标识且所有原始字段完全一致的重复导入不新增，计入 `duplicate`。
- 响应：`{"received":N,"imported":N,"duplicate":N}`。

### candidates 查找候选

```json
{"op":"candidates","payload":{"source":"crm","min_score":40,"page":1,"page_size":20}}
```

- 仅比较尚未归档的记录；空字段不参与、不加分。
- 得分：邮箱相同 +60，电话相同 +60，姓名相同 +20，公司相同 +20。
- 总分达到 40 才列为候选；仅姓名相同（20 分）不会成为候选。
- 每个字段返回双方原始值与得分（响应中的 `email/phone/name/company`）。
- 每对只出现一次，双方按来源、编号排序后，左侧字典序较小。
- 排序：总分降序，再按左、右双方标识升序。
- `source` 存在时，任一侧来源匹配即保留；`min_score` 低于 40 按 40 处理；
  返回筛选后总数 `total`、当前页 `page` 与 `items`。
- 候选只表示“可能重复”，不会因传递关系（甲≈乙、乙≈丙）自动合并。

### record 查看原文与证据

```json
{"op":"record","payload":{"source":"crm","id":"1001"}}
```

返回该记录的全部原始字段；若已归档，额外返回 `archived_in`（档案编号）。

### merge 确认合并

```json
{"op":"merge","payload":{
  "records":[{"source":"crm","id":"1"},{"source":"shop","id":"2"}],
  "choices":{
    "company":{"source":"crm","id":"1"},
    "email":{"source":"shop","id":"2"}
  }
}}
```

- 至少两条、且全部为尚未归档的记录；缺选择、引用组外记录、重复归入都整次
  拒绝。
- 对姓名、公司、邮箱、电话逐字段处理并忽略空值：
  - 只有一种规范值时，采用来源及编号字典序最小的非空记录的**原文**；
  - 存在多种规范值时，必须在 `choices` 中指定该字段采用哪条字段非空的
    源记录；
  - 全为空的字段在档案中保持为空。
- 响应返回唯一 `archive_id`（如 `A1`，重开后继续递增）、合并后的原文值、
  `sources` 成员列表与 `provenance`：每个非空输出字段记录所选来源记录。

### export 导出

```json
{"op":"export"}
```

返回 `archives`（按档案编号排序）与 `unmerged`（按来源、编号排序的尚未
合并联系人）。已被档案吸收的来源记录不会重复输出。

## 归一化规则

- 姓名、公司：去除首尾空白，连续空白合并为单个空格，比较时忽略大小写。
- 邮箱：去除首尾空白，比较时忽略大小写。
- 电话：仅移除空白、连字符 `-` 和括号 `()`，不猜测或补全国家区号。
- 所有原始值始终保留；归一化只用于比较，档案输出为所选源记录的原文。

## 字段来源查询

- 档案的 `provenance` 列出每个非空字段（`name/company/email/phone`）
  所采用记录的 `source` 与 `id`。
- 用 `record` 操作可按来源与编号取回任意记录的原始内容及所属档案编号。

## 测试

```sh
go test ./...
```

覆盖格式归一、空字段处理、候选排序与分页、导入冲突与重复、冲突字段选择、
重复合并拒绝以及重开后的导出。
