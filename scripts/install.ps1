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
    [ValidateSet("", "stable", "beta")][string]$Channel = "",
    [switch]$ResolveOnly,
    [switch]$NoPathUpdate,
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

function Get-EosChannel([string]$Tag) {
    if ($Tag -notmatch '^v?[0-9]+\.[0-9]+\.[0-9]+(-beta\.[0-9]+)?$') { throw "不支持的版本: $Tag" }
    if ($Tag.TrimStart('v') -eq '1.0.0-beta.34' -or $Tag -notmatch '-beta\.') { return 'stable' }
    return 'beta'
}
function Get-PublishedChannel($Release) {
    if ($Release.draft -isnot [bool] -or $Release.prerelease -isnot [bool] -or $Release.draft) { return '' }
    try { $actual = Get-EosChannel $Release.tag_name } catch { return '' }
    if ($Release.prerelease -ne ($actual -eq 'beta')) { return '' }
    return $actual
}
function Get-ReleaseMetadata([string]$Url) {
    return Invoke-RestMethod -Uri $Url -Headers @{ 'User-Agent' = 'EOS-Install'; 'Accept' = 'application/vnd.github+json' } -TimeoutSec 30
}

try {
    if ($InstallDir.Replace('\', '/').ToLower() -match '/(cellar|homebrew/caskroom|scoop/apps|chocolatey|nix/store|snap)/') {
        throw '此目录由包管理器维护，请使用原包管理器更新'
    }
    $markerPath = Join-Path $InstallDir 'eos-install.json'
    if (Test-Path $markerPath) {
        $installed = Get-Content $markerPath -Raw | ConvertFrom-Json
        if ($installed.channel -notin @('stable', 'beta') -or $installed.method -notin @('script', 'archive', 'package-manager')) { throw '安装渠道记录无效' }
        if ($installed.method -eq 'package-manager') { throw '此安装由包管理器维护，请使用原包管理器更新' }
        if (-not $Channel -and -not $Version) { $Channel = $installed.channel }
    }
    $installedExe = Join-Path $InstallDir 'eos.exe'
    if (-not $Channel -and -not $Version -and (Test-Path $installedExe)) {
        $installedVersion = (& $installedExe version | Select-Object -First 1) -split '\s+'
        if ($LASTEXITCODE -ne 0 -or $installedVersion.Count -lt 2 -or $installedVersion[0] -ne 'eos') { throw '无法识别现有安装，请指定 -Channel 或 -Version' }
        $Channel = Get-EosChannel $installedVersion[1]
    }
    $api = "https://api.github.com/repos/$Repo/releases"
    if ($Version) {
        $Version = 'v' + $Version.TrimStart('v')
        $release = Get-ReleaseMetadata "$api/tags/$Version"
        $actual = Get-PublishedChannel $release
        if (-not $actual -or $release.tag_name -ne $Version -or ($Channel -and $Channel -ne $actual)) { throw '指定版本不存在、不是公开发布或与渠道不一致' }
        $Channel = $actual
    } else {
        if (-not $Channel) { $Channel = 'stable' }
        if ($Channel -eq 'stable') {
            $release = Get-ReleaseMetadata "$api/latest"
            if ((Get-PublishedChannel $release) -ne 'stable') { throw 'latest 不是公开正式发布，拒绝跨渠道安装' }
            $Version = $release.tag_name
        } else {
            $best = $null
            for ($page = 1; $page -le 100; $page++) {
                $releases = Get-ReleaseMetadata "${api}?per_page=100&page=$page"
                if ($releases.Count -eq 1 -and $releases[0] -and -not $releases[0].tag_name) { throw 'Beta 发布信息不是有效的发布列表' }
                foreach ($release in $releases) {
                    if ((Get-PublishedChannel $release) -ne 'beta') { continue }
                    $base = [version](($release.tag_name.TrimStart('v') -split '-beta\.')[0])
                    $number = [uint64](($release.tag_name -split '-beta\.')[1])
                    if (-not $best -or $base -gt $best.Base -or ($base -eq $best.Base -and $number -gt $best.Number)) {
                        $best = @{ Tag = $release.tag_name; Base = $base; Number = $number }
                    }
                }
                if ($releases.Count -lt 100) { break }
                if ($page -eq 100) { throw '发布分页超出上限' }
            }
            if (-not $best) { throw '没有已发布的 Beta 版本' }
            $Version = $best.Tag
        }
    }
    if ($ResolveOnly) { Write-Output "$Version $Channel"; return }
    Write-Host "目标版本: $Version (windows/$Arch), 渠道: $Channel"

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
    $marker = @{ channel = $Channel; method = "script" } | ConvertTo-Json -Compress
    [IO.File]::WriteAllText((Join-Path $InstallDir "eos-install.json"), $marker + "`n", [Text.UTF8Encoding]::new($false))
    Get-ChildItem -Path (Split-Path -Parent $InstallDir) -Filter "eos.old-*" -Directory -ErrorAction SilentlyContinue |
        Remove-Item -Recurse -Force -ErrorAction SilentlyContinue

    # 用户 PATH 追加安装目录
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if (-not $NoPathUpdate -and $userPath -notlike "*$InstallDir*") {
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
