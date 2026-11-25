---
type: reference
purpose: 在写实现前定稿阶段 1 可执行安装 plan 的最小实例 schema、操作格式与项目事实保护边界
audience: 项目负责人、阶段 1 Executor、人工 Reviewer 与维护 aigov plan/apply 的后续 Agent
status: active
---

# PLAN-002-R1：可执行安装 Plan 人工定稿草案

## 用途

`T-PLAN-001` 只完成严格 manifest 校验与生命周期跳过；`T-CONTRACT-002` 又补齐了确定的显式 `instance_id`。在进入写入型 `apply` 前，本任务只让只读 `plan` 输出可由人工审核、由未来 apply 重新计算的目标文件操作。项目负责人已于 2026-11-25 回复“批准”，修订稿经独立复审 PASS，现按 contract-first 顺序执行。

## 已批准且不得扩大的决定

### 1. 本切片只计划四个目标

| operation `logical_id` | effective path | 所有权 | 本切片行为 |
|---|---|---|---|
| `agent_entry` | effective `agent_entry` | `project` | 不存在时 `CREATE`；已有普通文件时 `NOOP project_fact_preserved`，永不覆盖或用模板判冲突 |
| `instance_manifest` | effective `instance_manifest` | `tool` | 不存在时 `CREATE`；字节一致时 `NOOP managed_content_matches`；不一致时 `CONFLICT managed_content_differs` |
| `schema_root/install-manifest.schema.json` | effective `schema_root` + 固定叶名 | `tool` | 同上 |
| `schema_root/instance.schema.json` | effective `schema_root` + 固定叶名 | `tool` | 同上 |

`project_decisions` 与 `design_spec` 只在生命周期尚未满足时输出 `SKIP`。selection 未定稿时 `project_decisions` 为 `lifecycle_not_reached`；`selection_finalized=true` 时本切片不管理该 project fact，因此从 operations 省略，contract test 必须断言不存在该 logical ID。无前端时 `design_spec` 为 `no_frontend`；有前端但 manifest 没有设计事实时为 `design_facts_not_provided`。本切片不计划 runtime、phase/maturity/policy/trust、任务证据目录或其他 schema。

### 2. 目标路径、文件类型与 containment

四个写目标和两个条件资源从全部 17 个 effective resource path 派生并统一规范化，再做碰撞检查。为保证同一 manifest 跨平台得到相同结果，所有宿主都按 Windows ASCII 大小写折叠口径检查；任意最终文件路径相同、折叠后相同，或一个最终文件路径是另一最终文件路径的父路径，都返回 `E_RESOURCE_COLLISION`。每组碰撞按 base resource logical ID 排序：两个都显式时保留较小 ID、诊断指向较大 ID；只有一个显式时诊断始终指向该显式 `/layout/resources/<escaped-id>`；同一 Pointer 只报一次。不得输出绝对路径。至少覆盖 `agent_entry = schema_root/instance.schema.json` 和 `instance_manifest = schema_root`。

目标探测固定如下：

1. `project.root` 先解析为 canonical root。每个目标保持项目相对路径，逐级 `lstat`；最近已存在祖先经 realpath 后必须仍位于 canonical root 内。
2. 任一已存在父组件是 symlink、Windows junction/reparse point、普通文件或其他非目录对象，分别返回 `E_TARGET_ESCAPE` 或 `E_TARGET_TYPE`，退出 2；不得跟随到项目外。
3. 叶目标不存在时可以 `CREATE`；存在时必须是可读取普通文件。symlink/junction、broken link、目录和特殊文件返回 `E_TARGET_TYPE`；读取失败返回 `E_TARGET_READ`。诊断 path 固定为 `/targets/<escaped-logical-id>`。
4. 一项 target safety 诊断抑制该目标 operation；任一 validation/target diagnostic 都使 stdout 为空、stderr 仅输出稳定 JSONL、退出 2。未来 apply 必须在写入前重复相同检查；本任务仍为零写入。
5. contract tests 覆盖项目外 symlink/junction、broken link、目录、特殊文件、不可读文件和缺失父目录；平台无法构造的权限或 reparse fixture必须明确 skip 原因。

