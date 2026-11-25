---
type: reference
purpose: 冻结已有项目 AGENTS 的显式托管引用块、事实快照、plan 可见性与可回滚边界
audience: 项目负责人、阶段 1 实现 Agent 与后续复核入口能力的协作者
status: active
---

# T-ENTRY-001 R1：已有 AGENTS 最小可发现入口契约

## 用途

修复 `T-E2E-001 e2e-001-a001` 暴露的 `LOCAL_ENTRY_FACT_GAP`：目标项目已有自有 `AGENTS.md` 时，工具继续保护全部原有项目事实，但可在项目负责人显式选择并批准完整 preimage SHA-256 后追加一个最小、机器可识别的托管引用块，使全新 Agent 能从本地入口发现治理实例、当前 phase、核心假设和 instance ID。

本任务只冻结新输入契约、plan/apply/回滚语义并建立黑盒红灯测试，不实现生产代码。D 可复核证据和 E 自动止损缺口必须由后续独立任务处理。

## 当前事实与需要修正的表述

- `0.2-draft` 对已有 project-owned `agent_entry` 固定返回 `NOOP project_fact_preserved`，apply 绝不重写、chmod 或 touch。
- 当前 `.ai-governance/instance.json` 只包含 instance ID、contract/methodology 版本、assurance level、资源路径和 managed-file 摘要；它**不包含** phase 或 core hypothesis。
- 当前生成式 `AGENTS.md` 直接嵌入 phase、core hypothesis、instance ID 和技术栈；已有入口无法获得这份摘要。
- `instance.json.resources` 已声明 `phase_snapshot` 默认路径 `.ai-governance/phase.json`，但当前 plan/apply 不生成该文件。

因此 26 号状态任务中“从 `instance.json` 中发现 phase、core hypothesis 和 instance ID”应按本契约解释为：先从 `instance.json` 取得 instance ID 与 `phase_snapshot` 资源路径，再从 tool-owned phase snapshot 取得 phase 和 core hypothesis。不得把尚不存在的字段当作当前实现事实。

## 候选输入契约

### 1. contract version

安装 manifest 升级为 `0.3-draft`。不得在仍名为 `0.2-draft` 的输入下改变“已有 AGENTS 永远 preserve”的既有语义；旧输入必须 fail-closed，并明确要求重新 plan 和批准 `0.3-draft`。

`layout` 新增必填对象：

```json
{
  "agent_entry_integration": {
    "mode": "preserve-or-generate",
    "approved_sha256": null
  }
}
```

或：

```json
{
  "agent_entry_integration": {
    "mode": "append-managed-reference",
    "approved_sha256": "<existing AGENTS.md raw bytes SHA-256>"
  }
}
```

规则：

- `preserve-or-generate` 保留 `0.2-draft` 行为：缺失入口时生成；已有普通入口时 `NOOP project_fact_preserved`；`approved_sha256` 必须为 `null`。
- `append-managed-reference` 只适用于已存在的普通 UTF-8 文件；`approved_sha256` 必须为 64 位小写十六进制，并与 plan 时原始字节完全一致。
- mode、摘要或字段组合不合法时返回稳定诊断，零写入；不得猜测默认 mode。
- symlink、junction、reparse point、目录、设备文件、含 NUL 或非 UTF-8 内容继续 fail-closed。

### 2. tool-owned phase snapshot

`append-managed-reference` 必须计划创建 `.ai-governance/phase.json`（或 custom layout 中的 `phase_snapshot` 路径），其 canonical JSON + LF 至少包含：

```json
{
  "schema_version": "0.1-draft",
  "instance_id": "inst-example-01",
  "phase": 1,
  "core_hypothesis": "<manifest value>",
  "phase_basis": {
    "assessed_at": "<manifest value>",
    "assessed_by": "<manifest value>",
    "signals": ["<manifest values>"]
  }
}
```

- 所有字段只取自已验证 manifest，不读取或推断项目自然语言。
- `instance.json.managed_files` 必须记录 phase snapshot 的 SHA-256。
- tool-owned phase snapshot 已存在且字节相同时为 `NOOP managed_content_matches`；不同时为 `CONFLICT managed_content_differs`。
- `preserve-or-generate` 不生成 phase snapshot；只有 `append-managed-reference` 生成并管理该文件。

### 3. 托管引用块

托管块不复制 phase 或核心假设，只引用治理实例，避免产生第二份事实：

```md
<!-- aigov:managed-entry:v1 begin -->
AI Agent: after reading this file, read the governance instance manifest at
`<effective instance_manifest path>`, then read the `phase_snapshot` resource
named there. These local files expose the instance ID, current phase, and core
hypothesis. Confirmed project facts in this file take precedence on conflict.
Do not edit this managed block manually.
<!-- aigov:managed-entry:v1 end -->
```

精确字节规则：

