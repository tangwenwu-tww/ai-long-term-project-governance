---
type: adr
purpose: 定义并记录“全新 Agent 只依赖目标仓库本地治理实例”的最小验证方案与结果
audience: 主 Agent / PoC 执行者 / 独立 Reviewer
status: active
---

# 最小接入 PoC

## 背景

本仓库目前已有方法论、阶段手册和模板，但尚未证明全新 AI Agent 能仅依赖目标项目本地文件正确行动。本 PoC 用一个隔离的模拟目标项目验证最小闭环，不代表正式工具选型。

## 成功标准（实现前固定）

1. 本地治理配置能明确返回阶段及该阶段启用的门禁。
2. 缺少验收标准、允许修改范围或实现前检索时，任务校验失败。
3. 证据中的失败退出码、越界文件或未验证验收项会阻断通过。
4. 阶段 2 起必须提供独立 Reviewer；Reviewer 与作者的 Agent ID 或会话 ID 相同都会失败。
5. 同一 failure signature 连续失败 3 次后任务进入 `HALTED_FOR_HUMAN`，后续审计被阻断；成功一次清零。
6. 阶段 3 起要求 stdout SHA-256；阶段 4 增加所有失败尝试合计 10 次的硬上限。
7. PoC 运行时只读取 `poc/minimal-target/` 内的项目事实和治理文件，不读取本方法论仓库的其他文档。

## 允许修改范围

- `docs/正式项目接入/01-最小接入PoC.md`
- `poc/minimal-target/`
- `poc/tests/`

## 实现前检索

```powershell
rg -n "failure signature|allowed_files|stdout_sha256|Reviewer|阶段" AGENTS.md docs
```

结论：仓库已有规则和 JSON 示例，但没有可运行校验器或隔离目标项目，因此新增 PoC，不复写另一套方法论。

## 方案

- 用目标项目本地 `.ai-governance/governance.json` 保存阶段与止损阈值。
- 用目标项目本地 `.ai-governance/gate.py` 执行检查，避免运行时依赖本方法论仓库。
- 任务、证据、审查记录均使用 JSON，先验证协议闭环。
- Python 标准库只是当前环境可运行的 PoC 载体；通过后再比较安装器与正式运行时技术方案。

## 推翻条件

- 目标 Agent 环境无法稳定提供 Python；
- JSON 任务协议不能覆盖实际项目的必要语义；
- 本地复制校验器导致版本漂移且无法通过安装器或版本检查解决。

## 验证命令

```powershell
python -m unittest discover -s poc/tests -v
python poc/minimal-target/.ai-governance/gate.py inspect --project poc/minimal-target
python poc/minimal-target/.ai-governance/gate.py audit --project poc/minimal-target --task tasks/T-001.json --evidence evidence/T-001.json --review reviews/T-001.json
```

## 结果

自动门禁部分已执行：

| 命令 | 退出码 | 结果 |
|---|---:|---|
| `python -m unittest discover -s poc/tests -v` | 0 | 17 项测试通过，覆盖隔离副本运行、阶段识别与晋升、任务契约、越界修改与路径穿越、哈希重算、Reviewer 隔离、可信批准和阶段 3/4 止损 |
| `python poc/minimal-target/.ai-governance/gate.py inspect --project poc/minimal-target` | 0 | 返回阶段 3 及 8 个已启用门禁 |
| `python poc/minimal-target/.ai-governance/gate.py audit --project poc/minimal-target --task tasks/T-001.json --evidence evidence/T-001.json --review reviews/T-001.json` | 0 | 示例任务审计通过 |

`inspect` 的规范化 stdout SHA-256 为 `7017314af0fb99289ccf7d3054cb711c86db82499540c81a43b1266d6ff7edd9`。门禁会读取目标项目内的 stdout 产物重新计算哈希，并要求 Reviewer 独立产物与 Executor 证据一致；只填写一个格式正确的任意哈希不能通过。

成功标准第 7 项已取得实际行为证据：`fresh-executor` 在不继承主对话历史、且只允许读取 `poc/minimal-target/` 的条件下，读取本地 `AGENTS.md` 和任务契约，生成 `artifacts/fresh-agent-result.json` 与独立 evidence；随后 `independent-reviewer` 在另一个无制作历史的上下文中重跑核心命令。两者 stdout 的规范化 SHA-256 均为 `06cb9794d798965d6d9c5a4c5f4e60cd750f96fad9299c52cb9866566d41e7eb`，完整 `T-AGENT-001` audit 退出码为 0。

机器侧已进一步证明：把 `poc/minimal-target/` 复制到临时隔离目录后，只调用副本中的 `.ai-governance/gate.py`，阶段识别和完整审计仍能通过；`../`、目录中间的 `..` 和盘符绝对路径都不能绕过 `allowed_files`；阶段 4 使用不同 failure signature 累积到 10 次也会停止；已经进入 `HALTED_FOR_HUMAN` 的任务不能继续审计。

动态升级采用“成熟度信号评估 + 可信人工批准 + 逐级晋升”：`assess-phase` 根据可观测信号推荐阶段；`promote` 一次只能提升一级，且批准文件必须绑定当前 `from_phase → to_phase`、通过 `trust.json` 的 token 哈希验证。独立 Reviewer 第一轮因任意 `--approved-by` 可伪造而 REJECT；修复信任锚并补齐任务/evidence 后，第二轮 PASS，`T-GOVERNANCE-002` 完整 audit 退出码为 0。

## 结论

PoC 的七项成功标准全部具备机器输出与独立 Agent 行为证据，状态转为 `active`。该结论证明本地治理协议闭环可行；Python 标准库仍只是 PoC 技术载体，不能据此跳过正式接入工具的选型。

## 更新记录

- 2026-11-24：固定 PoC 范围、成功标准和技术假设。
- 2026-11-24：完成隔离 Executor、独立 Reviewer、自动止损和可信阶段晋升验证；两轮审查由 REJECT 修复至 PASS，PoC 转为 active。