新诊断按现有规则以 path、再以 code 升序排列。任一 manifest validation 或 resource collision 出现时抑制全部 target probe；同一 target 只报告最先遇到的祖先/叶错误，`E_TARGET_ESCAPE`/`E_TARGET_TYPE` 抑制其后 `E_TARGET_READ`。message 精确固定为：

| code | message |
|---|---|
| `E_TARGET_ESCAPE` | `use a target path whose existing ancestors stay inside project root` |
| `E_TARGET_TYPE` | `replace target or parent with the required regular file or directory type` |
| `E_TARGET_READ` | `make target readable before planning` |

### 3. instance schema 与精确字节

新增 `tool/contracts/v0/instance.schema.json`。文件固定为 UTF-8、无 BOM、以下单行 JSON 加一个 LF；SHA-256 固定为 `6dfe34eda589b1c8a948669fef164c60ff7272b5ed65eeb05f47af669bf559af`：

```json
{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://aigov.dev/contracts/v0/instance.schema.json","title":"aigov instance manifest 0.1-draft","type":"object","additionalProperties":false,"required":["schema_version","instance_id","install_contract_version","methodology_version","assurance_level","resources","managed_files"],"properties":{"schema_version":{"const":"0.1-draft"},"instance_id":{"type":"string","pattern":"^inst-[A-Za-z0-9][A-Za-z0-9._-]{2,122}$"},"install_contract_version":{"const":"0.2-draft"},"methodology_version":{"const":"0.1.0-dev"},"assurance_level":{"const":"declarative"},"resources":{"type":"object","additionalProperties":false,"required":["agent_entry","design_spec","project_decisions","instance_manifest","phase_snapshot","maturity_fact_store","maturity_snapshot","policy_snapshot","trust_anchor","runtime_root","schema_root","task_store","evidence_store","review_store","task_state_cache","task_audit_store","phase_audit_store"],"properties":{"agent_entry":{"type":"string"},"design_spec":{"type":"string"},"project_decisions":{"type":"string"},"instance_manifest":{"type":"string"},"phase_snapshot":{"type":"string"},"maturity_fact_store":{"type":"string"},"maturity_snapshot":{"type":"string"},"policy_snapshot":{"type":"string"},"trust_anchor":{"type":"string"},"runtime_root":{"type":"string"},"schema_root":{"type":"string"},"task_store":{"type":"string"},"evidence_store":{"type":"string"},"review_store":{"type":"string"},"task_state_cache":{"type":"string"},"task_audit_store":{"type":"string"},"phase_audit_store":{"type":"string"}}},"managed_files":{"type":"object","additionalProperties":false,"required":["schema_root/install-manifest.schema.json","schema_root/instance.schema.json"],"properties":{"schema_root/install-manifest.schema.json":{"type":"string","pattern":"^[0-9a-f]{64}$"},"schema_root/instance.schema.json":{"type":"string","pattern":"^[0-9a-f]{64}$"}}}}}
```

instance manifest 使用紧凑 JSON、无 BOM、末尾一个 LF。根键顺序固定为 `schema_version`、`instance_id`、`install_contract_version`、`methodology_version`、`assurance_level`、`resources`、`managed_files`；两个对象内部按 key 的 UTF-8 字节序排列。字符串使用 JSON 的 `\"`、`\\`、`\b`、`\f`、`\n`、`\r`、`\t` 和小写 `\u00xx` 控制字符转义；非 ASCII 直接编码 UTF-8，但 U+2028/U+2029 固定为 `\u2028`/`\u2029`；`<`、`>`、`&` 不转义。摘要包含末尾 LF。

`resources` 恰含 03 的 17 个 effective normalized path；`managed_files` 恰含两个 schema 目标原始字节 SHA-256，不含自身或 project-owned 文件。`assurance_level` 固定 `declarative`。`methodology_version` 复用 CLI 的单一版本源 `0.1.0-dev`，禁止复制第二个可漂移常量。

