---
type: reference
purpose: 在 T-PLAN-001 合并并通过独立审查后同步项目入口与后续阶段 1 边界
audience: 阶段 1 主 Agent、人工 Reviewer 与维护 aigov CLI 的后续 Agent
status: active
---

# T-STATE-002：同步首个只读 Plan 完成状态

## 用途

仅在 `T-PLAN-001` 的 contract-first 提交链、最终验证和独立审查均已通过并合并到 `master` 后执行。它修正项目入口中的生命周期事实，避免后续会话把最小只读 `plan` 误判为尚未开始，或直接越过可审核计划去实现写入型 `apply`。

## 任务契约

```yaml
task_id: T-STATE-002
title: 同步首个只读 plan 完成后的项目状态
phase: 1
acceptance_criteria:
  - AGENTS.md 明确 T-PLAN-001 已测试先行实现、合并并通过独立审查
  - AGENTS.md 仍明确核心假设尚未端到端验证，下一步只扩展只读安装计划
  - 正式项目接入 README 可导航到本状态同步任务，并把 17 描述为已完成契约事实
  - 17 记录提交链、命令退出码、独立审查结论和明确未实现边界
  - 不修改 decisions.md、03 治理实例规范、tool/、PoC 或实验目录
  - 提交后工作树 clean，且全部 changed_files 位于 allowed_files
allowed_files:
  - AGENTS.md
  - docs/正式项目接入/README.md
  - docs/正式项目接入/17-阶段1首个Plan契约任务单.md
  - docs/正式项目接入/18-阶段1Plan完成状态同步任务单.md
  - .git/
pre_implementation_search:
  commands:
    - rg -n 'T-PLAN-001|阶段 1|最小只读|apply|核心假设' AGENTS.md docs/decisions.md docs/正式项目接入 tool/README.md
    - git log --oneline --decorate -6
    - git status --short --branch
  evidence_required: 区分已完成的只读校验/计划能力、仍未完成的可执行文件变更计划、apply 与端到端核心假设验证
doc_type: reference
doc_purpose: 为后续 Agent 提供 T-PLAN-001 合并后的准确生命周期状态
```

## 已确认事实

- `95326a6` 在产品实现前冻结稳定诊断契约。
- `2479ba0` 先提交 schema、fixtures 与黑盒 contract tests，并以旧 CLI 的 `unknown command: plan` 形成有效红灯。
- `979eaf6` 实现最小只读 `plan`；最终测试、静态检查、构建和核心黑盒命令均通过。
- 最终独立审查为 PASS，Critical/Major/Minor 均为 0；`979eaf6` 已 fast-forward 合并到 `master`。
- 当前 `plan` 负责严格 manifest 校验与生命周期 `SKIP`，尚未生成可由写入命令消费的完整文件变更计划。

## 后续边界

- 下一项功能任务必须先用 contract tests 冻结 `CREATE / NOOP / SKIP / CONFLICT` 的只读输出、目标路径、所有权、内容摘要、项目事实保护和跨平台确定性。
- 在上述计划契约通过前，不实现 `apply`；之后的 `apply` 也必须作为独立 contract-first 任务处理幂等、原子写入、失败回滚和端到端验证。
- 不因 `T-PLAN-001` 完成而宣称核心假设已验证，也不进入阶段 2。

## 更新记录

- 2026-11-25：根据 `T-PLAN-001` 合并与独立 PASS 创建状态同步任务。
