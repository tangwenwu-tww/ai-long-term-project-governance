---
type: reference
purpose: 定义 minimal-target 中所有 AI Agent 的本地启动协议与硬约束
audience: 在 minimal-target 中执行任务的所有 AI Agent
status: active
---

# AGENTS.md — minimal-target 本地治理入口

## 项目事实

- 当前阶段：阶段 3
- 技术载体：Python 标准库 PoC
- 核心假设：全新 Agent 仅读取本目标仓库即可遵守阶段化治理协议。

## 启动顺序

1. 完整读取本文件。
2. 读取 `.ai-governance/governance.json`、`.ai-governance/maturity.json`，运行 `gate.py inspect` 与 `gate.py assess-phase`。
3. 读取指定任务 JSON；任务校验通过前不得实现。
4. 只读取与任务相关且 `status: active` 的本地文档。
5. `docs/decisions.md` 不存在是预期状态：本 PoC 尚未完成正式工具选型，禁止创建空文件占位。

## 执行规则

- 只修改任务 `allowed_files` 中的文件。
- 协议产物例外：Executor 只能额外写入 `evidence/<task_id>*`，Reviewer 只能额外写入 `reviews/<task_id>*`；这些文件不得夹带业务实现，且不计入任务的 `changed_files`。
- 实现前执行并记录 `pre_implementation_search.commands`。
- 完成声明必须附任务、证据和独立审查记录，并通过本地 `gate.py audit`。
- 阶段 2 起 Reviewer 的 Agent ID 和会话 ID 都必须与作者不同。
- 同一 failure signature 连续失败 3 次后停止，等待人工处理。
- 项目成熟度信号允许升级时，也只能通过 `gate.py promote --approval-file <人工批准文件>` 逐级晋升；批准必须绑定具体 `from_phase → to_phase` 并通过 `.ai-governance/trust.json` 的本地信任锚验证。Agent 不得直接改写阶段或信任锚。