对 canonical fixture 和当前 install schema SHA-256 `490a29358da5d1ad678effe58635fab5af88cf9693bf0f246d7b30a88abb4ed1`，完整 instance 字节为：

```json
{"schema_version":"0.1-draft","instance_id":"inst-example-01","install_contract_version":"0.2-draft","methodology_version":"0.1.0-dev","assurance_level":"declarative","resources":{"agent_entry":"AGENTS.md","design_spec":"DESIGN.md","evidence_store":"governance/evidence","instance_manifest":".ai-governance/instance.json","maturity_fact_store":"governance/maturity/facts","maturity_snapshot":".ai-governance/maturity.json","phase_audit_store":"governance/audit/phase","phase_snapshot":".ai-governance/phase.json","policy_snapshot":".ai-governance/policy.json","project_decisions":"docs/decisions.md","review_store":"governance/reviews","runtime_root":".ai-governance/runtime","schema_root":".ai-governance/schemas","task_audit_store":"governance/audit/tasks","task_state_cache":"governance/state","task_store":"governance/tasks","trust_anchor":".ai-governance/trust.json"},"managed_files":{"schema_root/install-manifest.schema.json":"490a29358da5d1ad678effe58635fab5af88cf9693bf0f246d7b30a88abb4ed1","schema_root/instance.schema.json":"6dfe34eda589b1c8a948669fef164c60ff7272b5ed65eeb05f47af669bf559af"}}
```

该字节串（含末尾 LF）SHA-256 固定为 `d2334d08bc2f9fec68d66b5f3af59e213d0f518e940057b194de2f25432bee3b`。

Schema contract tests 不引入第三方 evaluator：先对 schema 文档自身做严格 JSON 解析和固定结构断言，再由独立表驱动 validator 对 instance fixture执行同一约束。负向矩阵至少含每个根必填缺失、根/resources/managed_files 未知字段、三处版本/assurance const 错误、instance ID pattern、17 个资源缺失、资源越界/未规范化/碰撞、managed key 缺失/多余、摘要非 64 位小写 hex 及跨资源引用失效。

### 4. 新建 agent_entry 的固定字节

仅在目标不存在时生成下列 LF/UTF-8/末尾单 LF 模板。`<project-facts-json>` 使用与 instance 相同的 JSON escaping，键顺序固定为 `core_hypothesis`、`instance_id`、`name`、`phase`、`technology`；整体前置四个 ASCII 空格。`<>&` 保持 UTF-8，U+2028/U+2029 转义，换行、引号、反斜线和控制字符不能逃逸该单行 JSON。

```text
---
type: reference
purpose: 目标项目的 AI Agent 本地启动入口与项目事实摘要
audience: 参与目标项目的 AI Agent 与人类协作者
status: active
---

# Target project AI entry

Read this file first, then the instance manifest at:

    <instance-path-json>

## Project facts

    <project-facts-json>

## Working boundary

Follow the recorded phase and repository-local task constraints. Do not treat generated tool files as authority over confirmed project facts.
```

`<instance-path-json>` 是 effective `instance_manifest` path 的 JSON 字符串，使用同一 escaping 并缩进四个空格；因此反引号、换行和 Markdown 字符不能改变文档结构。canonical 默认路径对应行是 `    ".ai-governance/instance.json"`，完整模板 SHA-256 为 `08958b0e158adf4141fa7219e67146e439f476f240699306bbbfe3d18435ad0a`。文件一旦存在即成为 project-owned fact；后续 plan 只摘要实际字节并 `NOOP project_fact_preserved`。自定义路径必须有独立 golden。

### 5. 确定性 plan 协议

stdout 是单行 UTF-8 JSON 加一个 LF；顶层键顺序固定为 `contract_version`、`instance_id`、`operations`。`contract_version` 回显安装输入契约 `0.2-draft`，不是独立 plan schema 版本。每个 operation 固定七个有序键：`action`、`logical_id`、`path`、`ownership`、`expected_sha256`、`actual_sha256`、`reason`；缺省 hash 为 JSON `null`。

