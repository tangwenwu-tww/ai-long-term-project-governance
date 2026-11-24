---
type: how-to
purpose: 让独立 Reviewer 从 canonical source 重建 T-DELIVERY-003 的 Node SEA 产物并复核自包含、哈希、大小和格式边界
audience: 交付选型 Executor / 独立 Reviewer
status: active
---

# Delivery runtime experiment

## 用途与边界

该目录是 `T-DELIVERY-003` 的一次性阶段 0 实验输入，不是正式安装器源码。

- `go/main.go` 计划验证原生可执行文件、SHA-256 与 Ed25519 标准库能力；本轮因 Go 工具链获取停止而**未构建、未验证**。
- `node/main.js` 验证 Node SEA、SHA-256 与 Ed25519 内置能力。
- 两者直接使用规范中的既定 canonical bytes；本实验**没有实现 RFC 8785 JCS**。
- `downloads/`、`toolchains/`、`out/` 被 `.gitignore` 排除，不是 canonical source。
- Linux/macOS SEA 只从 Windows 交叉生成并检查格式，未在目标系统执行，不能声称可交付。

以下命令以 Windows PowerShell 7、当前目录为本 README 所在的 `experiments/delivery-runtime/` 为前提。所有下载均来自 Node 官方分发并在使用前校验 SHA-256。

## 1. 获取并校验固定 Node 工具链

```powershell
$experimentRoot = (Get-Location).Path
$downloads = Join-Path $experimentRoot 'downloads'
$toolchains = Join-Path $experimentRoot 'toolchains'
$out = Join-Path $experimentRoot 'out'
New-Item -ItemType Directory -Force -Path $downloads,$toolchains,$out | Out-Null

$nodeVersion = 'v26.13.1'
$base = "https://nodejs.org/dist/$nodeVersion"
$archives = @(
  @{ Name='node-v26.13.1-win-x64.zip'; Sha='e2edda75bfb089654705cf65a13b58933bfd5f57148e8ebda916f58a2474f1f7' },
  @{ Name='node-v26.13.1-linux-x64.tar.gz'; Sha='518674de2b621728a5e4486f5fb3b7d2900bb3cc4aa97969edaa463db99f9778' },
  @{ Name='node-v26.13.1-darwin-arm64.tar.gz'; Sha='aec2514ea1ceb2ba52455e9ec65986f6749a06dd759e5db2a72e6889668c9735' }
)

foreach ($archive in $archives) {
  $path = Join-Path $downloads $archive.Name
  Invoke-WebRequest -Uri "$base/$($archive.Name)" -OutFile $path -UseBasicParsing
  $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash.ToLowerInvariant()
  if ($actual -ne $archive.Sha) { throw "Checksum mismatch: $($archive.Name)" }
  Write-Output "$($archive.Name)=$actual"
}
```

预期三行哈希分别等于 `archives` 中的固定值；任一不符立即停止，不能解包或执行。

## 2. 只提取实验所需的三个 Node 可执行文件

```powershell
Expand-Archive -LiteralPath (Join-Path $downloads 'node-v26.13.1-win-x64.zip') -DestinationPath $toolchains -Force

$temporaryExtract = Join-Path ([IO.Path]::GetTempPath()) ([IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $temporaryExtract | Out-Null

tar -xzf (Join-Path $downloads 'node-v26.13.1-linux-x64.tar.gz') `
  -C $temporaryExtract 'node-v26.13.1-linux-x64/bin/node'
if ($LASTEXITCODE -ne 0) { throw 'Linux Node extraction failed' }

tar -xzf (Join-Path $downloads 'node-v26.13.1-darwin-arm64.tar.gz') `
  -C $temporaryExtract 'node-v26.13.1-darwin-arm64/bin/node'
if ($LASTEXITCODE -ne 0) { throw 'Darwin Node extraction failed' }

foreach ($platform in @('linux-x64','darwin-arm64')) {
  $destination = Join-Path $toolchains "node-v26.13.1-$platform/bin"
  New-Item -ItemType Directory -Force -Path $destination | Out-Null
  Copy-Item -LiteralPath (Join-Path $temporaryExtract "node-v26.13.1-$platform/bin/node") `
    -Destination (Join-Path $destination 'node') -Force
}