- 原 `AGENTS.md` 的全部字节必须是新文件的精确前缀；不得格式化、重排、替换或删除任何原文。
- 文件只含 LF 时追加 LF block，只含 CRLF 时追加 CRLF block；混合换行、孤立 CR 或无法确定的编码 fail-closed。
- 原文件没有末尾换行时先使用已检测的换行风格结束原行，再插入一个空行；已有末尾换行时补足且只补足一个空行。
- 空文件采用 LF；UTF-8 BOM 保持在文件起始位置，不在托管块中重复。
- begin/end 标记必须各恰好一次。完全相同的合法块得到 `NOOP managed_reference_matches`；残缺、重复、版本未知、内容不同或指向其他实例路径时得到 `CONFLICT managed_reference_differs`。

### 4. plan、批准与 ownership

已有入口追加托管块时，plan 必须输出：

```json
{
  "action": "UPDATE",
  "logical_id": "agent_entry",
  "path": "AGENTS.md",
  "ownership": "shared",
  "expected_sha256": "<postimage sha256>",
  "actual_sha256": "<approved preimage sha256>",
  "reason": "managed_reference_append"
}
```

- `shared` 只表示工具拥有标记之间的块；标记外全部内容仍为 project-owned。
- plan stdout 展示完整 preimage/postimage 摘要并继续整体绑定 `plan_sha256`。
- plan 与 apply 之间任一字节、类型、路径身份或托管块状态变化均返回 `E_TARGET_CHANGED`，零写入。
- `UPDATE` 不得被现有“只执行 CREATE”的 apply 静默接受；实现任务必须显式扩展批准、prepared bytes、collision 和 rollback 闭包。

### 5. 可回滚边界

- apply 更新已有入口前必须把完整 preimage 字节、摘要、mode/属性和可恢复时间元数据纳入 owned update ledger。
- postimage 必须通过同目录临时文件、Sync、摘要/属性复核和 no-follow 安全检查发布；具体跨平台 replace 原语由后续实现任务在不降低 `T-APPLY-001` 安全边界的前提下冻结。
- 任一可观察失败必须恢复 preimage 的字节和受支持元数据；无法证明恢复完成时保留 recovery lock，返回 `E_APPLY_ROLLBACK` 并阻止重试。
- 不提供通用文本编辑、任意块替换或无摘要 uninstall。后续显式 rollback/remove 命令必须只在当前块与获批 postimage 完全一致时删除该块。

## 黑盒红灯测试矩阵

测试必须能够编译，并在当前 `0.2-draft` 实现上因能力缺失而失败；不得修改生产实现使其提前变绿。

1. 无 `AGENTS.md` + `preserve-or-generate`：继续产生 generated entry，不回归现有事实摘要。
2. 已有普通 `AGENTS.md` + `preserve-or-generate`：仍为 `NOOP project_fact_preserved`。
3. 已有普通 UTF-8 `AGENTS.md` + `append-managed-reference`：精确 `UPDATE/shared`，preimage/postimage 摘要正确，同时 CREATE phase snapshot。
4. 已有完全一致托管块：agent entry 为 `NOOP managed_reference_matches`。
5. 错误 approved SHA、plan 后内容变化、残缺/重复/异版本块：CONFLICT 或稳定诊断，零写入。
6. LF、CRLF、无末尾换行、UTF-8 BOM、空文件：postimage 字节符合冻结规则，原字节保持精确前缀。
7. 混合换行、孤立 CR、非 UTF-8、NUL、symlink/junction/reparse point、目录：fail-closed。
8. custom `instance_manifest` 与 `phase_snapshot` 路径进入引用块和实例资源，禁止硬编码默认路径。
9. apply 成功、幂等与每个 failure injection point：成功只改变托管块和 tool-owned CREATE；失败恢复完整 preimage。
10. Windows amd64、Linux amd64、macOS arm64 运行相同 portable tests；平台链接与属性用例单独报告。

## 任务契约

```yaml
task_id: T-ENTRY-001
title: 冻结已有 AGENTS 最小可发现入口契约并建立红灯黑盒测试
phase: 1
acceptance_criteria:
  - 0.3-draft 以显式 mode 和 approved_sha256 选择 preserve/generate 或 append-managed-reference，不静默改变 0.2-draft
  - phase/core hypothesis 进入 tool-owned phase snapshot，instance manifest 记录资源路径和 managed hash
  - 托管块不复制治理事实，原 AGENTS 字节保持精确前缀，换行/编码/标记边界确定且 fail-closed
  - plan 对已有入口输出 UPDATE/shared、完整 preimage/postimage SHA-256 和稳定 reason
  - plan 后变化、摘要不符、块冲突或不安全对象全部零写入
  - 可观察失败必须恢复 preimage；无法安全恢复时 E_APPLY_ROLLBACK 并保留 recovery lock
  - 黑盒测试能够编译，并证明当前 0.2 实现尚不满足新契约
  - 不实现 production UPDATE/apply，不修复 evidence gate、runtime、record-attempt、doctor、review gate 或阶段跳转
  - 不读取或修改 F:\\oioi 及 e2e-001-a001 隔离副本
  - changed_files 全部位于 allowed_files，Markdown frontmatter 与 git diff --check 通过
allowed_files:
  - docs/正式项目接入/27-阶段1已有AGENTS最小入口契约任务单.md
  - docs/正式项目接入/README.md
  - tool/contracttest/entry_test.go
  - .git/
pre_implementation_search:
  commands:
    - rg -n 'agent_entry|project_fact_preserved|PreparedFile|CREATE|UPDATE|managed_files|phase_snapshot' tool docs/正式项目接入
    - rg -n 'AGENTS|LOCAL_ENTRY_FACT_GAP|instance.json|core_hypothesis|phase' docs/正式项目接入/25-阶段1真实目标仓库E2E契约任务单.md docs/正式项目接入/26-阶段1真实目标仓库E2E结果同步任务单.md
  evidence_required: 列出现有 preserve 分支、instance manifest 缺少的事实、可复用 target safety/apply ledger，以及 UPDATE 尚无实现的闭包
doc_type: reference
doc_purpose: 让实现者和复核者对已有 AGENTS 的最小托管引用块得到唯一、可测试、可回滚的行为定义
```