- operations 按 normalized `path`、再按 `logical_id` 的 UTF-8 字节序排列，只输出 `/` 分隔的项目相对路径。
- `CREATE`：expected 必填、actual null、`target_missing`。
- tool `NOOP`：expected/actual 相等、`managed_content_matches`。
- project `NOOP`：expected null、actual 为实际摘要、`project_fact_preserved`。
- `SKIP`：ownership=`project`，两个 hash 均 null，path 是 effective conditional resource path。
- `CONFLICT`：expected/actual 均必填且不同、`managed_content_differs`。
- 无 conflict 退出 0；任一 conflict 仍输出完整 plan、stderr 为空、退出 2。validation/target diagnostic 则不输出 plan，只写 stderr JSONL 并退出 2。
- 03 的 `NO_CHANGES` 仅是未来 apply 第二次不写入的完成语义；审核型 plan 始终保留 NOOP/SKIP operations，不输出裸 `NO_CHANGES`。

canonical blank target 的完整 stdout 为：

```json
{"contract_version":"0.2-draft","instance_id":"inst-example-01","operations":[{"action":"CREATE","logical_id":"instance_manifest","path":".ai-governance/instance.json","ownership":"tool","expected_sha256":"d2334d08bc2f9fec68d66b5f3af59e213d0f518e940057b194de2f25432bee3b","actual_sha256":null,"reason":"target_missing"},{"action":"CREATE","logical_id":"schema_root/install-manifest.schema.json","path":".ai-governance/schemas/install-manifest.schema.json","ownership":"tool","expected_sha256":"490a29358da5d1ad678effe58635fab5af88cf9693bf0f246d7b30a88abb4ed1","actual_sha256":null,"reason":"target_missing"},{"action":"CREATE","logical_id":"schema_root/instance.schema.json","path":".ai-governance/schemas/instance.schema.json","ownership":"tool","expected_sha256":"6dfe34eda589b1c8a948669fef164c60ff7272b5ed65eeb05f47af669bf559af","actual_sha256":null,"reason":"target_missing"},{"action":"CREATE","logical_id":"agent_entry","path":"AGENTS.md","ownership":"project","expected_sha256":"08958b0e158adf4141fa7219e67146e439f476f240699306bbbfe3d18435ad0a","actual_sha256":null,"reason":"target_missing"},{"action":"SKIP","logical_id":"design_spec","path":"DESIGN.md","ownership":"project","expected_sha256":null,"actual_sha256":null,"reason":"no_frontend"},{"action":"SKIP","logical_id":"project_decisions","path":"docs/decisions.md","ownership":"project","expected_sha256":null,"actual_sha256":null,"reason":"lifecycle_not_reached"}]}
```

同一 canonical 内容全部存在时的完整 stdout 为：

```json
{"contract_version":"0.2-draft","instance_id":"inst-example-01","operations":[{"action":"NOOP","logical_id":"instance_manifest","path":".ai-governance/instance.json","ownership":"tool","expected_sha256":"d2334d08bc2f9fec68d66b5f3af59e213d0f518e940057b194de2f25432bee3b","actual_sha256":"d2334d08bc2f9fec68d66b5f3af59e213d0f518e940057b194de2f25432bee3b","reason":"managed_content_matches"},{"action":"NOOP","logical_id":"schema_root/install-manifest.schema.json","path":".ai-governance/schemas/install-manifest.schema.json","ownership":"tool","expected_sha256":"490a29358da5d1ad678effe58635fab5af88cf9693bf0f246d7b30a88abb4ed1","actual_sha256":"490a29358da5d1ad678effe58635fab5af88cf9693bf0f246d7b30a88abb4ed1","reason":"managed_content_matches"},{"action":"NOOP","logical_id":"schema_root/instance.schema.json","path":".ai-governance/schemas/instance.schema.json","ownership":"tool","expected_sha256":"6dfe34eda589b1c8a948669fef164c60ff7272b5ed65eeb05f47af669bf559af","actual_sha256":"6dfe34eda589b1c8a948669fef164c60ff7272b5ed65eeb05f47af669bf559af","reason":"managed_content_matches"},{"action":"NOOP","logical_id":"agent_entry","path":"AGENTS.md","ownership":"project","expected_sha256":null,"actual_sha256":"08958b0e158adf4141fa7219e67146e439f476f240699306bbbfe3d18435ad0a","reason":"project_fact_preserved"},{"action":"SKIP","logical_id":"design_spec","path":"DESIGN.md","ownership":"project","expected_sha256":null,"actual_sha256":null,"reason":"no_frontend"},{"action":"SKIP","logical_id":"project_decisions","path":"docs/decisions.md","ownership":"project","expected_sha256":null,"actual_sha256":null,"reason":"lifecycle_not_reached"}]}
```

