---
type: reference
purpose: 所有 AI 会话启动时强制读取的唯一行为总纲
audience: 所有参与本项目的 AI Agent 与人类协作者
status: active
---

# AGENTS.md — 项目 AI 协作总纲

> 本文件是每个 AI 会话开始时**必须最先完整读取**的唯一总纲。总纲来自文档 1 模板（`docs/01-AGENTS模板.md`）。
> 体系全貌见 [docs/00-总纲.md](docs/00-总纲.md)。

## 1. 项目基本信息
- 项目名：AI 长期开发复杂项目
- 当前阶段：阶段 1（`e2e-001-a001` 的历史判定仍为 A/C/F PASS、B `FAIL (LOCAL_ENTRY_FACT_GAP)`、D `FAIL (NOT_VERIFIABLE)`、E `CAPABILITY_MISSING`，整体 `CORE_HYPOTHESIS_NOT_PROVEN`；`T-ENTRY-001/002` 已落地 B 的修复能力，`T-EVIDENCE-001/002` 已落地 D 的 begin/collect、无 Git 内容基线、命令摘要、变更清单与 sealed manifest 能力，两者均尚待新 E2E attempt 复验，不改写 a001；E 仍为 `CAPABILITY_MISSING`，不运行 a002，不进入阶段 2；下一任务为 `T-HALT-001` 同一 failure signature 连续三次自动止损契约）
- 技术栈：Markdown 方法论；正式工具层使用 Go `1.27.4` 构建的原生可执行文件；GitHub Releases 是 canonical 分发渠道；Python 3 标准库仅用于 PoC 和回归参考；Node SEA 仅保留为备选
- 核心假设（一句话）：让一个全新的 AI Agent 仅通过目标项目仓库中的本地治理实例，就能识别项目阶段、遵守任务边界、提供可验证证据、接受独立审查，并在失败时自动止损；治理强度随项目成熟度动态升级。

## 2. 强制阅读顺序
1. 本文件
2. `docs/decisions.md`（若存在则读取；若不存在，先判断是否尚未完成阶段 0 的“核心假设 → 反向验证调研 → 人工选型定稿”。仅在选型定稿后创建，禁止用空文件占位；选型已定稿但文件仍不存在，才视为缺失）
3. `docs/00-总纲.md`（当前阶段只做该阶段允许的动作）
4. 与任务相关的 `docs/` 下 `status: active` 状态文档

发现预期文件不存在时，必须先判断其生命周期状态：尚未到创建时机、当前阶段必须存在、或已经废弃；未经判断不得直接报告“缺失”。

## 3. 开发行为规则
- 任务未写验收标准 → 不开始实现。
- 需求模糊 → 先记录假设，不自行扩大范围。
- 接口/公共 API 变更 → 必须同步更新文档与调用方。
- 新增功能前必须检索已有组件/函数（见任务单 `pre_implementation_search` 字段，模板见 docs/08-模板库.md）。
- 只改任务单 `allowed_files` 范围内的文件；越界需人工批准。

## 4. 禁止事项
- 禁止引入未使用依赖。
- 禁止关键逻辑写死在 UI 层。
- 禁止新增无描述的 `stylelint-disable` / `eslint-disable`。
- 禁止只复述代码签名、不写用法的文档（见 docs/06-文档写作规范.md）。
- 禁止在未通过端到端验证时声称“完成”。

## 5. 文档规范
- 文档类型只能是四种：how-to / adr / runbook / reference。
- 每份文档开头必须有 type / purpose / audience / status。
- 缺 purpose 或 audience 的文档不得进入仓库。
- 非 `active` 状态的文档不得作为实现依据。

## 6. 审查与证据
- 阶段 1 不设人工/独立 Reviewer 合并门禁；作者可以执行机械自检，但不得把自检冒充独立审查。阶段 2 起作者不能审自己的产出。
- “完成”必须附带：命令、退出码、`changed_files`、逐条验收对应。
- 审查清单与证据标准见 docs/07-审查清单与证据标准.md。

## 7. 常驻规则
- 同一 failure signature 连续 3 次失败 → 停止，升级人工。
- 同一错误被纠正第二次 → 立即追加到本文件第 8 节。

## 8. 踩坑规则（持续追加，每条一句话）
- PowerShell 中不得将裸 `foreach (...) { ... }` 语句直接接管道；必须先收集到数组变量，再单独通过管道输出。
- 任何进入仓库的 Markdown（包括实验目录中的 canonical `README.md`）都必须以合法的 type / purpose / audience / status frontmatter 开头，禁止把实验说明误判为文档规范例外。
