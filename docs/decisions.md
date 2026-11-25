---
type: adr
purpose: 记录已经由项目负责人定稿、后续 AI Agent 不得自行推翻的项目级决策
audience: 所有参与本项目的 AI Agent、Reviewer 与项目负责人
status: active
---

# 项目决策记录

## DEC-2026-11-24-001：正式工具层选择 Go

- 状态：已接受
- 决策日期：2026-11-24
- 决策人：项目负责人
- 独立证据复核：`/root/independent_reviewer` 对 T-DELIVERY-005 修正版结论为 PASS（Critical 0 / Major 0 / Minor 0）
- 决策范围：治理实例安装器、自动校验器、状态机及其跨平台 CLI 的正式实现语言与交付载体

### 决定

正式工具层使用 **Go 原生可执行文件**。Node SEA 不作为 v0 的正式交付载体；Python 标准库实现继续只作为 PoC 和回归参考，不直接演化为正式产品源码。

本决定只冻结语言和交付载体，不代表正式安装器已经完成，也不自动冻结 Go 最低版本、依赖清单、代码签名方案、发布渠道或正式源码目录。这些内容必须在对应任务契约和验收证据完成后另行定稿。

### 选择理由

1. Go 与 Node SEA 在 Windows、Linux、macOS 上运行相同生命周期黑盒测试，6/6 组合均为 9/9 PASS，功能能力没有形成 Go 的硬门槛缺口。
2. Go 实验产物约 2.7–3.0 MB；Node SEA 约 100–151 MB。Node SEA 是 Go 的约 33.6–53.2 倍，会扩大下载、缓存、补丁传播和仓库存储成本。
3. Go 使用单一原生构建命令生成平台可执行文件；Node SEA 还需要维护平台对应的 SEA 配置并随产物携带 Node 运行时。
4. Go 与 Node SEA 都尚未通过可信代码签名和公证，因此签名边界没有给 Node SEA 带来足以抵消体积与维护成本的优势。
5. 治理实例契约已经验证 Go 能实现 `plan`、`apply`、幂等、`upgrade`、冲突停止、`rollback` 和 `doctor`，并能保护目标项目已有事实。

证据：

