---
type: reference
purpose: 约束阶段 1 首个只读 plan 纵向切片必须先建立黑盒 contract tests 再实现
audience: 阶段 1 Executor、人工 Reviewer 与维护 aigov CLI 的后续 Agent
status: active
---

# T-PLAN-001：测试先行的最小只读 Plan

## 用途

阶段 0 已人工放行后，以本任务验证第一个真实安装器核心路径。Executor 必须先提交可执行黑盒 contract tests，记录当前 CLI 因尚无 `plan` 而失败的证据，再实现刚好让这些测试通过的只读纵向切片。

## 任务契约

```yaml
task_id: T-PLAN-001
title: 用黑盒 contract tests 实现最小只读 plan
phase: 1
acceptance_criteria:
  - contract tests 先于产品实现形成独立 commit，并证明旧 CLI 不能满足 plan 契约
  - aigov plan --manifest <path> 在显式功能开关开启时可执行，默认关闭时拒绝并给出启用方法
  - 有效阶段 0 manifest 返回 0，至少输出 project_decisions 生命周期跳过与 design_spec 无前端跳过
  - 缺少 phase_basis 返回 2，并包含 E_REQUIRED 与 /governance/phase_basis
  - 未知字段和越界资源路径返回 2，并分别包含 E_UNKNOWN_FIELD 与 E_UNSAFE_PATH 的 JSON Pointer
  - contract_version 只接受精确值 0.1-draft，其他值返回 E_CONTRACT_VERSION
  - manifest 的每个必填字段、类型、枚举、复合字段一致性和各对象层未知字段都有表驱动负向测试
  - 自定义资源映射拒绝未知逻辑 ID、绝对路径、.. 路径和两个可写逻辑 ID 指向同一路径
  - 当前切片范围外的 manifest 输入 fail-closed，禁止忽略或猜测默认值后继续 plan
  - plan 对目标项目零写入；执行前后文件清单、文件字节和目录状态一致
  - 输出与诊断顺序确定，同一输入重复运行得到逐字节相同 stdout/stderr
  - 不实现 apply、doctor、upgrade、rollback、状态机、信任或发布逻辑
  - 仅使用 Go 标准库，go test -count=1 ./...、go vet ./... 与核心黑盒命令通过
  - README 提供可复制的功能开关、有效输入、失败输入和关闭回滚示例
allowed_files:
  - tool/README.md
  - tool/cmd/aigov/main.go
  - tool/internal/cli/run.go
  - tool/internal/cli/run_test.go
  - tool/internal/plan/plan.go
  - tool/internal/plan/plan_test.go
  - tool/contracttest/plan_test.go
  - tool/contracttest/testdata/plan/
  - tool/contracts/v0/install-manifest.schema.json
  - docs/正式项目接入/17-阶段1首个Plan契约任务单.md
  - .git/
pre_implementation_search:
  commands:
    - rg -n 'plan|E_REQUIRED|E_UNKNOWN_FIELD|E_UNSAFE_PATH|phase_basis|lifecycle_not_reached|no_frontend' docs/正式项目接入/03-治理实例规范v0.md poc experiments
    - rg -n 'func Run|unsupported command|AIGOV_' tool
    - git -C experiments/installer-lifecycle-matrix ls-files
  evidence_required: 列出规范测试向量、当前 CLI 扩展点和候选实验实现；逐项说明只复用契约事实，不复制 PoC/实验代码
doc_type: how-to
doc_purpose: 说明实验性只读 plan 的启用、有效/无效输入、确定性输出和一键关闭方式
```

## 固定执行顺序

1. 先只修改 contract-test、fixture 与 schema 文件，提交一个测试先行 commit。
2. 在该 commit 上运行定向 contract test，记录失败只来自 `plan` 尚未实现或功能开关未接入；若测试自身编译失败，不算有效红灯。
3. 独立 Reviewer 核对测试是否忠实覆盖 03 的完整 manifest 字段表、兼容策略、三个既定向量和下方负向矩阵，禁止为当前实现降低断言；需要产品语义取舍时再升级项目负责人。
4. 再实现最小 `plan` 并补必要的单元测试与 README。
5. 最终运行非缓存测试、静态检查、黑盒命令、确定性复跑和零写入快照比较。