若仅 instance manifest 的实际字节是 `{}\n`（SHA-256 `ca3d163bab055381827226140568f3bef7eaac187cebd76878e0b63e9e442356`），完整 conflict stdout 如下并退出 2：

```json
{"contract_version":"0.2-draft","instance_id":"inst-example-01","operations":[{"action":"CONFLICT","logical_id":"instance_manifest","path":".ai-governance/instance.json","ownership":"tool","expected_sha256":"d2334d08bc2f9fec68d66b5f3af59e213d0f518e940057b194de2f25432bee3b","actual_sha256":"ca3d163bab055381827226140568f3bef7eaac187cebd76878e0b63e9e442356","reason":"managed_content_differs"},{"action":"NOOP","logical_id":"schema_root/install-manifest.schema.json","path":".ai-governance/schemas/install-manifest.schema.json","ownership":"tool","expected_sha256":"490a29358da5d1ad678effe58635fab5af88cf9693bf0f246d7b30a88abb4ed1","actual_sha256":"490a29358da5d1ad678effe58635fab5af88cf9693bf0f246d7b30a88abb4ed1","reason":"managed_content_matches"},{"action":"NOOP","logical_id":"schema_root/instance.schema.json","path":".ai-governance/schemas/instance.schema.json","ownership":"tool","expected_sha256":"6dfe34eda589b1c8a948669fef164c60ff7272b5ed65eeb05f47af669bf559af","actual_sha256":"6dfe34eda589b1c8a948669fef164c60ff7272b5ed65eeb05f47af669bf559af","reason":"managed_content_matches"},{"action":"NOOP","logical_id":"agent_entry","path":"AGENTS.md","ownership":"project","expected_sha256":null,"actual_sha256":"08958b0e158adf4141fa7219e67146e439f476f240699306bbbfe3d18435ad0a","reason":"project_fact_preserved"},{"action":"SKIP","logical_id":"design_spec","path":"DESIGN.md","ownership":"project","expected_sha256":null,"actual_sha256":null,"reason":"no_frontend"},{"action":"SKIP","logical_id":"project_decisions","path":"docs/decisions.md","ownership":"project","expected_sha256":null,"actual_sha256":null,"reason":"lifecycle_not_reached"}]}
```

三个 contract golden 都必须保存并逐字节比较，禁止用部分字段断言替代。

## 任务契约

