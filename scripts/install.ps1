# EOS CLI 官方安装脚本（Windows PowerShell）
#
# 用法：
#   irm https://cdn.jsdelivr.net/gh/eosaios/eos@main/scripts/install.ps1 | iex
#   .\install.ps1 -Version v1.0.0-beta.3
#
# 行为：GitHub Releases 拉取 windows 归档 → SHA256 校验 → 安装到
#   %LOCALAPPDATA%\Programs\eos（eos.exe + core\）→ 用户 PATH 追加该目录。
# `eos update` 之后原地自升级。
# 入口走 jsdelivr CDN：raw.githubusercontent.com 在国内网络不可达。

param(
    [string]$Version = "",
    [string]$InstallDir = ""
)

$ErrorActionPreference = "Stop"
# Windows PowerShell 5.1 下 IWR 渲染进度条会把大文件下载拖慢 10 倍以上
$ProgressPreference = "SilentlyContinue"
$Repo = "eosaios/eos"

if (-not $InstallDir) {
    $InstallDir = Join-Path $env:LOCALAPPDATA "Programs\eos"
}

$Arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "ARM64" { "arm64" }
    default { "amd64" }
}

try {
    if (-not $Version) {
        Write-Host "正在获取最新版本..."
        # 不走 api.github.com：未认证 API 限流 60 次/小时/IP，国内共享出口
        # 极易触发「rate limit exceeded」。releases/latest 网页会 302 到
        # releases/tag/<版本>，从 Location 解析 tag，不占 API 配额。
        # HttpWebRequest 方式兼容 Windows PowerShell 5.1 与 PowerShell 7。
        try { [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12 } catch {}
        $req = [System.Net.HttpWebRequest]::Create("https://github.com/$Repo/releases/latest")
        $req.AllowAutoRedirect = $false
        $req.UserAgent = "EOS-Install"
        $resp = $req.GetResponse()
        $location = $resp.Headers["Location"]
        $resp.Close()
        if (-not $location) { throw "无法获取最新版本号（releases/latest 未返回重定向）" }
        $Version = ($location -split '/tag/')[-1]
        if (-not $Version -or $Version -eq $location) { throw "无法从重定向解析版本号: $location" }
    }
    Write-Host "目标版本: $Version (windows/$Arch)"

    $verNum = $Version.TrimStart("v")
    $asset = "eos-cli_v${verNum}_windows-${Arch}.zip"
    $baseUrl = "https://github.com/$Repo/releases/download/$Version"

    $tmp = New-Item -ItemType Directory -Force -Path (Join-Path $env:TEMP ([System.Guid]::NewGuid().ToString()))
    $assetPath = Join-Path $tmp $asset

    Write-Host "下载 $asset ..."
    Invoke-WebRequest -Uri "$baseUrl/$asset" -OutFile $assetPath -UseBasicParsing
    if (-not (Test-Path $assetPath)) { throw "下载失败（$Version 可能没有 windows-$Arch 归档）" }

    Write-Host "校验 SHA256 ..."
    $sumsPath = Join-Path $tmp "SHA256SUMS.txt"
    Invoke-WebRequest -Uri "$baseUrl/SHA256SUMS.txt" -OutFile $sumsPath -UseBasicParsing
    $want = $null
    foreach ($line in (Get-Content $sumsPath)) {
        $fields = $line -split '\s+'
        if ($fields.Count -ge 2 -and $fields[1].Trim() -eq $asset) { $want = $fields[0].Trim().ToLower(); break }
    }
    if (-not $want) { throw "SHA256SUMS.txt 中没有 $asset 条目" }
    $got = (Get-FileHash -Path $assetPath -Algorithm SHA256).Hash.ToLower()
    if ($got -ne $want) { throw "校验不匹配：期望 $want，实际 $got" }

    Write-Host "安装到 $InstallDir ..."
    $extractDir = Join-Path $tmp "extract"
    Expand-Archive -Path $assetPath -DestinationPath $extractDir -Force
    $eosBin = Get-ChildItem -Path $extractDir -Recurse -Filter "eos.exe" | Select-Object -First 1
    if (-not $eosBin) { throw "归档中未找到 eos.exe" }
    $stageRoot = Split-Path -Parent $eosBin.FullName

    # 旧版本移走后再落新文件（运行中的 eos-core.exe 无法即时删除时残留目录下次清理）
    if (Test-Path $InstallDir) {
        $stale = "$InstallDir.old-$(Get-Date -Format 'yyyyMMddHHmmss')"
        try { Move-Item -Path $InstallDir -Destination $stale -ErrorAction Stop } catch {
            throw "无法移开旧版本（eos 可能正在运行），请关闭后重试: $_"
        }
    }
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    Copy-Item -Path (Join-Path $stageRoot "*") -Destination $InstallDir -Recurse -Force
    Get-ChildItem -Path (Split-Path -Parent $InstallDir) -Filter "eos.old-*" -Directory -ErrorAction SilentlyContinue |
        Remove-Item -Recurse -Force -ErrorAction SilentlyContinue

    # 用户 PATH 追加安装目录
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -notlike "*$InstallDir*") {
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$InstallDir", "User")
        Write-Host "已将 $InstallDir 加入用户 PATH（新开的终端生效）"
    }

    & (Join-Path $InstallDir "eos.exe") version
    Write-Host ""
    Write-Host "安装完成。运行 eos 开始使用；eos update 可自升级到最新版。"
}
finally {
    if ($tmp -and (Test-Path $tmp)) { Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue }
}
