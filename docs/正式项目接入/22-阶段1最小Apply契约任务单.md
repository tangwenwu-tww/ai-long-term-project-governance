---
type: reference
purpose: 在任何写入实现前冻结阶段 1 最小 apply 的批准绑定、原子提交、失败回滚与幂等边界
audience: 项目负责人、阶段 1 Executor、独立 Reviewer 与维护 aigov plan/apply 的后续 Agent
status: active
---

# APPLY-001-R1：最小 Apply 人工定稿草案

## 用途

T-PLAN-002 已能确定性输出四个目标的 `CREATE/NOOP/SKIP/CONFLICT`。本任务只增加一个显式批准、只消费当前计划的最小 `apply`，以便在 blank target 上创建计划中的文件，并在失败时恢复到调用前状态。项目负责人已批准 `APPLY-001-R1`，草案独立复审 PASS，现按 contract-first 顺序执行。

## 建议决定

### 1. 命令、批准绑定与输出

命令固定为：

```text
aigov apply --manifest <path> --plan-sha256 <64-lowercase-hex>
```

- 必须显式设置 `AIGOV_ENABLE_EXPERIMENTAL_APPLY=1`；未启用时退出 2 且零写入。
- manifest 必须通过 plan 的全部校验，且 `integration.mode` 必须为 `apply`；否则 `E_APPLY_MODE /integration/mode`。
- `plan_sha256` 是同一 CLI 版本对同一 manifest 和目标状态产生的完整 plan stdout 原始 UTF-8 字节（包含末尾 LF）的 SHA-256。缺失/格式错误返回 `E_PLAN_APPROVAL_REQUIRED /plan_sha256`。
- apply 先在零写入状态重新计算 plan；若摘要不同，返回 `E_PLAN_CHANGED /plan_sha256`，stdout 为空、退出 2。不得接受“内容等价”、截断摘要或仅 instance ID 批准。
- 计划含 `CONFLICT` 时返回 `E_PLAN_CONFLICT /operations`，退出 2；只允许执行 `CREATE`，`NOOP/SKIP` 永不写入。
- 成功 stdout 是单行 JSON + LF，键顺序固定为 `contract_version`、`instance_id`、`plan_sha256`、`status`、`created`。`created` 按规范化 path 字节序排列：

```json
{"contract_version":"0.2-draft","instance_id":"inst-example-01","plan_sha256":"<64hex>","status":"APPLIED","created":[".ai-governance/instance.json",".ai-governance/schemas/install-manifest.schema.json",".ai-governance/schemas/instance.schema.json","AGENTS.md"]}
```

若重新计算的 plan 没有 `CREATE` 且无 conflict，返回 `status=NO_CHANGES`、`created=[]`、退出 0；这才是 03 的 apply 幂等语义。

### 2. 写入集合与项目事实保护

- apply 只能获取 plan 内 `CREATE` operation 对应的已生成原始字节；不得从 stdout 反解析模板，也不得自行增加目标。
- existing project-owned `agent_entry` 只能是 `NOOP project_fact_preserved`，绝不重写、chmod、touch 或纳入备份。
- tool-owned `NOOP` 不重写；任一 tool-owned mismatch 必须在 preflight 成为 conflict 并阻止全部写入。
- apply 不创建 `SKIP` 资源，不修改 manifest、selection record 或目标仓库 Git 状态。

### 3. 并发、TOCTOU 与逐文件原子提交

本阶段威胁模型是由单一负责人控制、执行期间保持静默的目标仓库。exclusive lock 只协调遵守本协议的 apply 进程；preflight 与获锁后重算检测普通并发变化。拥有目标仓库写权限、能在系统调用之间恶意替换父目录、reparse point 或 final inode 的外部进程不在 APPLY-001 保证内。Go 标准库无法在 Windows/Linux/macOS 共同提供完整的目录句柄相对 no-follow 原语，因此本任务不得声称对这种主动攻击实现 TOCTOU fail-closed；非静默仓库必须停止并在隔离环境重跑。

