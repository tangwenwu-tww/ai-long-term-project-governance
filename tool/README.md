---
type: how-to
purpose: 说明正式 Go CLI 骨架如何构建、测试和运行，以及当前明确未实现的能力
audience: 构建、验证或继续开发 aigov CLI 的 Agent 与维护者
status: active
---

# aigov Go CLI 骨架

## 用途

在正式治理功能开发前，用本目录验证 Go canonical source 能被构建、测试并作为原生 CLI 运行。当前命令面刻意只包含 `help`、`version` 和未知命令报错。

## 不适用

本骨架不是治理实例安装器，也不提供 `plan`、`apply`、`doctor`、`upgrade`、状态机、JCS、Ed25519、文件写入或可信发布能力。需要这些能力时，应等待对应任务和 contract tests 放行。

## 怎么用

要求 Go 1.27。仓库根目录的 `out/` 已被忽略，可安全用于本地构建产物。

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

前六条命令应返回 0；`unsupported` 应把诊断写入 stderr 并返回 2。

## 源码边界

- `experiments/delivery-runtime/go/` 只证明 Go 运行时、哈希和签名能力，不复制其中的 JCS/Ed25519 探针。
- `experiments/cloud-runtime-matrix/go/` 只提供三平台交付证据，不复制其实验入口。
- `experiments/installer-lifecycle-matrix/go/` 实现了实验性 `plan/apply/upgrade/rollback/doctor` 和文件写入，明确超出本任务范围，不作为正式源码复制。

三个实验仅作为交付与契约证据；正式代码从本目录的最小命令面独立建立。

## 常见错误 / 注意事项

- 不要把 CLI 空壳通过解读为安装器或治理状态机已经完成。
- 不要把 `out/aigov.exe` 提交到 Git。
- 未安装全局 Go 时，使用仓库外或已忽略目录中的官方便携工具链。

## 更新记录

- 2026-11-24：建立 Go 1.27 最小 CLI 骨架、自动测试和独立构建入口。
