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
| 未知字段 | root、project、governance、phase_basis、selection_record、independent_review_source、integration、layout、resources 各增加未知成员 | `E_UNKNOWN_FIELD`，不得静默忽略 |
| 字符串与集合 | 空白 name/core hypothesis、重复/空 technology、空/未知 signal、未来超过 300 秒的 assessed_at | 对应诊断并拒绝 |
| 复合一致性 | phase 缺 basis；selection false + 非 null record；selection true + 缺 record/文件/hash/批准字段；review false + 非 null source；phase ≥2 + review false | 03 规定的稳定错误或 `E_REQUIRED`，不得继续 plan |
| 项目环境 | root 不存在；git_required=true 但目标不是 Git 仓库 | `E_ROOT_NOT_FOUND` / `E_GIT_REQUIRED` |
| 资源映射 | 未知逻辑 ID、绝对路径、`..`、两个可写 ID 解析为同一路径 | `E_UNKNOWN_RESOURCE` / `E_UNSAFE_PATH` / `E_RESOURCE_COLLISION` |
| 确定性 | 同一无效输入重复运行两次 | exit/stdout/stderr 逐字节一致，诊断按 JSON Pointer 再按 code 排序 |

若 03 尚未给某个条件分配专用错误码，本任务必须在实现前把拟用 code 作为任务 revision 提交审查；不得在代码中临时发明后再倒写契约。

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
