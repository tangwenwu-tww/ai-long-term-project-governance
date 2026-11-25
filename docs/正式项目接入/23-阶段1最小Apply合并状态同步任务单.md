---
type: reference
purpose: 记录 T-APPLY-001 已通过三平台 CI 并合并 master 的事实，并冻结下一任务为真实目标仓库核心假设验证
audience: 项目负责人、阶段 1 Executor、Reviewer 与后续接入 Agent
status: active
---

# T-STATE-004：最小 Apply 合并状态同步

## 用途

在 T-APPLY-001 合并后，用本任务同步项目入口中的完成状态，避免后续 Agent 继续把三平台 workflow 或人工合并误判为待办。本任务只更新状态事实和下一任务边界，不修改产品代码或宣称核心假设已经验证。

## 任务契约

```yaml
task_id: T-STATE-004
title: 同步最小 Apply 的三平台通过与合并状态
phase: 1
acceptance_criteria:
  - 本地 master 以 fast-forward 同步到 GitHub 合并提交 5e0dedd
  - AGENTS.md、接入 README 与 T-APPLY-001 任务单一致记录 PR #1、6/6 检查和人工合并已完成
  - 项目仍明确处于阶段 1，核心假设尚未在真实目标仓库端到端验证
  - 下一任务只冻结为 T-E2E-001 契约，不在本任务实现或运行真实目标仓库验证
  - changed_files 与 allowed_files 完全一致，Markdown frontmatter 和 diff 检查通过
allowed_files:
  - AGENTS.md
  - docs/正式项目接入/README.md
  - docs/正式项目接入/22-阶段1最小Apply契约任务单.md
  - docs/正式项目接入/23-阶段1最小Apply合并状态同步任务单.md
  - .git/
pre_implementation_search:
  commands:
    - rg -n 'T-APPLY-001|等待三平台|尚无 Linux/macOS|人工合并|核心假设' AGENTS.md docs/正式项目接入/README.md docs/正式项目接入/22-阶段1最小Apply契约任务单.md
    - rg -n '^# T-STATE-|下一任务边界|changed_files' docs/正式项目接入/18-阶段1Plan完成状态同步任务单.md docs/正式项目接入/21-阶段1可执行Plan合并状态同步任务单.md
  evidence_required: 列出所有过时状态位置、前序状态同步任务的编号与边界，并确认本任务不改产品代码
doc_type: reference
doc_purpose: 让后续 Agent 识别 T-APPLY-001 已合并、项目仍处阶段 1，以及下一项只允许建立真实目标仓库 E2E 契约
```

## 已确认外部事实

- GitHub PR `#1` 已合并，合并提交为 `5e0dedd7b3815e4a868a47a0cfbee90cadc4a99c`。
- push 与 pull request 两次 workflow 各运行 Windows、Linux、macOS 三个 job，共 6/6 成功。
- 每个平台均通过 `go test -count=1 ./...`、`go vet ./...` 与 `go mod tidy -diff`。
- 本地 `master` 已从 `9b41bc7` fast-forward 到 `5e0dedd`，本任务分支从该合并提交创建。

## 下一任务边界

下一任务固定为独立的 T-E2E-001：先冻结真实目标仓库、人工批准点、新 Agent 输入边界、成功/失败验收和证据格式，再执行 `plan → 人工确认完整 plan stdout SHA-256 → apply → 全新 Agent 仅依据目标仓库本地治理实例完成受控任务并提交证据`。

T-E2E-001 必须区分“安装成功”和“核心假设成立”。只读取 `AGENTS.md`、`instance.json` 并复核摘要仍只是安装 E2E；核心假设验证还必须观察新 Agent 是否识别阶段、遵守任务 `allowed_files`、提供命令/退出码/changed_files，并在越界或同一 failure signature 连续三次时停止。T-STATE-004 不创建测试目标仓库、不运行该实验，也不进入阶段 2。

## 实现前检索结果

- 过时当前状态位于 `AGENTS.md` 第 15 行和接入 README 的 22 号文档导航；T-APPLY-001 的“等待后续完成”只存在于合并前更新记录与合并前证据，保留为历史并追加合并后状态。
- 前序状态同步任务依次为 T-STATE-002 与 T-STATE-003，因此本任务使用 T-STATE-004。
- 状态同步仅涉及本任务 `allowed_files` 中的四份 Markdown；没有产品源码改动。

## 验证证据

```text
git fetch origin --prune                                             exit 0
git switch master                                                   exit 0
git merge --ff-only origin/master                                   exit 0
git switch -c codex/t-state-004                                     exit 0
git merge-base --is-ancestor 5e0dedd HEAD                           exit 0
git diff --check                                                    exit 0
frontmatter check（四份 changed_files）                             all true
```

验收对应：

- 本地 `master`、`origin/master` 与任务分支基线均包含 `5e0dedd`。
- AGENTS、接入 README 与 T-APPLY-001 已同步 PR `#1`、6/6 job、合并提交与阶段 1 边界。
- T-E2E-001 只作为下一任务边界被冻结；本任务未创建目标仓库或修改 Go 源码。
- `changed_files` 全部位于 `allowed_files`：

```text
AGENTS.md
docs/正式项目接入/README.md
docs/正式项目接入/22-阶段1最小Apply契约任务单.md
docs/正式项目接入/23-阶段1最小Apply合并状态同步任务单.md
```

本证据是作者执行的机械验证，不替代人工或独立审查。

## 更新记录

- 2026-11-25：建立 T-STATE-004 任务契约，完成状态同步与机械验证；不宣称替代人工或独立审查。
