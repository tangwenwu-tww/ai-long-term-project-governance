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