## 明确不做

- 不修改 `F:\oioi`，不把隔离副本结果写回源项目。
- 不直接把整个统一 AGENTS 模板覆盖到项目自有入口。
- 不在托管块中复制 phase/core hypothesis，或把项目自然语言推断成治理事实。
- 不在本任务实现生产代码、发布升级器或迁移已有实验实例。
- 不因 B 的契约建立而把核心假设改判为已证明；D/E 仍是独立阻塞项。

## 人工批准点

本契约首次允许工具更新已有 project-owned 文件，属于高风险公共契约。项目负责人已明确回复：

```text
批准 ENTRY-001-R1；phase_snapshot 仅由 append-managed-reference 生成
```

批准表示接受：`0.3-draft` 版本升级、显式 mode + preimage SHA、仅由 `append-managed-reference` 生成的 tool-owned phase snapshot、shared 托管块、UPDATE plan 可见性、失败恢复要求和本任务 allowed_files；不批准生产实现、对源项目执行或阶段跃迁。

## 实现前检索结论

- `tool/internal/plan/plan.go` 的 `instanceManifest` 没有 phase/core hypothesis 字段；生成入口在 `facts` JSON 中单独嵌入这些事实。
- `phase_snapshot` 已是已知资源并进入 `instance.json.resources`，但不在当前 targets 或 managed files 中。
- 当前 project-owned target 只要存在就固定 `NOOP project_fact_preserved`；`PreparedFile` 与 apply 路径只携带和执行 `CREATE`。
- 现有 target safety、完整 plan stdout SHA-256、获锁后重算、internal namespace、failure injection 和 recovery lock 可作为后续 UPDATE 实现基础，但不能证明覆盖发布与原文件恢复已经解决。

## 红灯基线证据

测试使用从 Go 官方分发元数据解析并校验的 `go1.27.4.windows-amd64.zip`；归档 SHA-256 为 `178ab98950ac69ab25069307dc6ade79f3816e414cce95b4da166a115cde1e7e`，解压到系统临时目录，不读取 `e2e-001-a001` 隔离副本中的工具链。

```text
go test -count=1 -run '^TestEntry' ./contracttest          exit 1（预期红灯）
go test -count=1 -run '^$' ./...                           exit 0（全部 package 与新测试可编译）
go vet ./...                                               exit 0
go mod tidy -diff                                          exit 0
go test -count=1 -skip '^TestEntry' ./...                  exit 0（既有测试无回归）
git diff --check                                           exit 0
```

红灯 failure signature 固定为当前实现同时返回：

```text
E_CONTRACT_VERSION /contract_version set contract_version to 0.2-draft
E_UNKNOWN_FIELD /layout/agent_entry_integration remove unknown field /layout/agent_entry_integration
```

这证明测试已启动当前 CLI，且缺口位于尚未实现的 `0.3-draft` 与显式入口集成字段；不是编译失败、fixture 写入失败或生产代码被提前修改。红灯覆盖 preserve/generate、UPDATE/shared、phase snapshot、custom path、换行/BOM/空文件、合法块幂等、非法内容与批准组合、apply 幂等及 plan 后目标变化。failure injection 的完整内部步骤矩阵仍由后续生产实现任务在既有 apply 注入框架上落地。

`changed_files`：

```text
docs/正式项目接入/27-阶段1已有AGENTS最小入口契约任务单.md
docs/正式项目接入/README.md
tool/contracttest/entry_test.go
```

三份文件均在 `allowed_files` 内；没有修改 manifest/schema、plan/apply/CLI 生产实现，没有读取或修改 `F:\oioi` 或先前 E2E 隔离副本。

## 更新记录

- 2026-11-25：建立 R1 draft；冻结候选 `0.3-draft`、phase snapshot、shared 托管块、UPDATE/rollback 边界和红灯测试矩阵，等待项目负责人批准。
- 2026-11-25：项目负责人批准 `ENTRY-001-R1`，并定稿 phase snapshot 仅由 `append-managed-reference` 生成；文档转为 active，允许添加契约红灯测试。
- 2026-11-25：新增可编译的 `entry_test.go` 黑盒矩阵；旧实现以预期的版本/未知字段 signature 红灯，既有测试、vet 与 tidy 无回归。T-ENTRY-001 契约和红灯基线完成，生产实现仍未开始。
