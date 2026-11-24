---
type: runbook
purpose: 在治理实例规范和交付选型获批后，按低返工顺序实现自动安装、自动校验与完整状态机
audience: 主 Agent / Orchestrator / 工具层 Executor / 独立 Reviewer
status: draft
---

# 工具层开发 Runbook

## 触发条件

只有以下条件全部满足才执行本 Runbook：

1. [现状审计](./02-现状审计与返工风险.md) 已独立 Review 并转为 active；
2. [治理实例规范 v0](./03-治理实例规范v0.md) 已人工定稿并转为 active；
3. [交付方式选型](./04-交付方式选型任务书.md) 已完成带来源调研、实验和独立审查；
4. `docs/decisions.md` 已在选型定稿后创建；
5. 仓库已有可恢复的版本控制基线。

任一不满足，不开始正式工具代码。

## 不适用

- 只想验证单条治理规则时，使用现有 PoC，不启动完整产品化流程。
- 正式选型尚未完成时，不用本 Runbook 推测技术栈。
- 目标只是给一个项目手工加 `AGENTS.md` 时，按接入 README 操作即可。

## 总体顺序

```text
冻结契约
→ 完成选型与决策
→ 建 canonical source 和 schema
→ 先做只读 plan/doctor
→ 再做可回滚 apply/upgrade
→ 接入 Git/CI 自动校验
→ 实现事件驱动状态机
→ 两类项目 + 全新 Agent E2E
→ 发布 v0
```

不得把顺序改成“先写生成器，再补契约”。

## 步骤 0：建立可恢复基线

### 动作

1. 确认版本控制状态；当前仓库若仍无 `.git`，先由人工决定初始化位置和远端策略。
2. 清除不应进入正式源的缓存，如 `__pycache__/`，并配置忽略规则。
3. 对现有 PoC 打不可变基线标签或归档快照，后续只作为回归夹具。
4. 为工具层建立首张正式任务单，写明 allowed_files 和验收标准。

### 验证

```powershell
git status --short
git rev-parse --show-toplevel
rg --files | rg "__pycache__|\.pyc$"
```

### 放行条件

- 能恢复到产品化开发前的 PoC 状态；
- 缓存和运行产物不会污染源代码；
- Reviewer 能看到准确 diff。

## 步骤 1：冻结治理实例契约

### 动作

1. 对规范 v0 做逐字段 Review；
2. 裁决目录名、所有权、生命周期和状态事件；
3. 把未决项写成 ADR 问题，不留“实现时再说”；
4. 定义 `contract_version` 的兼容规则。

### 验证

- 每个生成文件都有 owner、创建条件、更新规则和回滚方式；
- `decisions.md`、`DESIGN.md` 不会被无条件创建；
- 项目事实与工具管理文件没有重叠所有权；
- 状态能由审计事件重放。

## 步骤 2：完成交付方式选型

### 动作

1. 按选型任务书执行候选调研；
2. 所有技术事实附官方 URL；
3. 做安装、离线、升级、信任和跨平台实验；
4. 独立 Reviewer 重跑硬门槛实验；
5. 人工定稿后创建 `docs/decisions.md`。

### 验证

- 能回答为什么选、为什么排除、什么情况下推翻；
- 许可证和目标平台有原始来源；
- 领先方案没有硬门槛失败；
- Python PoC 没有被当作默认理由。

## 步骤 3：建立 canonical source 与 schema

### 建议职责结构

具体目录由选型决定，但必须能映射以下职责：

```text
canonical/
├─ templates/       # 只保存可生成内容
├─ schemas/         # 正式 JSON schema
├─ runtime/         # 目标项目本地运行时源
├─ installer/       # plan/apply/doctor/upgrade
└─ tests/
   ├─ unit/
   ├─ contract/
   └─ fixtures/     # 阶段 0 新项目、阶段 2 既有项目
```

### 动作

1. 从 PoC 提取行为，不复制整个目录；
2. 先写 schema 和 contract tests，再迁移 gate 行为；
3. 为每个 schema 写有效、缺字段、越界版本和迁移样例；
4. runtime 内不得包含固定项目名、信任 token 或 Agent ID。

### 验证

- 全仓只有一个 canonical gate 实现源；
- fixture 是生成结果，不是手工维护的第二实现；
- schema 版本与 runtime 版本可以独立诊断；
- PoC 的 17 项关键行为全部迁入 contract tests。

## 步骤 4：先实现只读 `plan` 和 `doctor`

### 动作

1. `plan` 读取 manifest 和目标项目，只输出拟创建、修改、跳过和冲突项；
2. `doctor` 检查契约版本、文件所有权、生命周期、runtime 哈希、schema、信任边界和阶段一致性；
3. 所有错误使用稳定错误码和可操作修复建议；
4. 两个命令禁止写入目标项目。

### 验收

