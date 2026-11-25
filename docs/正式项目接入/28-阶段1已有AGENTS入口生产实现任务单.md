---
type: reference
purpose: 实现已批准的 0.3-draft 已有 AGENTS 托管引用块、phase snapshot 与安全 UPDATE 回滚闭包
audience: 阶段 1 实现 Agent、项目负责人和后续复核者
status: active
---

# T-ENTRY-002：已有 AGENTS 最小入口生产实现

## 用途

在 `ENTRY-001-R1` 已批准且红灯测试已建立后，实现 `0.3-draft`、显式 `agent_entry_integration`、append-only shared 托管块、tool-owned phase snapshot，以及已有入口 UPDATE 的批准绑定、幂等和进程内失败回滚。实现不得触碰真实目标项目，不修复 evidence/runtime 等其他能力。

## 任务契约

```yaml
task_id: T-ENTRY-002
title: 实现已有 AGENTS 最小可发现入口
phase: 1
acceptance_criteria:
  - manifest/schema/诊断升级到 0.3-draft，旧版本 fail-closed，mode 与 approved_sha256 组合严格校验
  - preserve-or-generate 保持原行为且不生成 phase snapshot
  - append-managed-reference 只接受获批普通 UTF-8 入口，按 LF/CRLF/BOM/末尾换行契约生成精确 postimage
  - plan 输出 UPDATE/shared、preimage/postimage 摘要；phase snapshot 与 instance managed_files 摘要确定
  - apply 在锁内重算，对 CREATE 与 UPDATE 使用独立发布语义；plan 后变化零写入
  - UPDATE 使用同目录 temp/backup、preimage 与 metadata ledger；可观察失败恢复原字节，不能恢复则保留 recovery lock 并 E_APPLY_ROLLBACK
  - 成功后托管块和 phase snapshot 幂等，第二次 apply 为 NO_CHANGES
  - T-ENTRY-001 红灯全部转绿，既有 plan/apply 测试同步 0.3 后无回归
  - Go 1.27.4 test/vet/tidy/build 与 git diff --check 通过
  - 不读取或修改 F:\\oioi 或 e2e-001-a001 隔离副本，不实现 evidence gate/runtime/record-attempt/doctor/review gate
  - changed_files 全部位于 allowed_files
allowed_files:
  - docs/decisions.md
  - docs/正式项目接入/03-治理实例规范v0.md
  - docs/正式项目接入/27-阶段1已有AGENTS最小入口契约任务单.md
  - docs/正式项目接入/28-阶段1已有AGENTS入口生产实现任务单.md
  - docs/正式项目接入/README.md
  - tool/README.md
  - tool/contracts/v0/install-manifest.schema.json
  - tool/contracts/v0/instance.schema.json
  - tool/contracttest/entry_test.go
  - tool/contracttest/plan_test.go
  - tool/contracttest/testdata/plan/*.json
  - tool/internal/plan/plan.go
  - tool/internal/plan/plan_test.go
  - tool/internal/apply/apply.go
  - tool/internal/apply/apply_test.go
  - tool/internal/cli/run_test.go
  - .git/
pre_implementation_search:
  commands:
    - rg -n 'agent_entry|project_fact_preserved|PreparedFile|CREATE|UPDATE|managed_files|phase_snapshot' tool docs/正式项目接入
    - rg -n 'rollback|owned|publish|temp|backup|os.Link|os.Rename|failure' tool/internal/apply
  evidence_required: 列出现有 CREATE-only 数据流、可复用 target safety/plan digest/lock/failpoint，以及 UPDATE 必须新增的 preimage/backup/metadata 闭包
doc_type: reference
doc_purpose: 让生产实现严格对应已批准入口契约，并能由测试复算成功、冲突和回滚行为
```

## 实现前检索结论

- `PreparedFile` 只有 postimage，必须增加 UPDATE preimage；apply 目前遇到已有 final 即 `E_TARGET_CHANGED`。
- CREATE 使用 sibling temp + hard-link no-replace，不能直接用于 UPDATE；UPDATE 需要独立 sibling backup，并在发布前再次核对 preimage。
- transaction ledger 只记录新建 final；必须增加 original bytes/hash/mode/mtime 与 backup 状态，rollback 先恢复 UPDATE，再删除 CREATE。
- target safety、plan stdout SHA-256、exclusive lock、锁内二次 Evaluate、路径碰撞和 recovery lock 可以复用。
- 当前所有 canonical fixture 和 schema hash 固定在 0.2，需要一次性同步调用方，禁止保留两套默认事实。