测试先行 commit 可以在最终实现后保留在同一任务分支历史中；最终提交必须恢复 `go test ./...` 全绿，不允许把红测试合入最终基线。

## Manifest 契约负向矩阵

`install-manifest.schema.json` 必须完整表达 03“安装输入 manifest”已冻结的结构；Go contract tests 再覆盖 JSON Schema 难以表达的跨字段和环境规则。不得只针对三个样例文件写条件分支。

| 分类 | 最小必测集合 | 预期 |
|---|---|---|
| 契约版本 | 缺失、非字符串、`0.1`、未知版本 | `E_REQUIRED` / `E_TYPE` / `E_CONTRACT_VERSION` |
| 必填字段 | 对 03 字段表中每个必填叶子逐一删除 | `E_REQUIRED` + 精确 JSON Pointer |
| 类型与枚举 | 每个 boolean/integer/string/array/object 使用错误 JSON 类型；phase、CI、mode、profile、assurance 等越界枚举 | `E_TYPE` / 对应稳定错误码 + Pointer |
| 未知字段 | root、project、governance、phase_basis、selection_record、independent_review_source、integration、layout 各增加未知成员；`resources` 不属于本行，其未知 key 由“资源映射”行覆盖 | `E_UNKNOWN_FIELD`，不得静默忽略 |
| 字符串与集合 | 空白 name/core hypothesis、重复/空 technology、空/未知 signal、未来超过 300 秒的 assessed_at | 对应诊断并拒绝 |
| 复合一致性 | phase 缺 basis；selection false + 非 null record；selection true + 缺 record/文件/hash/批准字段；review false + 非 null source；phase ≥2 + review false | 03 规定的稳定错误或 `E_REQUIRED`，不得继续 plan |
| 项目环境 | root 不存在；git_required=true 但目标不是 Git 仓库 | `E_ROOT_NOT_FOUND` / `E_GIT_REQUIRED` |
| 资源映射 | 未知逻辑 ID、绝对路径、`..`、两个可写 ID 解析为同一路径 | `E_UNKNOWN_RESOURCE` / `E_UNSAFE_PATH` / `E_RESOURCE_COLLISION` |
| 确定性 | 同一无效输入重复运行两次 | exit/stdout/stderr 逐字节一致，诊断按 JSON Pointer 再按 code 排序 |

若 03 尚未给某个条件分配专用错误码，本任务必须在实现前把拟用 code 作为任务 revision 提交审查；不得在代码中临时发明后再倒写契约。

## Revision R1：首个切片的稳定诊断契约

03 已冻结 `E_REQUIRED`、`E_UNKNOWN_FIELD`、`E_CONTRACT_VERSION`、`E_UNSAFE_PATH`、`E_GIT_REQUIRED` 与 `E_REVIEW_UNAVAILABLE` 的语义，但没有为本任务负向矩阵中的其余条件逐一分配 code。T-PLAN-001 在产品实现前补充以下最小集合；这些 code 只约束安装 manifest 的只读校验，不扩展到 apply、状态机或信任协议。

| code | 本切片中的固定条件 |
|---|---|
| `E_JSON` | manifest 不是单一合法 JSON 值：包括语法错误、首个值后的非空白 token 或同一对象内重复成员名；固定指向根 Pointer `""`，并且只输出这一条诊断 |
| `E_TYPE` | JSON 值类型与字段表不符；整数不接受小数，`null` 只在明确允许的字段中合法 |
| `E_ENUM` | phase 范围、CI、mode、profile、assurance level、provider 或 phase signal 不在冻结枚举中 |
| `E_VALUE` | 字符串去除首尾空白后为空、`core_hypothesis` 含 `[待定]`、数组项为空或重复、主体 ID/RFC 3339/SHA-256 格式无效，或 assessed_at 超前超过 300 秒 |
| `E_INCONSISTENT` | selection/review 布尔开关与对应 null/object 不一致；不包括已有专用码的 phase ≥2 且 review 不可用 |
| `E_ROOT_NOT_FOUND` | `project.root` 不能解析为现有目录 |
| `E_FILE_NOT_FOUND` | selection record 的项目内相对路径不存在或不是普通文件 |
| `E_HASH_MISMATCH` | selection record 声明的 SHA-256 与目标文件原始字节不一致 |
| `E_UNKNOWN_RESOURCE` | `layout.resources` 包含未冻结的逻辑资源 ID |
| `E_RESOURCE_COLLISION` | 两个可写逻辑资源 ID 解析到同一规范化项目内路径 |

