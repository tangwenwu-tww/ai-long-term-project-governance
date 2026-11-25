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

当前切片不提供 `apply`、`doctor`、`upgrade`、`rollback`、状态机、JCS、Ed25519、文件写入或可信发布能力。不要把只读 `plan` 解释为治理实例已经安装。

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

当前安装 manifest 必须使用精确的 `contract_version: 0.2-draft`，并由项目负责人或受控编排器提供符合契约的显式 `instance_id`；CLI 不会生成或猜测实例身份。

```powershell
$env:AIGOV_ENABLE_EXPERIMENTAL_PLAN = '1'
../out/aigov.exe plan --manifest ./contracttest/testdata/plan/valid-phase0.json
if ($LASTEXITCODE -ne 0) { throw "valid plan exit code was $LASTEXITCODE" }
```

预期 stdout 是单行 JSON envelope，包含四个最小安装目标和条件资源；空目标仓库中的 action 为 `CREATE`/`SKIP`：

```json
{"contract_version":"0.2-draft","instance_id":"inst-example-01","operations":[...]}
```

每项 operation 固定给出 `action`、`logical_id`、项目相对 `path`、`ownership`、expected/actual SHA-256 和 `reason`。已有 project-owned `AGENTS.md` 只会 `NOOP project_fact_preserved`；tool-owned 文件不一致时输出 `CONFLICT` 并返回 2。

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