- [云端三平台原生运行实验报告](./正式项目接入/10-云端三平台运行实验报告.md)
- [安装、升级与回滚对比实验报告](./正式项目接入/12-安装升级回滚对比实验报告.md)
- [Go command 官方文档](https://pkg.go.dev/cmd/go#hdr-Compile_packages_and_dependencies)

### 排除方案

- **Node SEA**：没有功能性失败，保留为备选方案；v0 不采用的原因是原生产物大约高出 33.6–53.2 倍、包含完整 Node 运行时、需要额外平台配置，并增加运行时安全补丁传播责任。
- **Python zipapp / 依赖系统 Python**：不作为唯一正式运行时，因为不能保证目标 Windows、Linux、macOS 环境都预装兼容 Python，无法满足自包含交付边界。
- **直接产品化现有 Python PoC**：PoC 用于验证行为，不是 canonical source；直接扩建会把实验结构和临时约束固化到正式产品中。

### 已接受的风险

- Windows 产物尚无 Authenticode 签名；macOS 仅见 ad hoc linker signature，尚无 Developer ID 和 notarization。
- 当前实验没有证明可重现构建；Go 默认嵌入的 VCS 信息可能使不同 commit 的产物哈希变化。
- RFC 8785 JCS、正式 schema、可信身份、事件状态机、安装器全量 E2E 和发布渠道仍待正式开发验证。
- 当前工作站没有本地 Go 工具链；已有 Go 证据来自受控的三平台 GitHub Actions 原生 runner。

### 推翻条件

只有出现新的可复核证据并再次获得项目负责人批准，才允许推翻本决定。至少满足以下任一条件：

1. Go 无法通过正式契约测试、可信签名、公证或目标平台发布硬门槛，而备选方案能够通过；
2. Go 的依赖、安全补丁或供应链维护成本经同口径实验持续高于 Node SEA；
3. 目标平台范围发生变化，Go 不再能够覆盖必须支持的平台；
4. 治理协议需要的关键能力无法在 Go 中可靠实现，且独立 Reviewer 能复现该阻塞；
5. 使用与本次相同或更严格的跨平台矩阵重新比较后，Node SEA 或新候选在总体交付成本上形成明确优势。

任何 AI Agent 不得仅凭偏好、新会话上下文或未经验证的“更熟悉某技术栈”自行改回 Node、Python或其他语言。

### 后续约束

- 正式开发从 Go canonical source 和 contract tests 开始，不复制整个 PoC 目录。
- 第一张正式开发任务单必须先建立可恢复的版本控制基线，并明确 `allowed_files`、验收标准和实现前检索。
- 在可信签名实验通过前，不得宣称正式发布已可安全分发。
- 对本决定的任何修改都必须追加新决策记录，不得静默改写本条历史。

## DEC-2026-11-24-002：v0 使用 GitHub Releases 分发

- 状态：已接受
- 决策日期：2026-11-24
- 决策人：项目负责人
- 批准 ID：`PHASE0-CLOSE-2026-11-24-R1`
- 独立证据复核：`/root/independent_reviewer` 对最终定稿包结论为 PASS（Critical 0 / Major 0 / Minor 0）
- 决策范围：v0 构建工具链、canonical 分发渠道、支持平台、发布资产和可信稳定发布门槛

### 决定

v0 固定使用 Go `1.27.4` 构建 Windows amd64、Linux amd64（glibc）和 macOS arm64 原生产物，以 GitHub Releases 作为 canonical artifact source。Windows 发布 `.zip`、Linux 发布 `.tar.gz`、macOS 发布可 notarize/staple 的 `.dmg`，并同时发布 `SHA256SUMS`、release manifest、变更说明和每个资产的 provenance attestation。

稳定发布必须满足：公开仓库或已验证的 GitHub Enterprise Cloud attestation 资格；Windows SHA-256 Authenticode、RFC 3161 时间戳和发布者身份验证；macOS Developer ID、hardened runtime、notarization、stapling 与 Gatekeeper 验证。任一门禁或凭证缺失时只允许 draft/pre-release，不得称为可信稳定发布。

v0 不以 `curl | shell`、自更新器或 winget/Homebrew/apt 作为主入口。用户或受控 Agent 下载明确版本并验证 manifest、摘要、provenance 与平台签名后运行 `plan`。系统包管理器以后可以作为镜像入口，但不得形成第二套版本事实。

完整决策和可执行验收见[阶段 0 最终定稿包](./正式项目接入/16-阶段0最终定稿包.md)。

### 推翻条件

只有出现新的可复核证据并获得项目负责人批准，才允许改变 canonical 渠道。包括：GitHub Releases 无法满足组织合规或区域可用性；目标平台范围改变；artifact attestation 资格长期无法获得；受控镜像能保持相同 manifest、摘要和 provenance 且显著降低交付风险。

## DEC-2026-11-24-003：Go 骨架通过人工放行并进入阶段 1

- 状态：已接受
- 决策日期：2026-11-24
- 决策人：项目负责人
- 批准 ID：`PHASE0-CLOSE-2026-11-24-R1`
- 决策范围：阶段 0 完成状态、正式源码骨架和阶段 1 的首项工作边界

### 决定

项目负责人确认 `tool/` 的最小 CLI、测试入口和零第三方依赖结构可以承载后续开发。阶段 0 完成，项目进入阶段 1；首项工作必须先用黑盒 contract tests 冻结最小只读 `plan` 行为，再写实现。

该放行不表示安装器、自动校验器、状态机、可信签名发布或治理实例接入已经完成。阶段 1 只验证核心路径，不提前启用阶段 2–3 的多角色状态机和重型证据治理。

### 推翻条件

若 contract tests 证明当前目录阻碍跨平台构建、测试隔离或公共命令稳定性，可通过新 ADR 调整骨架；不得为了复用实验代码而直接把 Python PoC 或 installer-lifecycle 候选复制进正式源码。

## DEC-2026-11-25-004：安装 Manifest 0.2 使用显式实例身份

- 状态：已接受
- 决策日期：2026-11-25
- 决策人：项目负责人
- 决策范围：安装 manifest 的精确 contract 版本、实例身份输入及其确定性边界

### 决定

安装 manifest 从 `0.1-draft` 升级为 `0.2-draft`，根对象新增必填 `instance_id`。该值由项目负责人或受控编排器在调用 `plan` 前显式分配，固定匹配 `^inst-[A-Za-z0-9][A-Za-z0-9._-]{2,122}$`；工具不得随机生成、从路径/项目名称推导或在缺失时填默认值。

该变更只升级安装输入的 `contract_version`。事件、批准信封和 trust 协议的 `schema_version: 0.1-draft` 不变。当前尚无正式 `apply` 产生的已安装实例，因此不提供 `0.1-draft` 自动迁移或兼容猜测；旧输入必须 fail-closed 并提示升级到 `0.2-draft`。

### 选择理由

可执行安装 plan 必须在写入前给出 `.ai-governance/instance.json` 的确定目标字节和 SHA-256。plan 内随机生成身份会使相同输入复跑漂移；从本地绝对路径派生会使跨机器结果不同；从项目名称派生不能保证身份唯一。显式输入使身份进入可审核、可重放的调用边界，同时不把格式校验冒充全局唯一性证明。

### 推翻条件

只有新的可复核证据证明外部身份分配无法满足非交互安装、复制项目重新实例化或跨平台重放要求，并获得项目负责人新批准，才允许改变该边界。任何实现不得仅为减少一个输入字段而恢复随机或隐式身份。

## DEC-2026-11-25-005：阶段 1 取消人工审核合并门禁

- 状态：已接受
- 决策日期：2026-11-25
- 决策人：项目负责人
- 决策范围：阶段 1 任务的合并门禁与阶段 2 审核职责分离边界

### 决定

项目初创且仍处阶段 1 时，不要求人工或独立 Reviewer 审核作为合并前置条件。单 Agent 在任务契约、`allowed_files`、自动检查、命令退出码、`changed_files` 和逐条验收证据全部满足后，可以直接合并。

作者执行的机械自检不得标记为独立审查。取消审核门禁不取消项目负责人对高风险契约、不可逆项目决策和阶段跃迁的人工批准，也不降低同一 failure signature 连续三次后的停止要求。项目进入阶段 2 后，恢复作者与 Reviewer 分离，作者不得审自己的产出。

### 选择理由

阶段 1 的目标是以最低治理负担验证核心假设。当前只有单 Agent、尚无真实外部用户或阶段 2 升级信号；继续把独立审核设为所有任务的硬门禁，会在状态同步和小步验证上产生高于风险收益的流程成本。自动检查与可复核证据仍能保留基本事实边界，重大授权继续由项目负责人掌握。

### 推翻条件

出现第一个真实外部用户、首次线上故障、代码约 2 万行、模块超过 5 个，或项目负责人提前要求恢复审核时，重新评估阶段并恢复独立 Reviewer 门禁。进入阶段 2 后本决定自动停止适用，无需另行豁免。

## DEC-2026-11-25-006：已有 AGENTS 使用获批托管引用块

- 状态：已接受
- 决策日期：2026-11-25
- 决策人：项目负责人
- 批准 ID：`ENTRY-001-R1`
- 批准原文：`批准 ENTRY-001-R1；phase_snapshot 仅由 append-managed-reference 生成`
- 决策范围：安装 manifest 版本、已有 Agent 入口的最小接入方式，以及安装期 phase snapshot 的生成边界

### 决定

安装 manifest 升级为精确版本 `0.3-draft`，新增必填 `layout.agent_entry_integration`。`preserve-or-generate` 保持原行为且 `approved_sha256` 必须为 null；已有 `AGENTS.md` 是项目事实，plan 只输出 `NOOP project_fact_preserved`，不生成 phase snapshot。

只有 `append-managed-reference` 可以修改已有入口。调用方必须提供获批 preimage 的 SHA-256；工具校验普通 UTF-8、NUL、换行风格和摘要后，仅在原始字节末尾追加一个有稳定边界标记的最小引用块。plan 将其表示为 `UPDATE/shared`，原始字节必须是 postimage 的精确前缀。该模式同时生成 tool-owned `.ai-governance/phase.json`，并在 instance manifest 的 `managed_files` 中记录其摘要；其他模式不得生成该文件。

apply 必须在 exclusive lock 内重算并重新核对 preimage。UPDATE 使用同目录 temp 和显式 backup，ledger 记录原字节、preimage/postimage 摘要、mode 与 mtime；可观察失败恢复原状态，不能确认安全恢复时保留 recovery lock 并返回 `E_APPLY_ROLLBACK`。外部修改后的 postimage 不得被回滚逻辑删除。

### 选择理由

阶段 1 E2E 已证明新 Agent 无法只靠目标仓库发现 phase、核心假设和 instance manifest。对已有入口整份替换会越过项目事实所有权；只写 instance 又不能建立可发现路径。获批 preimage 上的最小 append-only 引用块同时保留原入口、建立本地发现链，并给 UPDATE 提供可审核的前后摘要和回滚边界。

### 范围限制与推翻条件

本决定不补 evidence gate、runtime、自动三次止损、doctor、review gate，也不证明强杀/掉电恢复或跨平台 CI 已通过。若后续证据证明托管块无法与主流 Agent 入口规则共存、目标平台无法可靠实现同目录 backup/rename，或 phase 事实迁移到另一已批准本地入口，可通过新 ADR 和契约版本升级替换；不得静默扩大托管块或覆盖项目事实。

## DEC-2026-11-25-007：安装本地 runtime 并以不可变 attempt ledger 自动止损

- 状态：已接受
- 决策日期：2026-11-25
- 决策人：项目负责人
- 批准 ID：`HALT-001-R1`
- 决策范围：安装 manifest `0.4-draft`、目标项目本地 runtime、阶段 1 failure-attempt 事实与同签名三次停止

### 决定

安装 manifest 升级为精确 `0.4-draft`。plan/apply 把当前平台正在运行的原生 `aigov` 字节作为 tool-owned `runtime_entrypoint` 安装到目标项目，并把 SHA-256 写入 instance manifest；不下载或猜测其他平台产物。

安装后的 runtime 提供 `record-attempt` 与只读 `halt-status`。每个 task 的 `aNNN` attempt 按严格连续顺序写入不可覆盖、带自身内容摘要的 canonical ledger；state cache 只由已验证 ledger 重建。成功清零连续计数，不同失败签名从 1 重计，同一签名第三次失败持久化后进入 `HALTED_FOR_HUMAN` 并返回 3，第四次及以后返回 `E_TASK_HALTED` 且零写入。

### 范围限制与推翻条件

本决定不提供 Agent 可调用的 unhalt/reset，不实现阶段 4 不同签名总失败上限或完整任务状态机，也不证明三平台 CI 或 E2E 已通过。若跨平台文件系统证据证明当前 runtime 发布或 ledger no-replace 无法满足原子性，或后续可信批准协议需要替换人工恢复边界，必须通过新契约、迁移计划与项目负责人批准调整；不得静默删除既有 attempt 事实。