诊断使用 UTF-8 JSON Lines 写入 stderr，每行对象的键顺序固定为 `code`、`path`、`message`；`path` 使用 RFC 6901 JSON Pointer，整份文档的根 Pointer 是 JSON 空字符串 `""`，`/` 只表示空成员名。一次输入中的诊断按 `path`、再按 `code` 的 UTF-8 字节序升序输出，末行也带 `\n`。任何诊断存在时 stdout 必须为空且退出码为 2。message 必须是稳定、可操作的修复建议，不得回显不受控输入，也不得嵌入当前时间、临时绝对路径或操作系统生成的错误文本。同一输入和未变化目标目录重复运行必须得到逐字节相同的 stdout/stderr；时间负向测试使用 `9999-12-31T23:59:59Z`，远离 300 秒边界，连续复跑不得漂移。

稳定修复建议模板如下；其中 `<path>` 只能替换为已转义的诊断 Pointer，`<type>` 与 `<allowed>` 只能来自代码中冻结的字段契约，不能来自 manifest 原文：`E_JSON` 为 `provide one JSON object without duplicate keys or trailing tokens`；`E_REQUIRED` 为 `add required field <path>`；`E_UNKNOWN_FIELD` 为 `remove unknown field <path>`；`E_CONTRACT_VERSION` 为 `set contract_version to 0.1-draft`；`E_TYPE` 为 `set <path> to JSON type <type>`；`E_ENUM` 为 `set <path> to one of: <allowed>`；`E_VALUE` 为 `replace <path> with a value satisfying its documented constraints`；`E_INCONSISTENT` 为 `make <path> consistent with its controlling field`；`E_ROOT_NOT_FOUND` 为 `set project.root to an existing directory`；`E_FILE_NOT_FOUND` 为 `set selection_record.path to an existing project file`；`E_HASH_MISMATCH` 为 `set selection_record.sha256 to the selected file SHA-256`；`E_UNKNOWN_RESOURCE` 为 `use a known logical resource ID at <path>`；`E_UNSAFE_PATH` 为 `use a project-relative path without absolute prefixes or .. segments`；`E_RESOURCE_COLLISION` 为 `map <path> to a distinct project-relative path`；`E_GIT_REQUIRED` 为 `initialize Git at project.root or set git_required to false`；`E_REVIEW_UNAVAILABLE` 为 `provide an independent review source for phase 2 or later`。contract tests 对三个既定向量断言完整固定 message，对表驱动矩阵至少断言非空 message 与对应 code 的上述修复方向。

字段缺失一律使用 `E_REQUIRED` 指向缺失字段的精确 Pointer；普通对象的未知字段一律使用 `E_UNKNOWN_FIELD`。`layout.resources` 是逻辑 ID 映射而不是普通 schema object，其未知成员唯一使用 `E_UNKNOWN_RESOURCE`，不再同时返回 `E_UNKNOWN_FIELD`。selection record 的 `path` 若为绝对路径或包含 `..`，仍使用 `E_UNSAFE_PATH`，只有安全路径解析后不存在才使用 `E_FILE_NOT_FOUND`。`contract_version` 类型错误使用 `E_TYPE`，字符串但不是精确 `0.1-draft` 才使用 `E_CONTRACT_VERSION`。

### 校验阶段、级联抑制与诊断 Pointer

校验顺序固定如下，后一阶段只在它依赖的字段已通过前一阶段时运行：