1. 零写入 preflight 验证 manifest、目标安全、plan digest 和无 conflict。
2. effective `instance_manifest` 父目录下的 `.aigov-internal/` 是保留内部命名空间，lock 固定为 `.aigov-internal/apply.lock`。每个 temp 必须在对应 final 的同一父目录，文件名精确为 `.aigov-tmp-<plan_sha256>-<sha256(logical_id)>`，从而与 final 同卷并在 Windows 继承同一父目录 ACL。
3. 写入前构造所有 final、internal dir、lock 和 temp path，并按 ASCII case-fold 检查相等与父子关系。唯一允许的 internal 父子边是 `internal dir → apply.lock`；final 与任一 internal path、final 与任一 temp、temp 与 temp 的相等/父子冲突，以及其他 internal-internal 关系均返回 `E_INTERNAL_PATH_COLLISION /apply`。logical ID 摘要必须逐项唯一。禁止 manifest 把资源映射到保留 namespace、其祖先/后代，或映射到任一 `.aigov-tmp-*` sibling。
4. 逐层创建缺失的 instance parent 和 `.aigov-internal`。若 internal dir 已存在，只有它是 real directory、不是 symlink/junction/reparse point 且为空时才可复用，并记录为 pre-existing、成功/回滚时不得删除；非空但没有 lock 返回 `E_APPLY_TEMP_CONFLICT`，类型或安全边界错误返回 `E_TARGET_CHANGED`。再以 `O_CREATE|O_EXCL` 创建 lock；只有 create 返回 `IsExist` 才是 `E_APPLY_LOCKED`，其他 create/write/Sync/close/verify 失败均为 `E_APPLY_WRITE`。lock 内容固定为批准摘要加 LF。父目录若由本次创建，记录为 rollback-owned。
5. 获锁后重新计算 plan 和摘要；任何变化返回 `E_TARGET_CHANGED /apply` 并清理本次 lock/空目录，零目标写入。
6. 逐层创建每个 final parent 并重查安全边界，再以 `O_CREATE|O_EXCL` 创建 sibling temp。temp 已存在返回 `E_APPLY_TEMP_CONFLICT`，其他 create/write/Sync/close/hash/mode-or-type verify 失败为 `E_APPLY_WRITE`。全部 temp 就绪后按 path 排序提交。
7. 使用 hard-link create 把 sibling temp 原子发布到仍不存在的 final path；禁止 `os.Rename` 的可覆盖语义。final 已出现返回 `E_TARGET_CHANGED`；hard-link 不支持或其他发布失败返回 `E_APPLY_WRITE`，均不降级为覆盖写。
8. 每个 final 发布后立即核对原始字节摘要和平台属性，再删除对应 temp。所有成功后删除 lock；仅当 `.aigov-internal` 为本次创建且为空时才删除，最后输出成功 JSON。

目录创建只允许发生在 CREATE 目标父链和上述内部命名空间；逐层 `Mkdir` 后必须重查 containment、symlink/junction/reparse point 和类型。不得使用无边界 `MkdirAll` 后直接信任结果。受控仓库中若在获锁后仍观察到路径身份变化，立即按 `E_TARGET_CHANGED` 回滚；不尝试与外部写者竞争。

POSIX 上 lock/temp/final 创建后分别 `Chmod` 并复核为 `0600/0644/0644`，不受 umask 漂移；目录为 `0755`。任何 Chmod 或 mode 复核失败为 `E_APPLY_WRITE`。Windows 不声明 POSIX mode 等价；sibling temp 由 OS 正常继承 final 父目录 ACL，hard link 保留该文件对象的 security descriptor。Windows 验证 regular/non-reparse、内容摘要及 temp/final security descriptor 一致，且实现不得调用 Chmod 或显式放宽 ACL；属性/ACL 复核失败为 `E_APPLY_WRITE`。

### 4. 失败回滚与崩溃边界

- 可注入的每个步骤失败（建目录、lock、temp create/write/sync/close/hash、publish、final verify、cleanup）都必须进入同一 rollback 路径。
- 写入前的恢复点是 owned-file ledger：记录每个 CREATE target 的“原先不存在”、expected hash、内部路径和本次创建目录清单。由于 APPLY-001 不覆盖、不删除、不 chmod 任何既有对象，该 ledger 足以把进程内失败恢复为调用前状态，是 03 对纯 CREATE 切片的恢复点例外。
- rollback 逆序删除本次发布且摘要仍等于 expected 的 final，再删除各 sibling temp、lock、本次创建且为空的 internal dir 和其他目录；原先存在的文件/目录/mode/mtime 不得变化。
- 若 final 在失败后被外部改动，不得删除，返回 `E_APPLY_ROLLBACK /apply` 并保留 lock 作为人工恢复信号；原始失败和 rollback failure 都写入稳定诊断，不包含绝对路径或 OS 错误文本。
- 进程被强杀、掉电和内核崩溃不冒充已解决：本切片只保证进程可观察错误的回滚与单文件原子发布。遗留 lock/temp 或部分 final 会使下一次 apply fail-closed；自动 crash recovery、独立 rollback 命令和 durable journal 属于后续任务。

