# Freedom 框架构建脚本（Windows / PowerShell）
#
# 产物输出到 dist/ 目录：
#   dist/hello.exe            内嵌模式示例（Go 内嵌后端）
#   dist/multiproc.exe        多后端示例壳（任意语言后端）
#   dist/backends/go_backend.exe      Go 后端（编译）
#   dist/backends/rust_backend.exe    Rust 后端（编译，零依赖）
#   dist/backends/node_backend.mjs    Node 后端（脚本，需 node）
#   dist/backends/py_backend.py       Python 后端（脚本，需 python3）
#
# 用法：
#   .\build.ps1                  # 构建全部（不注版本，Version=dev）
#   .\build.ps1 -Version 1.2.3   # 版本戳注入（-X freedom.Version）+ strip
#   .\build.ps1 -SkipRust        # 跳过 Rust（本机无 rustc 时）
param(
    [switch]$SkipRust,
    [string]$Version = ""
)
$ErrorActionPreference = "Stop"

# 壳层链接参数：GUI 子系统恒在；-Version 时追加符号剥离与版本注入。
# 白名单校验防命令注入（$Version 会进入 go build 参数字符串）。
$ldflags = "-H windowsgui"
if ($Version) {
    if ($Version -notmatch '^[0-9A-Za-z.\-+]+$') { throw "-Version 含非法字符: $Version" }
    $ldflags = "-s -w -X freedom.Version=$Version $ldflags"
}

# $ErrorActionPreference 对原生命令（go/rustc）不生效，必须显式检查 $LASTEXITCODE，
# 否则编译失败会静默继续并以"构建完成"收场。
function Invoke-Native {
    param([string]$Description, [scriptblock]$Command)
    & $Command
    if ($LASTEXITCODE -ne 0) { throw "$Description 失败（退出码 $LASTEXITCODE）" }
}

$root = $PSScriptRoot
$dist = Join-Path $root "dist"
$bk = Join-Path $dist "backends"
New-Item -ItemType Directory -Force -Path $dist, $bk | Out-Null

# 工具链检查
foreach ($t in @("go", "node", "python")) {
    if (-not (Get-Command $t -ErrorAction SilentlyContinue)) {
        Write-Warning "缺少工具链: $t"
    }
}
$hasRust = Get-Command rustc -ErrorAction SilentlyContinue

# 1) 壳层（三平台内核 webview_go，此处产出 Windows 版）
Write-Host "==> go build 壳层 (hello / multiproc)"
$env:CGO_ENABLED = "1"
Push-Location $root
try {
    # GUI 子系统（-H windowsgui）：运行时壳层与后端均不弹出 cmd 黑窗
    Invoke-Native "go build hello" { go build -ldflags $ldflags -o (Join-Path $dist "hello.exe") ./examples/hello }
    Invoke-Native "go build multiproc" { go build -ldflags $ldflags -o (Join-Path $dist "multiproc.exe") ./examples/multiproc }
} finally {
    Pop-Location
}

# 2) Go 后端（编译型）
Write-Host "==> go build Go 后端"
Push-Location $root
try {
    Invoke-Native "go build go_backend" { go build -o (Join-Path $bk "go_backend.exe") ./examples/multiproc/backends }
} finally {
    Pop-Location
}

# 3) 脚本后端（直接复制）
Write-Host "==> 复制脚本后端 (Node / Python)"
Copy-Item (Join-Path $root "examples\multiproc\backends\node_backend.mjs") $bk
Copy-Item (Join-Path $root "examples\multiproc\backends\py_backend.py") $bk
Copy-Item (Join-Path $root "examples\multiproc\backends\rust_backend.rs") $bk

# 4) Rust 后端（零依赖单文件，rustc 直接编译）
if ($hasRust -and -not $SkipRust) {
    Write-Host "==> rustc 编译 Rust 后端"
    Push-Location (Join-Path $root "examples\multiproc\backends")
    try {
        Invoke-Native "rustc rust_backend" { rustc -O -o (Join-Path $bk "rust_backend.exe") rust_backend.rs }
    } finally {
        Pop-Location
    }
} else {
    Write-Warning "跳过 Rust 后端（未安装 rustc 或 -SkipRust）"
}

# 5) 校验清单（sha256sum -c 兼容格式：小写哈希 + 两空格 + dist 内相对路径）
Write-Host "==> 生成 SHA256SUMS.txt"
$sums = Get-ChildItem -Recurse $dist -File |
    Where-Object { $_.Name -ne "SHA256SUMS.txt" } | Sort-Object FullName |
    ForEach-Object {
        $h = (Get-FileHash -Algorithm SHA256 $_.FullName).Hash.ToLower()
        $rel = $_.FullName.Substring($dist.Length + 1).Replace("\", "/")
        "$h  $rel"
    }
# LF 行尾：Set-Content 写 CRLF 会让 GNU sha256sum -c 把 \r 算进文件名而报错
[IO.File]::WriteAllText((Join-Path $dist "SHA256SUMS.txt"), (($sums -join "`n") + "`n"), [Text.Encoding]::ASCII)

Write-Host ""
Write-Host "构建完成 -> $dist"
Get-ChildItem -Recurse $dist -File | Select-Object @{n="文件";e={$_.FullName.Replace($root+"\","")}}, @{n="大小(KB)";e={[math]::Round($_.Length/1KB,1)}}