- 阶段 0 且 `selection_finalized=false` 明确跳过 `decisions.md`；
- 已有 `AGENTS.md` 显示三方合并计划，不显示覆盖；
- 重复运行输出稳定；
- 对 PoC 漂移、缺 schema、被修改 runtime 能给出准确诊断。

## 步骤 5：实现可回滚 `apply`

### 动作

1. 只接受已批准 plan 的哈希；
2. 写入前创建恢复点；
3. 使用临时文件和原子替换；
4. 中途失败恢复全部文件；
5. 第二次相同 apply 返回 `NO_CHANGES`。

### 验收

| 场景 | 期望 |
|---|---|
| 空白阶段 0 项目首次安装 | 只生成阶段 0 最小文件 |
| 同 manifest 第二次安装 | 零 diff，`NO_CHANGES` |
| 已有 AGENTS 项目 | 停止并给三方差异，不覆盖 |
| 写到一半模拟失败 | 完整恢复写入前哈希 |
| 生命周期未到 | 不创建空文件 |

## 步骤 6：实现升级与迁移

### 动作

1. `upgrade-plan` 对比旧模板、项目当前文件和新模板；
2. 工具管理文件可升级，项目事实只能产生建议 patch；
3. schema 破坏性变更逐版本迁移，不允许跨版本猜测；
4. runtime 安全补丁保留回滚版本。

### 验收

- 项目修改过 `AGENTS.md` 后升级仍保留修改；
- runtime v0→v1 可诊断、迁移和回滚；
- 跳过版本时按迁移链执行；
- 失败升级不会留下混合版本实例。

## 步骤 7：实现通用自动校验

### 动作

1. 校验 task/evidence/review/approval/state schema；
2. 比对 `changed_files` 与 Git diff；
3. 重算 stdout 和关键产物哈希；
4. 从受信运行上下文读取 Agent/run 身份；
5. 提供本地命令和 CI adapter；
6. 无法强验证时明确输出降级级别。

### 验收

- 漏报、越界、路径穿越和绝对路径全部失败；
- 自报 Reviewer ID 不能替代受信 run ID；
- CI 与本地对同一证据得出相同结果；
- 敏感输出不会未经处理进入长期证据。

## 步骤 8：实现事件驱动状态机

### 动作

1. 实现规范中的全部合法跃迁；
2. 状态只通过命令跃迁，禁止直接改 state；
3. 每次跃迁追加事件，当前状态由事件重放；
4. REVIEW_REJECT 返回 EXECUTING 并携带可操作项；
5. 三次同签名失败立即进入 HALTED；
6. 阶段 4 累计失败上限和安全模式接入同一状态机。

### 验收

```text
PLANNED → EXECUTING → REVIEW_PENDING → VERIFIED → CLOSED
                    ↘ REJECT → EXECUTING
任意非终态 → HALTED_FOR_HUMAN
```

- 非法跃迁失败且不写事件；
- 删除缓存状态后能从事件完整恢复；
- HALTED 任务未经人工恢复批准不能继续；
- 重复提交同一事件保持幂等。

## 步骤 9：端到端试点

至少运行两个隔离 fixture：

### Fixture A：新建阶段 0 项目

- 无 decisions、无 DESIGN、无 CI；
- 验证最小生成、生命周期跳过、人工骨架放行。

### Fixture B：既有阶段 2 项目

- 已有 AGENTS、CI、业务代码和本地规则；
- 验证三方冲突、Git diff、独立 Reviewer 和升级。

两个 fixture 都要让全新 Executor 和独立 Reviewer 只读取目标仓库完成任务。安装器源仓库不可作为运行时依赖。

## 步骤 10：发布 v0

### 放行清单

- 所有硬门槛实验通过；
- 三平台测试通过或明确缩小支持范围；
- 安装/升级/回滚 E2E 通过；
- 完整状态机和自动止损通过；
- 独立 Reviewer 无 Critical；
- 文档从 draft 转 active；
- `decisions.md`、版本说明和推翻条件已更新。

## 失败与回滚

- 同一 failure signature 三次：停止当前实现，保存 failure memory，升级人工。
- 安装器造成项目文件丢失或覆盖：立即停止全部产品化工作，从恢复点还原，新增 Critical 回归测试。
- schema/实例契约需要破坏性修改：不在原版本上静默修改，提升 contract major version。
- 信任边界无法在目标平台实现：降级声明并重新评估候选，禁止用文档承诺代替机器保证。

## 完成证据

每个步骤必须附：

- 任务单；
- 实际 `changed_files` 与 Git diff；
- 命令、退出码和关键 stdout 哈希；
- 验收逐条映射；
- 独立 Reviewer 决定；
- 失败时的恢复证据。

## 更新记录

- 2026-11-24：建立从契约冻结到 v0 发布的低返工开发顺序。