所有失败 stdout 必须为空、stderr 为稳定 JSONL、退出 2。preflight 诊断沿用 plan 排序；写入阶段最多输出一个 primary diagnostic，rollback 失败时紧随其后输出一个 `E_APPLY_ROLLBACK`，按该语义顺序而非字典序，重复 code/path 只保留一次。

逐阶段映射：mode/approval/digest/conflict/internal collision 使用下表专用 code；lock create 只有 `IsExist` 为 `E_APPLY_LOCKED`，其他失败为 `E_APPLY_WRITE`。获锁后 plan 变化、final 已出现、internal 类型/安全或其他安全重检变化为 `E_TARGET_CHANGED`；既有非空 internal dir 或 temp 已存在为 `E_APPLY_TEMP_CONFLICT`；mkdir、lock write/sync/close/verify、temp 非 IsExist create/write/sync/close/hash、POSIX chmod/mode、Windows type/reparse/ACL、hard-link 不支持/失败、final hash/属性、temp/lock/internal cleanup 失败统一为 `E_APPLY_WRITE`；任何 rollback remove/hash/属性/目录清理失败追加 `E_APPLY_ROLLBACK`。

稳定错误 message：

| code | path | message |
|---|---|---|
| `E_APPLY_MODE` | `/integration/mode` | `set integration.mode to apply` |
| `E_PLAN_APPROVAL_REQUIRED` | `/plan_sha256` | `provide the complete approved plan stdout SHA-256` |
| `E_PLAN_CHANGED` | `/plan_sha256` | `run plan again and approve its new stdout SHA-256` |
| `E_PLAN_CONFLICT` | `/operations` | `resolve every conflict and approve a new plan` |
| `E_INTERNAL_PATH_COLLISION` | `/apply` | `move every target outside the reserved apply namespace` |
| `E_APPLY_LOCKED` | `/apply` | `remove the apply lock only after manual recovery confirms target state` |
| `E_TARGET_CHANGED` | `/apply` | `rerun plan after the target state changed` |
| `E_APPLY_TEMP_CONFLICT` | `/apply` | `inspect and remove only the stale temp after manual recovery` |
| `E_APPLY_WRITE` | `/apply` | `fix the target filesystem and rerun plan before applying` |
| `E_APPLY_ROLLBACK` | `/apply` | `stop and recover the target manually before another apply` |

### 5. contract-first 验证矩阵

先提交 tests，证明旧 CLI 不认识 apply 或不能满足写入契约；再提交实现。至少覆盖：

- blank target：合法 internal→lock 和 sibling temp 层级必须成功；精确创建四个文件，内容/hash 正确，无 temp/lock；POSIX 精确验证 mode，Windows 验证 regular/non-reparse、temp/final security descriptor 一致且未显式放宽 ACL；随后 plan 全 NOOP，第二次 apply `NO_CHANGES`。
- existing arbitrary AGENTS：只创建三个 tool-owned 文件，AGENTS 字节/mode/mtime 不变。
- 缺批准、错误摘要、错误 mode、conflict、功能关闭：退出 2，目标树逐字节/mode/mtime 不变。
- plan 与 apply 之间新增/修改/删除目标、预先存在的 symlink/junction、大小写碰撞：fail-closed 且不写入；不模拟威胁模型外的获锁后恶意父目录替换。
- 两个并发 apply：至多一个提交；另一方 `E_APPLY_LOCKED` 或在重算时 `E_TARGET_CHANGED`，最终内容完整。
- internal collision：分别覆盖 final 对 internal dir/lock/temp 的相等与父子碰撞、temp 对 temp 碰撞，以及合法 `internal→lock` 层级；既有空 internal 可复用，既有非空 internal、既有 temp、lock IsExist 和 lock 其他 create failure 分别命中冻结 code。
- 对每个 failure injection point 验证 rollback 后目标树等于调用前；rollback 期间外部改写 final 时保留 lock 并 `E_APPLY_ROLLBACK`。
- Windows amd64、Linux amd64、macOS arm64 跑相同 portable contract tests；Windows junction 与 Unix symlink/权限用例单独报告。
- 新进程仅从目标仓库读取 `AGENTS.md` 和 `instance.json`，能确认 instance ID、阶段事实入口和两个 managed schema 摘要；这只是安装 E2E，不宣称核心假设已验证。

