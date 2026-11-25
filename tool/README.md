---
type: how-to
purpose: 说明正式 Go CLI 如何构建、测试并运行 plan、apply、机器可复核 evidence 与自动止损命令
audience: 构建、验证或继续开发 aigov CLI 的 Agent 与维护者
status: active
---

# aigov Go CLI

## 用途

用本目录构建和验证 Go canonical source，并运行阶段 1 的实验性 `plan`、`apply`、`evidence begin/collect` 与安装后本地 `record-attempt`/`halt-status`。

## 不适用

当前切片只提供实验性、显式开启的最小 `apply`、hash-only evidence 与阶段 1 同签名三次止损；不提供 crash auto-recovery、rollback、`doctor`、`upgrade`、unhalt、阶段 4 总失败上限、完整状态机或可信发布能力。

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

当前安装 manifest 必须使用精确的 `contract_version: 0.4-draft`，并由项目负责人或受控编排器提供符合契约的显式 `instance_id`；CLI 不会生成或猜测实例身份。apply 会把当前平台正在运行的原生 `aigov` 复制为 `runtime_entrypoint` 并记录摘要。

```powershell
$env:AIGOV_ENABLE_EXPERIMENTAL_PLAN = '1'
../out/aigov.exe plan --manifest ./contracttest/testdata/plan/valid-phase0.json
if ($LASTEXITCODE -ne 0) { throw "valid plan exit code was $LASTEXITCODE" }
```

预期 stdout 是单行 JSON envelope，包含基础安装目标和条件资源；空目标仓库中的 action 为 `CREATE`/`SKIP`：

```json
{"contract_version":"0.4-draft","instance_id":"inst-example-01","operations":[...]}
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

### 生成机器可复核 evidence

先在任务修改前准备 `0.1-draft` request，其中冻结 task/attempt、`allowed_files`、验收项和不经 shell 解析的 executable + argv。目标项目必须已有 `.ai-governance/instance.json`，且其 `resources.evidence_store` 为安全项目相对路径。

```json
{"schema_version":"0.1-draft","task_id":"T-EXAMPLE-001","task_revision":1,"attempt_id":"a001","task_sha256":"1111111111111111111111111111111111111111111111111111111111111111","allowed_files":["src/","tests/*.json"],"acceptance_criteria":[{"id":"AC-01","text":"core verification passes","command_ids":["verify"]}],"commands":[{"id":"verify","executable":"go","args":["test","./..."],"cwd":"tool","expected_exit_code":0}]}
```

```powershell
$env:AIGOV_ENABLE_EXPERIMENTAL_EVIDENCE = '1'
$projectRoot = 'C:\path\to\target'
$requestPath = Join-Path $projectRoot '.ai-governance\requests\T-EXAMPLE-001.json'
$receiptPath = Join-Path $env:TEMP 'aigov-evidence-begin.json'
$errorPath = Join-Path $env:TEMP 'aigov-evidence-begin.stderr'
$process = Start-Process -FilePath '..\out\aigov.exe' -ArgumentList @('evidence','begin','--project',$projectRoot,'--request',$requestPath) -Wait -PassThru -NoNewWindow -RedirectStandardOutput $receiptPath -RedirectStandardError $errorPath
if ($process.ExitCode -ne 0) { throw "evidence begin failed with $($process.ExitCode)" }
$beginSHA256 = (Get-FileHash -Algorithm SHA256 -LiteralPath $receiptPath).Hash.ToLowerInvariant()

# 完成任务修改后，传入完整 begin stdout 的摘要。
..\out\aigov.exe evidence collect --project $projectRoot --request $requestPath --begin-sha256 $beginSHA256
```

`begin` 创建不可复用的 attempt request/baseline；`collect` 先重算该绑定，再运行全部命令。成功验收返回 0；命令退出码不符、验收项未通过或越界时仍封存 `FAIL` manifest，并返回 1；契约、篡改或安全错误返回 2。manifest 只保留 stdout/stderr 的 raw-byte SHA-256、字节数、UTF-8 与行数摘要，不保留原始输出。

### 记录 attempt 并查询止损状态

安装后的 `.ai-governance/runtime/aigov.exe`（Unix 为 `aigov`）可在目标项目离线运行：

```powershell
./.ai-governance/runtime/aigov.exe record-attempt --project . --task-id T-EXAMPLE-001 --attempt-id a001 --exit-code 1 --signature-sha256 <64位小写SHA-256>
./.ai-governance/runtime/aigov.exe halt-status --project . --task-id T-EXAMPLE-001
```

成功 attempt 不传 signature，并清零连续失败计数；失败必须传 signature。同一 signature 连续第三次失败会先封存记录，再返回 3 和 `HALTED_FOR_HUMAN`；第四次返回 3/`E_TASK_HALTED` 且不写文件。attempt ID 从 `a001` 严格连续，ledger 是权威源，state cache 可删除并从 ledger 重建；不存在 Agent 可调用的解除停止命令。

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
- 不要修改 begin 生成的 request/baseline，或对已封存 attempt 重跑 collect；请使用新 attempt ID。
- evidence 直接启动 executable + argv，不支持 shell pipeline、重定向或自定义环境变量。

## 更新记录

- 2026-11-24：建立 Go 1.27 最小 CLI 骨架、自动测试和独立构建入口。
- 2026-11-25：增加测试先行的实验性只读 `plan`、严格 manifest 校验与关闭示例。
- 2026-11-25：把有效输出扩展为确定性安装文件计划，加入 instance/schema 字节摘要、项目事实保护与 target safety 检查；仍不提供 apply。
- 2026-11-25：增加绑定完整 plan 摘要的最小 apply、exclusive lock、sibling temp、hard-link no-replace、幂等与进程内失败回滚。
- 2026-11-25：实现安装契约 `0.3-draft`；增加已有 `AGENTS.md` 的获批 append-only shared UPDATE、仅 append 模式生成的 phase snapshot、同目录 backup 与安全回滚。
- 2026-11-25：实现 evidence `0.1-draft` begin/collect；增加无 Git 内容基线、直接 argv 命令摘要、内容变更清单、验收映射与不可覆盖 manifest。
- 2026-11-25：实现安装契约 `0.4-draft`；安装平台 runtime，并增加不可变 attempt ledger、只读 halt-status、第三次停止与第四次零写入拒绝。
