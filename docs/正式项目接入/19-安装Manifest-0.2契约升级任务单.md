---
type: reference
purpose: 约束可执行安装 plan 前必须先把显式实例身份加入安装 manifest 并完成测试先行升级
audience: 阶段 1 Executor、人工 Reviewer 与维护安装 manifest/plan 的后续 Agent
status: active
---

# T-CONTRACT-002：安装 Manifest 0.2 显式实例身份

## 用途

`T-PLAN-002` 需要为 `.ai-governance/instance.json` 给出确定的目标字节和 SHA-256；当前 `0.1-draft` 安装 manifest 没有 `instance_id`，若由 `plan` 临时随机生成会破坏复跑确定性，若从项目路径或名称推导又会造成跨机器漂移或身份碰撞。项目负责人已批准把显式 `instance_id` 加入 manifest，并将 `contract_version` 升级为 `0.2-draft`。本任务必须先完成该公共输入契约升级，再允许开发可执行安装 plan。

## 人工批准

- 批准事项：安装 manifest 新增必填 `instance_id`，`contract_version` 从 `0.1-draft` 升为 `0.2-draft`。
- approved by：项目负责人（当前线程用户）。
- approved at：2026-11-25（Asia/Shanghai）。
- 批准边界：只批准显式身份输入与必要的 contract/schema/tests/validator/documentation 同步，不批准 `apply`、随机身份生成、状态机、信任或发布逻辑。

## 任务契约

```yaml
task_id: T-CONTRACT-002
title: 将安装 manifest 升级为带显式 instance_id 的 0.2-draft
phase: 1
acceptance_criteria:
  - 03 正式规范的安装 manifest 根对象新增必填 instance_id，并把该输入契约精确版本升级为 0.2-draft
  - instance_id 只接受 8..128 个 ASCII 字符，固定匹配 ^inst-[A-Za-z0-9][A-Za-z0-9._-]{2,122}$，无默认值且禁止 plan 自行生成
  - 缺少 instance_id 返回 E_REQUIRED /instance_id，非字符串返回 E_TYPE /instance_id，格式无效返回 E_VALUE /instance_id
  - contract_version 只接受精确 0.2-draft；0.1-draft 与其他字符串均返回 E_CONTRACT_VERSION /contract_version，并给出 0.2-draft 修复建议
  - schema、三个 canonical fixtures、表驱动 contract tests、plan validator 与文档调用方同步更新
  - 17 保留 T-PLAN-001 的 0.1-draft 历史证据，但明确当前安装输入契约已由本任务升级为 0.2-draft，禁止继续把 0.1-draft 当作实现依据
  - 先提交 docs/schema/fixtures/contract tests，并证明旧 validator 不能满足新契约；再修改产品实现使测试恢复全绿
  - 保留 03 中 schema_version=0.1-draft 的事件、批准和 trust 协议，不做无关的全局版本替换
  - 不实现 instance.json 内容生成、CREATE/NOOP/SKIP/CONFLICT、apply、迁移器、状态机、信任或发布逻辑
  - 仅使用 Go 标准库，go test -count=1 ./...、go vet ./...、go mod tidy -diff 与核心黑盒命令通过
  - 最终由独立 Reviewer 确认公共契约、测试、实现和 changed_files 一致
allowed_files:
  - docs/decisions.md
  - docs/正式项目接入/03-治理实例规范v0.md
  - docs/正式项目接入/17-阶段1首个Plan契约任务单.md
  - docs/正式项目接入/19-安装Manifest-0.2契约升级任务单.md
  - docs/正式项目接入/README.md
  - tool/contracts/v0/install-manifest.schema.json
  - tool/contracttest/plan_test.go
  - tool/contracttest/testdata/plan/valid-phase0.json
  - tool/contracttest/testdata/plan/missing-phase-basis.json
  - tool/contracttest/testdata/plan/unknown-and-unsafe.json
  - tool/internal/plan/plan.go
  - tool/internal/plan/plan_test.go
  - tool/README.md
  - .git/
pre_implementation_search:
  commands:
    - rg -n 'contract_version|0\.1-draft|instance_id' docs/decisions.md docs/正式项目接入 tool
    - rg -n 'contractVersion|contract_version|valid-phase0' tool/internal/plan tool/contracttest
    - rg -n 'instance_id|manifestPath|plan\(' experiments/installer-lifecycle-matrix poc
  evidence_required: 区分安装输入 contract_version 与事件/trust schema_version；列出全部 schema、fixture、test、validator 和文档调用方；说明只复用实验边界证据，不复制候选实现
doc_type: reference
doc_purpose: 冻结安装 manifest 0.2 的显式实例身份、兼容策略和测试先行升级步骤
```

## 固定语义

- `instance_id` 由项目负责人或受控编排器在调用 `plan` 前分配；推荐使用带 `inst-` 前缀的 UUID/ULID 表示，但工具只验证上述 ASCII 语法，不把格式校验冒充全局唯一性证明。
- 相同 manifest 必须携带相同 `instance_id`，因此计划内容和摘要可逐字节复跑；复制项目准备重新实例化时必须显式提供新的 ID。
- 当前尚无正式 `apply` 产生的已安装实例，不提供 `0.1-draft` 自动迁移或兼容猜测；旧输入 fail-closed，并给出升级到 `0.2-draft` 的建议。
- 本次版本升级只作用于安装 manifest 的 `contract_version`。事件、任务、批准信封和 trust 测试向量中的 `schema_version: 0.1-draft` 保持不变。

## 固定执行顺序

1. 先修改 03、decisions、schema、fixtures 和 contract tests，形成不含产品实现的独立 commit。
2. 在该提交上运行定向测试，红灯必须来自旧 validator 仍要求 `0.1-draft` 或尚未校验 `instance_id`；测试编译/fixture/schema 自身失败不算有效红灯。
3. 独立 Reviewer 核对版本边界、字段语义、负向矩阵和未修改的事件/trust `schema_version`。
4. 再修改 `internal/plan` 与必要的单元测试/README，使全部测试恢复全绿。
5. 最终复跑非缓存测试、vet、tidy、黑盒命令与独立审查。

## 明确不做

- 不由 CLI 生成随机或路径派生的 `instance_id`。
- 不生成 `.ai-governance/instance.json`，不输出文件操作计划。
- 不接受 `0.1-draft` 并静默补默认身份。
- 不修改事件哈希、批准、trust 或其他 schema 的 `0.1-draft` 测试向量。

## 更新记录

- 2026-11-25：项目负责人批准显式 `instance_id` 与安装 manifest `0.2-draft` 升级后创建任务。
- 2026-11-25：以 `e48d4b5` 先冻结 docs/schema/fixtures/contract tests 红灯，再以 `88a1c7f` 更新 validator 与调用文档；最终独立审查 PASS（Critical 0 / Major 0 / Minor 0）。

## 完成证据

- `go test -count=1 ./...`：退出码 0。
- `go vet ./...`：退出码 0。
- `go mod tidy -diff`：退出码 0，无 diff。
- 独立构建和有效 manifest：退出码 0；缺少 `phase_basis`、未知字段与越界路径、功能关闭均按契约返回 2。
- `0.1-draft` 输入固定返回 `E_CONTRACT_VERSION /contract_version`，message 为 `set contract_version to 0.2-draft`。
- 最终 changed_files 共 11 个，全部位于本任务 `allowed_files`；未实现任何文件计划、身份生成、`apply`、迁移、状态机或信任逻辑。