## 拟定任务契约

```yaml
task_id: T-APPLY-001
title: 消费已批准 plan 并以失败回滚方式创建最小治理实例
phase: 1
approval_required: APPLY-001-R1
acceptance_criteria:
  - 人工批准并独立复审 PASS 后转 active；tests 先红后实现
  - apply 绑定完整 plan stdout SHA-256，preflight 与获锁后均重算
  - 只执行 CREATE，保护 project facts，conflict/changed/disabled 全路径零写入
  - 保留内部命名空间、exclusive temp + Sync + hard-link no-replace 发布，禁止覆盖 final
  - 进程内任一步失败逆序回滚；无法安全回滚时保留 lock 并人工止损
  - blank、已有 AGENTS、幂等、守约并发、内部路径碰撞、failure injection、三平台与 fresh-process E2E 通过
  - 不实现 crash auto-recovery、rollback 命令、upgrade、doctor、状态机、trust、签名或发布
  - 仅使用 Go 标准库；test/vet/tidy/build 通过并由独立 Reviewer 复算
allowed_files:
  - AGENTS.md
  - docs/正式项目接入/03-治理实例规范v0.md
  - docs/正式项目接入/21-阶段1可执行Plan合并状态同步任务单.md
  - docs/正式项目接入/22-阶段1最小Apply契约任务单.md
  - docs/正式项目接入/README.md
  - tool/README.md
  - tool/internal/apply/
  - tool/internal/plan/plan.go
  - tool/internal/plan/plan_test.go
  - tool/internal/cli/run.go
  - tool/internal/cli/run_test.go
  - tool/contracttest/plan_test.go
  - .github/workflows/aigov-plan-contract.yml
  - .git/
pre_implementation_search:
  commands:
    - rg -n 'apply|NO_CHANGES|恢复点|rollback|原子|批准' docs/正式项目接入/03-治理实例规范v0.md docs/正式项目接入/12-安装升级回滚对比实验报告.md
    - rg -n 'func Evaluate|buildPlan|readTarget|Operation|Conflict|marshalJSON' tool/internal/plan tool/internal/cli tool/contracttest
    - rg -n 'func apply|atomicWrite|rollback|os\.Rename|os\.Link|O_EXCL|Sync' experiments/installer-lifecycle-matrix tool
  evidence_required: 列出现有 plan 生成字节的唯一来源、可复用 target safety、实验实现的覆盖风险，以及为何正式实现不能复制其 Rename/MkdirAll 行为
doc_type: how-to
doc_purpose: 说明如何批准并安全执行最小 apply，以及失败时何时自动回滚、何时人工止损
```

## 人工批准语义

项目负责人回复：

```text
批准 APPLY-001-R1
```

表示批准受控静默仓库威胁模型、plan 摘要绑定、只执行 CREATE、保留内部命名空间、exclusive lock/temp、hard-link no-replace、owned-file ledger、进程内失败回滚、崩溃后 fail-closed 和上述任务范围；不批准对抗恶意并发写者、自动 crash recovery、rollback 命令、upgrade 或其他治理写入。

## 更新记录

- 2026-11-25：T-PLAN-002 合并 master 后形成 R1 草案，等待独立审查与人工批准。
- 2026-11-25：首次独立审查 FAIL（Critical 1 / Major 4 / Minor 0）；收紧威胁模型，并补齐内部路径碰撞、纯 CREATE 恢复点、错误闭包和平台 mode 语义，等待复审。
- 2026-11-25：后续复审关闭 3 个 Major，最终独立审查 PASS（Critical 0 / Major 0 / Minor 0）；项目负责人明确批准 `APPLY-001-R1`，任务转为 active。

