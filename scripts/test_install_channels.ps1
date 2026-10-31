# Exercise the actual PowerShell installer with synthetic API responses and ZIPs.
$ErrorActionPreference = 'Stop'
$installer = Join-Path $PSScriptRoot 'install.ps1'
$root = Join-Path ([IO.Path]::GetTempPath()) ('eos-channel-tests-' + [guid]::NewGuid())
New-Item -ItemType Directory $root | Out-Null
$global:EosInstallTestFixtures = @{}
$global:EosInstallTestPageOne = @()
$global:EosInstallTestPageTwo = @()
$global:EosInstallTestLatest = $null
$global:EosInstallTestNetworkFailure = $false
function Release([string]$tag, [string]$kind = 'beta') {
    return [pscustomobject]@{ tag_name=$tag; prerelease=($kind -ne 'stable'); draft=($kind -eq 'draft') }
}
function Invoke-RestMethod {
    param($Uri, $Headers, $TimeoutSec)
    if ($global:EosInstallTestNetworkFailure) { throw 'fixture network error' }
    if ($Uri -match '/tags/(.+)$') { return $global:EosInstallTestFixtures[$Matches[1]] }
    if ($Uri -match '/latest$') { return $global:EosInstallTestLatest }
    if ($Uri -match 'page=2$') { return ,$global:EosInstallTestPageTwo }
    return ,$global:EosInstallTestPageOne
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
    $global:EosInstallTestLatest = Release 'v1.0.0-beta.34' 'stable'
    $global:EosInstallTestPageOne = @((Release 'v1.0.0-beta.9'),(Release 'v1.0.0-beta.100' 'draft'),(Release 'v9.0.0' 'stable'),(Release 'v1.0.0-beta.34' 'stable'),(Release 'v1.0.0-beta.36'),(Release 'v1.0.0-beta.35'),(Release 'v1.0.0-beta.40' 'stable'))
    $global:EosInstallTestFixtures['v1.0.0-beta.34'] = $global:EosInstallTestLatest
    $global:EosInstallTestFixtures['v1.0.0-beta.36'] = Release 'v1.0.0-beta.36'
    $dest = Join-Path $root 'install'
    Assert-Equal (& $installer -ResolveOnly -InstallDir $dest) 'v1.0.0-beta.34 stable'
    Assert-Equal (& $installer -ResolveOnly -Channel beta -InstallDir $dest) 'v1.0.0-beta.36 beta'
    Assert-Equal (& $installer -ResolveOnly -Version v1.0.0-beta.34 -InstallDir $dest) 'v1.0.0-beta.34 stable'
    Assert-Equal (& $installer -ResolveOnly -Version v1.0.0-beta.36 -InstallDir $dest) 'v1.0.0-beta.36 beta'
    Assert-Failure { & $installer -ResolveOnly -Version v1.0.0-beta.36 -Channel stable -InstallDir $dest }
    $global:EosInstallTestLatest = Release 'v1.0.0' 'stable'
    Assert-Equal (& $installer -ResolveOnly -InstallDir $dest) 'v1.0.0 stable'
    $global:EosInstallTestLatest = Release 'v1.0.0-beta.36'
    Assert-Failure { & $installer -ResolveOnly -InstallDir $dest }
    $global:EosInstallTestPageOne = @(1..100 | ForEach-Object { Release 'v1.0.0-beta.9' })
    $global:EosInstallTestPageTwo = @((Release 'v1.0.0-beta.10'))
    Assert-Equal (& $installer -ResolveOnly -Channel beta -InstallDir $dest) 'v1.0.0-beta.10 beta'
    $global:EosInstallTestPageOne = @()
    Assert-Failure { & $installer -ResolveOnly -Channel beta -InstallDir $dest }
    $global:EosInstallTestPageOne = @([pscustomobject]@{ message='rate limit' })
    Assert-Failure { & $installer -ResolveOnly -Channel beta -InstallDir $dest }
    $global:EosInstallTestNetworkFailure = $true
    Assert-Failure { & $installer -Channel beta -InstallDir $dest }
    if (Test-Path $dest) { throw 'network failure changed installation' }
    $global:EosInstallTestNetworkFailure = $false
    Assert-Failure { & $installer -Channel beta -InstallDir (Join-Path $root 'scoop/apps/eos/current') }

    if ($PSVersionTable.Platform -eq 'Unix') {
        Write-Host 'PASS PowerShell resolver cases; native ZIP installation is covered by the Windows CI runner'
        return
    }

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
    $global:EosInstallTestZipFixture = $zip
    $global:EosInstallTestSumFixture = Join-Path $root 'SHA256SUMS.txt'
    [IO.File]::WriteAllText($global:EosInstallTestSumFixture, (Get-FileHash $zip -Algorithm SHA256).Hash.ToLower() + "  $asset`n")
    function Invoke-WebRequest {
        param($Uri, $OutFile, [switch]$UseBasicParsing)
        if ($Uri.EndsWith('/SHA256SUMS.txt')) { Copy-Item $global:EosInstallTestSumFixture $OutFile }
        else { Copy-Item $global:EosInstallTestZipFixture $OutFile }
    }
    if ($env:PROCESSOR_ARCHITECTURE -ne 'AMD64') { throw 'full ZIP fixture requires the Windows amd64 CI runner' }
    & $installer -Version v1.0.0-beta.36 -InstallDir $dest -NoPathUpdate
    $marker = Get-Content (Join-Path $dest 'eos-install.json') -Raw | ConvertFrom-Json
    Assert-Equal $marker.channel beta
    Assert-Equal $marker.method script
    [IO.File]::WriteAllText((Join-Path $dest 'eos-install.json'), '{"channel":"beta","method":"package-manager"}')
    Assert-Failure { & $installer -Version v1.0.0-beta.36 -InstallDir $dest -NoPathUpdate }
    [IO.File]::WriteAllText((Join-Path $dest 'eos-install.json'), '{"channel":"beta","method":"script"}')

    $global:EosInstallTestPageOne = @((Release 'v1.0.0-beta.36'))
    Assert-Equal (& $installer -ResolveOnly -InstallDir $dest) 'v1.0.0-beta.36 beta'
    $before = (Get-FileHash (Join-Path $dest 'eos.exe')).Hash
    [IO.File]::WriteAllText($global:EosInstallTestSumFixture, ('0' * 64) + "  $asset`n")
    Assert-Failure { & $installer -Version v1.0.0-beta.36 -InstallDir $dest -NoPathUpdate }
    Assert-Equal (Get-FileHash (Join-Path $dest 'eos.exe')).Hash $before
    Write-Host 'PASS PowerShell channel selection, legacy stable, version pins, pagination, failures, isolation, package manager boundary and verified install'
} finally {
    Remove-Variable -Scope Global -Name 'EosInstallTest*' -ErrorAction SilentlyContinue
    Remove-Item $root -Recurse -Force -ErrorAction SilentlyContinue
}
