# Exercise the actual PowerShell installer with synthetic API responses and ZIPs.
$ErrorActionPreference = 'Stop'
$installer = Join-Path $PSScriptRoot 'install.ps1'
$root = Join-Path ([IO.Path]::GetTempPath()) ('eos-channel-tests-' + [guid]::NewGuid())
New-Item -ItemType Directory $root | Out-Null
$script:fixtures = @{}
$script:pageOne = @()
$script:pageTwo = @()
$script:latest = $null
$script:networkFailure = $false
function Release([string]$tag, [string]$kind = 'beta') {
    return [pscustomobject]@{ tag_name=$tag; prerelease=($kind -ne 'stable'); draft=($kind -eq 'draft') }
}
function Invoke-RestMethod {
    param($Uri, $Headers, $TimeoutSec)
    if ($script:networkFailure) { throw 'fixture network error' }
    if ($Uri -match '/tags/(.+)$') { return $script:fixtures[$Matches[1]] }
    if ($Uri -match '/latest$') { return $script:latest }
    if ($Uri -match 'page=2$') { return ,$script:pageTwo }
    return ,$script:pageOne
}
function Assert-Equal($actual, $expected) {
    if ($actual -ne $expected) { throw "expected <$expected>, received <$actual>" }
}
function Assert-Failure([scriptblock]$Action) {
    $failed = $false
    try { & $Action | Out-Null } catch { $failed = $true }
    if (-not $failed) { throw 'expected failure' }
}
try {
    $script:latest = Release 'v1.0.0-beta.34' 'stable'
    $script:pageOne = @((Release 'v1.0.0-beta.9'),(Release 'v1.0.0-beta.100' 'draft'),(Release 'v9.0.0' 'stable'),(Release 'v1.0.0-beta.34' 'stable'),(Release 'v1.0.0-beta.36'),(Release 'v1.0.0-beta.35'),(Release 'v1.0.0-beta.40' 'stable'))
    $script:fixtures['v1.0.0-beta.34'] = $script:latest
    $script:fixtures['v1.0.0-beta.36'] = Release 'v1.0.0-beta.36'
    $dest = Join-Path $root 'install'
    Assert-Equal (& $installer -ResolveOnly -InstallDir $dest) 'v1.0.0-beta.34 stable'
    Assert-Equal (& $installer -ResolveOnly -Channel beta -InstallDir $dest) 'v1.0.0-beta.36 beta'
    Assert-Equal (& $installer -ResolveOnly -Version v1.0.0-beta.34 -InstallDir $dest) 'v1.0.0-beta.34 stable'
    Assert-Equal (& $installer -ResolveOnly -Version v1.0.0-beta.36 -InstallDir $dest) 'v1.0.0-beta.36 beta'
    Assert-Failure { & $installer -ResolveOnly -Version v1.0.0-beta.36 -Channel stable -InstallDir $dest }
    $script:latest = Release 'v1.0.0' 'stable'
    Assert-Equal (& $installer -ResolveOnly -InstallDir $dest) 'v1.0.0 stable'
    $script:latest = Release 'v1.0.0-beta.36'
    Assert-Failure { & $installer -ResolveOnly -InstallDir $dest }
    $script:pageOne = @(1..100 | ForEach-Object { Release 'v1.0.0-beta.9' })
    $script:pageTwo = @((Release 'v1.0.0-beta.10'))
    Assert-Equal (& $installer -ResolveOnly -Channel beta -InstallDir $dest) 'v1.0.0-beta.10 beta'
    $script:pageOne = @()
    Assert-Failure { & $installer -ResolveOnly -Channel beta -InstallDir $dest }
    $script:pageOne = @([pscustomobject]@{ message='rate limit' })
    Assert-Failure { & $installer -ResolveOnly -Channel beta -InstallDir $dest }
    $script:networkFailure = $true
    Assert-Failure { & $installer -Channel beta -InstallDir $dest }
    if (Test-Path $dest) { throw 'network failure changed installation' }
    $script:networkFailure = $false
    Assert-Failure { & $installer -Channel beta -InstallDir (Join-Path $root 'scoop/apps/eos/current') }

    # A real test executable lets the installer verify its installed version.
    $source = Join-Path $root 'main.go'
    [IO.File]::WriteAllText($source, 'package main; import "fmt"; func main(){ fmt.Println("eos v1.0.0-beta.36 (fixture)") }')
    $stage = Join-Path $root 'stage'
    New-Item -ItemType Directory -Force (Join-Path $stage 'core/fixture') | Out-Null
    & go build -o (Join-Path $stage 'eos.exe') $source
    if ($LASTEXITCODE -ne 0) { throw 'fixture binary build failed' }
    [IO.File]::WriteAllText((Join-Path $stage 'core/fixture/eos-core.exe'), 'fixture core')
    $asset = 'eos-cli_v1.0.0-beta.36_windows-amd64.zip'
    $zip = Join-Path $root $asset
    Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $zip
    $script:zipFixture = $zip
    $script:sumFixture = Join-Path $root 'SHA256SUMS.txt'
    [IO.File]::WriteAllText($script:sumFixture, (Get-FileHash $zip -Algorithm SHA256).Hash.ToLower() + "  $asset`n")
    function Invoke-WebRequest {
        param($Uri, $OutFile, [switch]$UseBasicParsing)
        if ($Uri.EndsWith('/SHA256SUMS.txt')) { Copy-Item $script:sumFixture $OutFile }
        else { Copy-Item $script:zipFixture $OutFile }
    }
    if ($env:PROCESSOR_ARCHITECTURE -ne 'AMD64') { throw 'full ZIP fixture requires the Windows amd64 CI runner' }
    & $installer -Version v1.0.0-beta.36 -InstallDir $dest -NoPathUpdate
    $marker = Get-Content (Join-Path $dest 'eos-install.json') -Raw | ConvertFrom-Json
    Assert-Equal $marker.channel beta
    Assert-Equal $marker.method script
    [IO.File]::WriteAllText((Join-Path $dest 'eos-install.json'), '{"channel":"beta","method":"package-manager"}')
    Assert-Failure { & $installer -Version v1.0.0-beta.36 -InstallDir $dest -NoPathUpdate }
    [IO.File]::WriteAllText((Join-Path $dest 'eos-install.json'), '{"channel":"beta","method":"script"}')

    $script:pageOne = @((Release 'v1.0.0-beta.36'))
    Assert-Equal (& $installer -ResolveOnly -InstallDir $dest) 'v1.0.0-beta.36 beta'
    $before = (Get-FileHash (Join-Path $dest 'eos.exe')).Hash
    [IO.File]::WriteAllText($script:sumFixture, ('0' * 64) + "  $asset`n")
    Assert-Failure { & $installer -Version v1.0.0-beta.36 -InstallDir $dest -NoPathUpdate }
    Assert-Equal (Get-FileHash (Join-Path $dest 'eos.exe')).Hash $before
    Write-Host 'PASS PowerShell channel selection, legacy stable, version pins, pagination, failures, isolation, package manager boundary and verified install'
} finally {
    Remove-Item $root -Recurse -Force -ErrorAction SilentlyContinue
}