$builder = Join-Path $toolchains 'node-v26.13.1-win-x64/node.exe'
& $builder --version
```

预期版本为 `v26.13.1`。只提取 `bin/node`，避免实验依赖 npm/npx 符号链接或全量安装。

## 3. 构建三个 SEA

```powershell
Push-Location (Join-Path $experimentRoot 'node')
try {
  & $builder --build-sea '.\sea-config.json'
  if ($LASTEXITCODE -ne 0) { throw 'Windows SEA build failed' }
  & $builder --build-sea '.\sea-config-linux-x64.json'
  if ($LASTEXITCODE -ne 0) { throw 'Linux SEA build failed' }
  & $builder --build-sea '.\sea-config-darwin-arm64.json'
  if ($LASTEXITCODE -ne 0) { throw 'Darwin SEA build failed' }
} finally {
  Pop-Location
}
```

三个子构建的退出码都必须为 `0`。配置已关闭 snapshot/code cache，并禁止通过 `NODE_OPTIONS` 扩展运行参数。

## 4. Windows 空 PATH 运行

```powershell
$windowsSea = Join-Path $out 'governance-node-sea.exe'
$savedPath = $env:PATH
try {
  $env:PATH = ''
  & $windowsSea
  $runExit = $LASTEXITCODE
} finally {
  $env:PATH = $savedPath
}
if ($runExit -ne 0) { throw "Windows SEA failed: $runExit" }
```

预期 stdout：

```json
{"runtime":"v26.13.1","platform":"win32","arch":"x64","event_sha256":"81588443ac9948f2eda8f0269294b006d10be5d8f4e7dcf4bb23ef17a88d624d","hash_matches":true,"owner_signature_ok":true}
```

## 5. 核对产物哈希、大小和格式

```powershell
$names = @(
  'governance-node-sea.exe',
  'governance-node-sea-linux-x64',
  'governance-node-sea-darwin-arm64'
)
$rows = foreach ($name in $names) {
  $path = Join-Path $out $name
  $bytes = [IO.File]::ReadAllBytes($path)
  [pscustomobject]@{
    Name = $name
    Length = (Get-Item -LiteralPath $path).Length
    Magic = (($bytes[0..3] | ForEach-Object { $_.ToString('X2') }) -join '')
    SHA256 = (Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash.ToLowerInvariant()
  }
}
$rows | Format-Table -AutoSize
Get-AuthenticodeSignature -LiteralPath (Join-Path $out 'governance-node-sea.exe') |
  Select-Object Status,StatusMessage
```

预期：

| 目标 | 字节数 | Magic | SHA-256 |
|---|---:|---|---|
| Windows x64 | 100,233,032 | PE（前两字节 `4D5A`） | `952c61ec37878536d3a96c5a65433cd31769b8b8bdac523e1d18c4f0e4cd42ff` |
| Linux x64 | 150,699,136 | `7F454C46` | `04c21b14e7678228670a3bc438a17e733a58b0400d9d73fea0df65ed372caaf0` |
| macOS arm64 | 145,415,332 | `CFFAEDFE` | `78b870f89367319256479cbf42c722ace89c2b834cd6e33549b53b957a04b5e7` |

Windows Authenticode 当前预期为 `NotSigned`。Linux/macOS 的 magic 和大小只证明生成格式，不证明目标系统运行。不同 Node patch、路径、配置或签名步骤可能改变哈希，不得把上表当成永久发布摘要。

## 6. 重复构建检查

在不改变工具链、源码和配置的前提下，记录三个 SHA-256，再重复步骤 3 和 5。当前实验预期同一 Windows 主机上的前后哈希逐项相同；这不等同于跨 CI 主机 reproducible build。

Go 实验必须在取得可校验的官方工具链后另行继续。此前不得把 `go/main.go` 的存在当成任何 Go 能力证据。