1. JSON 层：拒绝语法错误、尾随 token 和任一对象中的重复 key；失败时只返回 `E_JSON`，path 为根 Pointer `""`。根 JSON 值不是 object 时返回 `E_TYPE`，path 同样为 `""`。
2. 结构层：从根对象向下检查必填字段、类型和普通对象未知字段。父对象缺失或类型错误时，返回父对象诊断并抑制其全部子字段诊断；未知成员的值不再递归校验。
3. 字段层：只对类型正确的字段检查格式、枚举、范围、安全相对路径及数组项。一个字段的字段层错误不会产生依赖它的跨字段或环境诊断。
4. 复合层：只在所有依赖字段通过结构层和字段层后检查 selection/review 一致性与 phase/review 门禁。
5. 环境层：只在 root、git_required、selection record path/hash 均通过其先决校验后访问文件系统。root 不存在时抑制 Git、selection 文件与 hash 检查。

跨字段和环境诊断的 Pointer 固定为：selection 开关不一致指向 `/governance/selection_record`；review 开关不一致指向 `/governance/independent_review_source`；phase ≥2 且 review 不可用指向 `/governance/independent_review_available`；root 不存在和 Git 要求不满足均指向 `/project/root`；selection 文件不存在指向 `/governance/selection_record/path`；hash 不一致指向 `/governance/selection_record/sha256`。

本切片中 03 已冻结的全部逻辑 ID 都参与 collision：`agent_entry`、`design_spec`、`project_decisions`、`instance_manifest`、`phase_snapshot`、`maturity_fact_store`、`maturity_snapshot`、`policy_snapshot`、`trust_anchor`、`runtime_root`、`schema_root`、`task_store`、`evidence_store`、`review_store`、`task_state_cache`、`task_audit_store`、`phase_audit_store`。不存在只读或条件资源例外。effective map 先使用 03“目录边界”表中的 17 个默认路径初始化，再用 `layout.resources` 的已知、安全映射逐项覆盖，最后对全部 17 个规范化结果检查 collision；因此仅覆盖一个 ID 指向另一个未显式覆盖 ID 的默认路径也必须拒绝。

资源 collision 只消费逻辑 ID 已知、值为 string 且路径已通过安全校验的覆盖。未知逻辑 ID 只返回 `E_UNKNOWN_RESOURCE`，抑制其 value 的 `E_TYPE`/`E_UNSAFE_PATH` 等诊断并排除出 effective map。每组规范化目标相同的 effective 映射按逻辑 ID 的 UTF-8 字节序排序：若组内含未覆盖的默认映射，则对组内每个显式覆盖 ID 输出一条 `E_RESOURCE_COLLISION`；若组内全部是显式覆盖，则保留排序最前者作为基准，对其余每个 ID 输出一条。Pointer 固定为 `/layout/resources/<escaped-logical-id>`，且总是指向 manifest 中实际存在的覆盖成员。

`project.name` 去除首尾空白后的长度按 Unicode code point 计算，必须为 1..128；越界固定返回 `E_VALUE /project/name`。

数组项诊断始终指向具体索引。technology 项在去除首尾空白后为空时返回 `E_VALUE /project/technology/<index>`；去除首尾空白后以区分大小写的字节值比较，第一次出现保留，其后每个重复项各返回一条 `E_VALUE` 指向后出现的索引。signals 为空数组时只返回 `E_VALUE /governance/phase_basis/signals`，不生成索引级诊断并抑制依赖该数组的复合校验。非空 signals 中的未知枚举返回 `E_ENUM /governance/phase_basis/signals/<index>`。03 没有禁止重复 signal，因此本任务不得因 signal 重复而拒绝。无效 technology 项或未知 signal 不参与依赖该数组的复合校验。

### 字符串身份与路径规范化

本切片中的受信主体 ID 固定为 `<namespace>:<id>`：namespace 必须匹配 `[a-z][a-z0-9_-]{0,31}`，id 必须匹配 `[A-Za-z0-9][A-Za-z0-9._@+/-]{0,127}`，整体不得含空白。该语法只验证可重放格式，不把 Agent 自报身份升级为强信任。