## 更新记录

- 2026-11-25：从 `5a22400` 红灯基线创建实现任务，冻结验收、allowed_files 与 CREATE/UPDATE 分离边界。
- 2026-11-25：完成 `0.3-draft`、入口模式校验、append-only 托管块、phase snapshot、`UPDATE/shared` 发布及进程内失败回滚；T-ENTRY-001 红灯与既有测试全部通过。

## 实现结果与验收对应

| 验收项 | 结果 | 实现/证据 |
|---|---|---|
| 0.3 严格输入 | PASS | schema 与运行时均只接受 `0.3-draft`；`agent_entry_integration` 必填，mode/hash 组合在 schema 和诊断层双重 fail-closed |
| preserve 行为 | PASS | 缺少入口仍 CREATE；已有入口仍 `NOOP project_fact_preserved`；两者均不产生 phase snapshot |
| append 渲染 | PASS | 覆盖 LF、CRLF、BOM、空文件和末尾换行；拒绝混合换行、孤立 CR、NUL、非法 UTF-8、摘要漂移和托管块冲突 |
| plan 与快照 | PASS | 已有入口输出 `UPDATE/shared` 及 preimage/postimage 摘要；phase snapshot 与 instance `managed_files` 摘要确定 |
| 锁内 UPDATE | PASS | apply 二次 Evaluate 后核对 preimage；CREATE 与 UPDATE 分离，UPDATE 使用 sibling temp/backup 与 rename 发布 |
| 失败回滚 | PASS | `update-backup`、`update-publish`、`update-final-hash`、lock/internal/backup cleanup 故障均恢复原字节、mode、mtime；恢复失败保留 recovery lock 并附加 `E_APPLY_ROLLBACK`；外部修改不删除 |
| 幂等与回归 | PASS | 首次 apply 为 `APPLIED`，第二次为 `NO_CHANGES`；完整 Go test 无回归 |
| 范围 | PASS | 未读取或修改 `F:\oioi`/隔离副本；未实现 evidence/runtime/doctor/review/自动三次止损 |
| 平台边界 | LOCAL PASS | 本次只在 Windows 本地验证；未运行三平台 CI，不声称跨平台完成 |

## 可复核证据

工作分支：`codex/t-entry-002-implementation`。工具链为仓库外官方 Go `1.27.4`；下载归档 SHA-256 为 `178ab98950ac69ab25069307dc6ade79f3816e414cce95b4da166a115cde1e7e`。

| 命令 | 退出码 | 结果 |
|---|---:|---|
| `go test -count=1 ./...`（`tool/`） | 0 | contracttest、apply、cli、plan 全部通过 |
| `go vet ./...`（`tool/`） | 0 | 无输出 |
| `go mod tidy -diff`（`tool/`） | 0 | 无 diff |
| `go build -o %TEMP%/aigov-t-entry-002.exe ./cmd/aigov`（`tool/`） | 0 | 构建成功；本地产物 SHA-256 `1a710adcf1a1a4c8cf296cb3b8177dd3be622a9d1f6be3862e06cf5ac55ab34b` |
| `git diff --check` | 0 | 无 whitespace error |
| changed Markdown frontmatter 检查 | 0 | 全部包含 type/purpose/audience/status |

### changed_files

- `docs/decisions.md`
- `docs/正式项目接入/03-治理实例规范v0.md`
- `docs/正式项目接入/28-阶段1已有AGENTS入口生产实现任务单.md`
- `docs/正式项目接入/README.md`
- `tool/README.md`
- `tool/contracts/v0/install-manifest.schema.json`
- `tool/contracts/v0/instance.schema.json`
- `tool/contracttest/plan_test.go`
- `tool/contracttest/testdata/plan/missing-phase-basis.json`
- `tool/contracttest/testdata/plan/unknown-and-unsafe.json`
- `tool/contracttest/testdata/plan/valid-phase0.json`
- `tool/internal/apply/apply.go`
- `tool/internal/apply/apply_test.go`
- `tool/internal/plan/plan.go`
- `tool/internal/plan/plan_test.go`

上述文件全部位于本任务 `allowed_files`；T-ENTRY-001 的 `tool/contracttest/entry_test.go` 红灯基线无需修改即已转绿。