```yaml
task_id: T-PLAN-002
title: 输出可审核、可重算的最小安装文件计划
phase: 1
approval_required: PLAN-002-R1
acceptance_criteria:
  - 本文经独立复审 PASS 后转为 active；contract tests 先提交并证明旧实现失败，再提交产品实现
  - blank、all-NOOP、conflict 三个 golden 冻结四个目标、两个 SKIP、四种 action、七键 operation、退出码和排序
  - 每个非空 SHA-256 由 tests 对原始目标字节独立重算；canonical 摘要必须匹配本文
  - instance schema/fixture 通过正向及完整负向矩阵，17 个 resources 与实际 effective map 一致
  - 已有 AGENTS.md 始终 NOOP project_fact_preserved；其字节不参与 expected content，也不判模板冲突
  - target type、containment、读取失败与派生路径碰撞 fail-closed，diagnostics 不泄露绝对路径
  - 同步 03 的当前 plan JSON 示例；17 只保留 T-PLAN-001 历史文本输出证据，不继续作为当前输出依据
  - plan 输出不含绝对路径、当前时间、随机数、临时目录或 OS 错误文本
  - Windows amd64、Linux amd64、macOS arm64 对 portable fixtures 得到相同字节；平台 fixture 单独报告
  - 所有路径执行前后目标文件清单、字节、mode 与目录状态一致
  - 不实现 apply、恢复点、rollback、doctor、upgrade、状态机、trust、签名或发布逻辑
  - 仅用 Go 标准库，go test -count=1 ./...、go vet ./...、go mod tidy -diff 与核心黑盒命令通过
allowed_files:
  - tool/README.md
  - tool/contracts/v0/instance.schema.json
  - tool/contracts/v0/embed.go
  - tool/contracttest/plan_test.go
  - tool/contracttest/testdata/plan/
  - tool/internal/cli/run.go
  - tool/internal/cli/run_test.go
  - tool/internal/plan/plan.go
  - tool/internal/plan/plan_test.go
  - docs/正式项目接入/03-治理实例规范v0.md
  - docs/正式项目接入/20-阶段1可执行安装Plan契约任务单.md
  - docs/正式项目接入/README.md
  - .github/workflows/aigov-plan-contract.yml
  - .git/
pre_implementation_search:
  commands:
    - rg -n 'agent_entry|instance_manifest|schema_root|project_decisions|design_spec|plan|NO_CHANGES' docs/正式项目接入/03-治理实例规范v0.md tool
    - rg -n 'CREATE|NOOP|SKIP|CONFLICT|managed_files|methodology_version|assurance_level' docs poc experiments
    - rg -n 'func Evaluate|defaultResources|safeRelativePath|snapshotTree|const version|0\.1\.0-dev|go:embed' tool
  evidence_required: 列出当前生成/摘要能力、项目事实保护边界、实验候选行为、版本单一来源与 instance schema；只复用契约事实，不复制实验实现
doc_type: how-to
doc_purpose: 说明如何读取和审核最小安装 plan，以及为何 conflict 会阻止未来 apply
```

## 人工批准与更新记录

- 2026-11-25：在安装 manifest 0.2 契约独立 PASS 后形成 R1 草案。
- 2026-11-25：项目负责人回复“批准”，批准四个目标、固定模板、最小 schema、七键 operation、动作/退出码/排序/摘要语义；不授权 apply 或目标写入。
- 2026-11-25：首次独立审查 FAIL（Critical 2 / Major 4 / Minor 1）；补齐精确字节、目标安全、派生碰撞、plan/NO_CHANGES 边界、完整 operation/golden 与 schema 负向矩阵，等待复审。
- 2026-11-25：第二次复审 FAIL（Critical 1 / Major 4 / Minor 1）后补齐真实 conflict fixture、自定义 instance path、安全诊断和稳定 collision Pointer；第三次独立复审 PASS（Critical 0 / Major 0 / Minor 0），任务转为 active。
- 2026-11-25：以 `6f61ffc` 提交 contract tests 并取得旧实现退出 1 的有效红灯；随后完成最小实现、跨平台 workflow 与 target/instance 负向矩阵，最终独立代码复审 PASS（Critical 0 / Major 0 / Minor 0）。

## 完成证据

- 红灯：`go test -count=1 ./contracttest` → 退出码 1；失败仅来自旧实现仍输出两行文本且未拒绝目录 target。
- 绿灯：`go test -count=1 ./...`、`go vet ./...`、`go mod tidy -diff`、`go build ./cmd/aigov` → 均退出码 0。
- 黑盒：同一 blank target 连续两次退出 0、stdout 逐字节一致、目标目录条目数保持 0。
- golden：blank/all-NOOP/conflict、canonical schema/instance/AGENTS SHA-256 全部逐字节通过。
- safety：Windows junction、任意 project-owned AGENTS、missing parent 实际 PASS；当前 Windows 无权限/无等价 fixture 的 symlink、special-file、mode-000 用例明确 SKIP，由三平台 workflow 继续验证可构造分支。
- final changed_files 为 11 个，全部位于 `allowed_files`；未实现 `apply`、恢复点、rollback、doctor、upgrade、状态机、trust、签名或发布逻辑。