资源路径与 `selection_record.path` 共用同一套词法安全规则：先把 `\\` 视为分隔符并转换为 `/`，再折叠重复分隔符和 `.` 段。以 `/`、`//` 或盘符前缀（包括 `C:` 与 `C:relative`）开头，或任一原始路径段为 `..` 时返回 `E_UNSAFE_PATH`。因此 `../x`、`a\\..\\x`、盘符路径和 UNC 路径都必须拒绝。

资源映射的只读 plan 只执行上述词法规则；未来任何写命令仍须在写入前对真实文件系统重新执行 containment 检查。selection record 需要读取现有普通文件，因此还必须分别解析 project root 与目标文件的 symlink 后，用目标操作系统的路径语义确认目标仍在真实 root 内；逃逸到 root 外返回 `E_UNSAFE_PATH /governance/selection_record/path`，不得读取或哈希该文件。collision 比较使用清理后的 `/` 路径；Windows 上额外按 ASCII 大小写不敏感比较，其他平台按字节区分。因此 `a/b` 与 `a/./b` 总是冲突，`a\\b` 与 `a/b` 总是冲突，`A/b` 与 `a/b` 仅在 Windows 上冲突。

## 功能开关

阶段 1 使用环境变量 `AIGOV_ENABLE_EXPERIMENTAL_PLAN=1` 开启 `plan`。未设置时命令返回 2，并明确说明功能仍处于实验验证；回滚只需取消该变量，不改目标项目文件。

## 明确不做

- 不生成任何治理文件，不创建备份，不修改时间戳。
- 不把 03 的全部状态机、信任或十一份实例 schema 一次性塞进首个切片。
- 不复制 `experiments/installer-lifecycle-matrix` 的候选实现；它只用于验证契约和发现边界。
- 不因阶段 1 已开始就宣称核心假设已验证；只有关键路径端到端通过后才能评估阶段 2。

## 验证命令

在 `tool/` 目录执行：

```powershell
$taskTemp = Join-Path ([System.IO.Path]::GetTempPath()) ("aigov-contract-" + [guid]::NewGuid().ToString('N'))
$taskBin = Join-Path $taskTemp 'aigov-contract.exe'
New-Item -ItemType Directory -Path $taskTemp | Out-Null
try {
    go test -count=1 ./...
    if ($LASTEXITCODE -ne 0) { throw "go test exit code was $LASTEXITCODE" }
    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw "go vet exit code was $LASTEXITCODE" }
    go build -o $taskBin ./cmd/aigov
    if ($LASTEXITCODE -ne 0) { throw "go build exit code was $LASTEXITCODE" }

    $env:AIGOV_ENABLE_EXPERIMENTAL_PLAN = '1'
    & $taskBin plan --manifest ./contracttest/testdata/plan/valid-phase0.json
    if ($LASTEXITCODE -ne 0) { throw "valid plan exit code was $LASTEXITCODE" }
    & $taskBin plan --manifest ./contracttest/testdata/plan/missing-phase-basis.json
    if ($LASTEXITCODE -ne 2) { throw "missing phase_basis exit code was $LASTEXITCODE" }
    & $taskBin plan --manifest ./contracttest/testdata/plan/unknown-and-unsafe.json
    if ($LASTEXITCODE -ne 2) { throw "invalid manifest exit code was $LASTEXITCODE" }

    Remove-Item Env:AIGOV_ENABLE_EXPERIMENTAL_PLAN
    & $taskBin plan --manifest ./contracttest/testdata/plan/valid-phase0.json
    if ($LASTEXITCODE -ne 2) { throw "disabled plan exit code was $LASTEXITCODE" }
}
finally {
    Remove-Item Env:AIGOV_ENABLE_EXPERIMENTAL_PLAN -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $taskBin -PathType Leaf) { Remove-Item -LiteralPath $taskBin -Force }
    if (Test-Path -LiteralPath $taskTemp -PathType Container) { Remove-Item -LiteralPath $taskTemp -Force }
}
```

第一条有效 `plan` 应返回 0；两个无效 manifest 和功能关闭命令应返回 2。测试必须在临时目标目录执行并自行断言零写入，不能依赖人工目测。

## 更新记录

- 2026-11-24：在阶段 0 最终人工放行后创建首个阶段 1 contract-first 任务。
