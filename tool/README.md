---
type: how-to
purpose: 说明正式 Go CLI 如何构建、测试并运行可关闭的实验性只读 plan
audience: 构建、验证或继续开发 aigov CLI 的 Agent 与维护者
status: active
---

# aigov Go CLI

## 用途

用本目录构建和验证 Go canonical source，并运行阶段 1 的实验性只读 `plan`。`plan` 严格校验安装 manifest，输出可审核的单行 JSON 文件计划或稳定 JSONL 诊断，不修改目标项目。

## 不适用

当前切片只提供实验性、显式批准的最小 `apply`；不提供 crash auto-recovery、rollback 命令、`doctor`、`upgrade`、状态机、JCS、Ed25519 或可信发布能力。不要把最小安装路径解释为完整治理运行时。

## 怎么用

要求 Go 1.27.4。仓库根目录的 `out/` 已被忽略，可安全用于本地构建产物。

```powershell
cd tool
go test ./...
go vet ./...
New-Item -ItemType Directory -Force ..\out | Out-Null
go build -o ../out/aigov.exe ./cmd/aigov
../out/aigov.exe help
../out/aigov.exe version
../out/aigov.exe unsupported
```

除 `unsupported` 外，上述验证命令应返回 0；`unsupported` 应把诊断写入 stderr 并返回 2。

启用并运行有效 manifest：

当前安装 manifest 必须使用精确的 `contract_version: 0.3-draft`，并由项目负责人或受控编排器提供符合契约的显式 `instance_id`；CLI 不会生成或猜测实例身份。`layout.agent_entry_integration` 也是必填对象：新项目或仅保留已有入口时使用 `preserve-or-generate` + `approved_sha256: null`；只有明确批准已有 `AGENTS.md` 原始字节时才使用 `append-managed-reference` + 对应 SHA-256。

```powershell
$env:AIGOV_ENABLE_EXPERIMENTAL_PLAN = '1'
../out/aigov.exe plan --manifest ./contracttest/testdata/plan/valid-phase0.json
if ($LASTEXITCODE -ne 0) { throw "valid plan exit code was $LASTEXITCODE" }
```

预期 stdout 是单行 JSON envelope，包含基础安装目标和条件资源；空目标仓库中的 action 为 `CREATE`/`SKIP`：

```json
{"contract_version":"0.3-draft","instance_id":"inst-example-01","operations":[...]}
```

每项 operation 固定给出 `action`、`logical_id`、项目相对 `path`、`ownership`、expected/actual SHA-256 和 `reason`。`preserve-or-generate` 下已有 project-owned `AGENTS.md` 仍为 `NOOP project_fact_preserved`，且不会创建 phase snapshot。`append-managed-reference` 下获批入口输出 `UPDATE/shared`，原字节保持为精确前缀，同时生成 `.ai-governance/phase.json`；摘要漂移或托管块冲突时 fail-closed。tool-owned 文件不一致时输出 `CONFLICT` 并返回 2。

执行已批准计划时，manifest 的 `integration.mode` 必须为 `apply`。对 plan stdout 原始字节（包含末尾 LF）计算 SHA-256，再显式传入：

```powershell
$env:AIGOV_ENABLE_EXPERIMENTAL_APPLY = '1'
../out/aigov.exe apply --manifest <apply-manifest.json> --plan-sha256 <64位小写SHA-256>
if ($LASTEXITCODE -ne 0) { throw "apply failed with $LASTEXITCODE" }
```

首次成功返回 `status: APPLIED` 和实际创建路径；`created` 只列 CREATE，不把 UPDATE 冒充新文件。批准新的全 NOOP plan 后再次执行返回 `status: NO_CHANGES`。UPDATE 在 lock 内复算并再次核对 preimage，使用同目录 temp/backup；可观察失败恢复原字节、mode 与 mtime，无法安全恢复时保留 recovery lock 并返回 `E_APPLY_ROLLBACK`。摘要不匹配、conflict、已有 lock 或 target 变化均停止。目标仓库必须由单一负责人控制并在 apply 期间保持静默；本切片不对抗拥有仓库写权限的恶意并发进程，也不提供强杀/掉电后的自动恢复命令。

运行失败输入：

```powershell
../out/aigov.exe plan --manifest ./contracttest/testdata/plan/missing-phase-basis.json
if ($LASTEXITCODE -ne 2) { throw "invalid plan exit code was $LASTEXITCODE" }
```

诊断写入 stderr，格式为一行一个 `code`、`path`、`message`；上述输入固定返回 `E_REQUIRED /governance/phase_basis`。

一键关闭与回滚验证：

```powershell
Remove-Item Env:AIGOV_ENABLE_EXPERIMENTAL_PLAN -ErrorAction SilentlyContinue
../out/aigov.exe plan --manifest ./contracttest/testdata/plan/valid-phase0.json
if ($LASTEXITCODE -ne 2) { throw "disabled plan exit code was $LASTEXITCODE" }
```

关闭功能开关不会修改目标项目，也不需要删除任何治理文件。

## 源码边界

- `experiments/delivery-runtime/go/` 只证明 Go 运行时、哈希和签名能力，不复制其中的 JCS/Ed25519 探针。
- `experiments/cloud-runtime-matrix/go/` 只提供三平台交付证据，不复制其实验入口。
- `experiments/installer-lifecycle-matrix/go/` 实现了实验性 `plan/apply/upgrade/rollback/doctor` 和文件写入，明确超出本任务范围，不作为正式源码复制。

三个实验仅作为交付与契约证据；正式代码从本目录的最小命令面独立建立。

## 常见错误 / 注意事项

- 不要把只读 `plan` 通过解读为安装器或治理状态机已经完成。
- 不要把 `out/aigov.exe` 提交到 Git。
- 未安装全局 Go 时，使用仓库外或已忽略目录中的官方便携工具链。
- 不要在自动化环境中默认设置 `AIGOV_ENABLE_EXPERIMENTAL_PLAN`；当前功能必须显式 opt-in。

## 更新记录

- 2026-11-24：建立 Go 1.27 最小 CLI 骨架、自动测试和独立构建入口。
- 2026-11-25：增加测试先行的实验性只读 `plan`、严格 manifest 校验与关闭示例。
- 2026-11-25：把有效输出扩展为确定性安装文件计划，加入 instance/schema 字节摘要、项目事实保护与 target safety 检查；仍不提供 apply。
- 2026-11-25：增加绑定完整 plan 摘要的最小 apply、exclusive lock、sibling temp、hard-link no-replace、幂等与进程内失败回滚。
- 2026-11-25：实现安装契约 `0.3-draft`；增加已有 `AGENTS.md` 的获批 append-only shared UPDATE、仅 append 模式生成的 phase snapshot、同目录 backup 与安全回滚。
