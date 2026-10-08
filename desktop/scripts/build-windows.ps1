<#
.SYNOPSIS
  Builds the Tunnelkey Windows installers.

.DESCRIPTION
  1. wails build  -> build\bin\Tunnelkey.exe
  2. go build     -> build\bin\tunnelkey-helper.exe
  3. downloads and verifies the official OpenVPN MSI into build\cache\
  4. wix build    -> build\out\Tunnelkey-<ver>-x64.msi
  5. wix build    -> build\out\Tunnelkey-<ver>-setup.exe (OpenVPN + Tunnelkey)

  Requirements: Go, Node/npm, Wails CLI v2.12, WiX Toolset 4.0.6 dotnet tool
  ("dotnet tool install -g wix --version 4.0.6") with the extensions
  WixToolset.UI.wixext, WixToolset.Util.wixext and WixToolset.Bal.wixext 4.0.6
  ("wix extension add -g WixToolset.Bal.wixext/4.0.6", same for UI and Util).

  Code signing is off unless SIGN_CERT_SHA1 (certificate thumbprint in the
  certificate store) or SIGN_PFX (+ SIGN_PFX_PASSWORD) is set. Optional:
  SIGNTOOL (path to signtool.exe), SIGN_TIMESTAMP_URL.

.PARAMETER PlaceholderGui
  Use a stub Tunnelkey.exe when "wails build" fails (installer testing only).

.PARAMETER SkipGui
  Do not run wails build; reuse build\bin\Tunnelkey.exe.

.PARAMETER SkipValidation
  Skip "wix msi validate" (ICE checks).
#>
[CmdletBinding()]
param(
    [switch]$PlaceholderGui,
    [switch]$SkipGui,
    [switch]$SkipValidation
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 3.0

# Official OpenVPN community MSI bundled into setup.exe.
# Source: https://build.openvpn.net/downloads/releases/ (also linked from
# https://openvpn.net/community-downloads/). Authenticode: "OpenVPN Inc.".
$OpenVPNVersion = '2.6.23-I002'
$OpenVPNMsiName = "OpenVPN-$OpenVPNVersion-amd64.msi"
$OpenVPNMsiUrl  = "https://build.openvpn.net/downloads/releases/$OpenVPNMsiName"
$OpenVPNMsiSha256 = '31809e97debe44c12be06ed892c2a43e55b67b31cb852827b7ec8f2d6c0b8dc6'

$WixVersion = '4.0.6'

$Desktop  = Split-Path -Parent $PSScriptRoot
$BuildDir = Join-Path $Desktop 'build'
$BinDir   = Join-Path $BuildDir 'bin'
$CacheDir = Join-Path $BuildDir 'cache'
$OutDir   = Join-Path $BuildDir 'out'
$ObjDir   = Join-Path $OutDir 'obj'
$WinDir   = Join-Path $BuildDir 'windows'
$InstDir  = Join-Path $WinDir 'installer'

function Step([string]$msg) { Write-Host "==> $msg" -ForegroundColor Cyan }

function Invoke-Native {
    param([string]$Exe, [string[]]$Arguments)
    & $Exe @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$Exe failed with exit code $LASTEXITCODE" }
}

function Require([string]$cmd) {
    if (-not (Get-Command $cmd -ErrorAction SilentlyContinue)) { throw "$cmd not found on PATH" }
}

function Get-SignTool {
    if ($env:SIGNTOOL) { return $env:SIGNTOOL }
    $cmd = Get-Command signtool.exe -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    $kits = Join-Path ${env:ProgramFiles(x86)} 'Windows Kits\10\bin'
    if (Test-Path $kits) {
        $found = Get-ChildItem $kits -Recurse -Filter signtool.exe -ErrorAction SilentlyContinue |
            Where-Object { $_.FullName -match '\\x64\\' } | Sort-Object FullName -Descending | Select-Object -First 1
        if ($found) { return $found.FullName }
    }
    throw 'signing requested but signtool.exe was not found (set SIGNTOOL)'
}

$SignEnabled = [bool]($env:SIGN_CERT_SHA1 -or $env:SIGN_PFX)

function Sign-File([string]$path) {
    if (-not $SignEnabled) { return }
    $tool = Get-SignTool
    $ts = if ($env:SIGN_TIMESTAMP_URL) { $env:SIGN_TIMESTAMP_URL } else { 'http://timestamp.digicert.com' }
    $signArgs = @('sign', '/fd', 'SHA256', '/tr', $ts, '/td', 'SHA256', '/d', 'Tunnelkey')
    if ($env:SIGN_CERT_SHA1) {
        $signArgs += @('/sha1', $env:SIGN_CERT_SHA1)
    } else {
        $signArgs += @('/f', $env:SIGN_PFX)
        if ($env:SIGN_PFX_PASSWORD) { $signArgs += @('/p', $env:SIGN_PFX_PASSWORD) }
    }
    $signArgs += $path
    Step "signing $(Split-Path -Leaf $path)"
    Invoke-Native $tool $signArgs
}

# Plain-text license -> minimal RTF for the MSI license dialog.
function Write-LicenseRtf([string]$src, [string]$dst) {
    $sb = New-Object System.Text.StringBuilder
    [void]$sb.Append('{\rtf1\ansi\ansicpg1252\deff0{\fonttbl{\f0\fmodern Consolas;}}\f0\fs16 ')
    foreach ($line in [System.IO.File]::ReadAllLines($src)) {
        $esc = $line.Replace('\', '\\').Replace('{', '\{').Replace('}', '\}')
        foreach ($ch in $esc.ToCharArray()) {
            if ([int]$ch -gt 127) { [void]$sb.Append('\u' + [int]$ch + '?') } else { [void]$sb.Append($ch) }
        }
        [void]$sb.Append("\par`r`n")
    }
    [void]$sb.Append('}')
    [System.IO.File]::WriteAllText($dst, $sb.ToString(), [System.Text.Encoding]::ASCII)
}

# --- preflight ---------------------------------------------------------------
Require go
Require wix
if (-not $SkipGui) { Require wails; Require npm }

$wixVer = (& wix --version) -replace '\+.*$', ''
if ($wixVer -ne $WixVersion) { Write-Warning "wix $wixVer found, scripts are tested with $WixVersion" }

$wails = Get-Content (Join-Path $Desktop 'wails.json') -Raw | ConvertFrom-Json
$Version = $wails.info.productVersion
if ($Version -notmatch '^\d+\.\d+\.\d+$') { throw "wails.json info.productVersion '$Version' must be x.y.z" }
Step "Tunnelkey $Version"

New-Item -ItemType Directory -Force $BinDir, $CacheDir, $OutDir, $ObjDir | Out-Null
$ldflags = "-s -w -X main.version=$Version"

# --- GUI -----------------------------------------------------------------------
$guiExe = Join-Path $BinDir 'Tunnelkey.exe'
$usedPlaceholder = $false
if (-not $SkipGui) {
    Step 'wails build (GUI)'
    Push-Location $Desktop
    try {
        & wails build -clean -platform windows/amd64 -trimpath -ldflags $ldflags
        $wailsExit = $LASTEXITCODE
    } finally { Pop-Location }
    # vite empties frontend/dist; keep the committed placeholder for go:embed.
    $keep = Join-Path $Desktop 'frontend\dist\gitkeep'
    if (-not (Test-Path $keep)) { New-Item -ItemType File -Force $keep | Out-Null }
    if ($wailsExit -ne 0 -or -not (Test-Path $guiExe)) {
        if (-not $PlaceholderGui) { throw "wails build failed (exit $wailsExit); rerun with -PlaceholderGui to test the installers only" }
        Write-Warning 'wails build failed: using a placeholder Tunnelkey.exe (NOT FOR RELEASE)'
        $stub = Join-Path $ObjDir 'placeholder'
        New-Item -ItemType Directory -Force $stub | Out-Null
        Set-Content -Path (Join-Path $stub 'go.mod') -Value "module placeholder`n`ngo 1.21`n" -Encoding ASCII
        Set-Content -Path (Join-Path $stub 'main.go') -Encoding ASCII -Value @'
package main

import "fmt"

func main() { fmt.Println("Tunnelkey placeholder build") }
'@
        New-Item -ItemType Directory -Force $BinDir | Out-Null
        Push-Location $stub
        try {
            $env:GOOS = 'windows'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
            Invoke-Native go @('build', '-o', $guiExe, '.')
        } finally {
            Pop-Location
            Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
        }
        $usedPlaceholder = $true
    }
} elseif (-not (Test-Path $guiExe)) {
    throw "-SkipGui given but $guiExe does not exist"
}

# --- helper --------------------------------------------------------------------
Step 'go build (tunnelkey-helper)'
$helperExe = Join-Path $BinDir 'tunnelkey-helper.exe'
Push-Location $Desktop
try {
    $env:GOOS = 'windows'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
    Invoke-Native go @('build', '-trimpath', '-ldflags', $ldflags, '-o', $helperExe, './cmd/tunnelkey-helper')
} finally {
    Pop-Location
    Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
}

Sign-File $guiExe
Sign-File $helperExe

# --- OpenVPN MSI -----------------------------------------------------------------
$ovpnMsi = Join-Path $CacheDir $OpenVPNMsiName
function Test-Sha([string]$p) {
    (Test-Path $p) -and ((Get-FileHash -Algorithm SHA256 $p).Hash.ToLowerInvariant() -eq $OpenVPNMsiSha256)
}
if (-not (Test-Sha $ovpnMsi)) {
    Step "downloading $OpenVPNMsiUrl"
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    $tmp = "$ovpnMsi.part"
    Invoke-WebRequest -UseBasicParsing -Uri $OpenVPNMsiUrl -OutFile $tmp
    Move-Item -Force $tmp $ovpnMsi
}
if (-not (Test-Sha $ovpnMsi)) { throw "SHA-256 mismatch for $ovpnMsi (expected $OpenVPNMsiSha256)" }
$sig = Get-AuthenticodeSignature $ovpnMsi
if ($sig.Status -ne 'Valid' -or $sig.SignerCertificate.Subject -notmatch 'O=OpenVPN Inc\.') {
    throw "OpenVPN MSI Authenticode check failed: $($sig.Status) $($sig.SignerCertificate.Subject)"
}
Step "OpenVPN MSI verified ($OpenVPNVersion)"

# Burn may not pass ADDLOCAL. Generate a transform that moves the "OpenVPN"
# core feature from level 2 to level 1; with INSTALLLEVEL=1 (set by the
# bundle) only OpenVPN + Drivers (ovpn-dco, Wintun, TAP-Windows6) install.
# The official MSI itself stays untouched (and keeps its signature).
function Invoke-Com($obj, [string]$name, [string]$kind, [object[]]$argv) {
    $flags = [System.Reflection.BindingFlags]::$kind
    if ($null -ne $argv) {
        # COM late binding rejects PSObject-wrapped values (e.g. Join-Path output).
        $argv = @($argv | ForEach-Object { if ($_ -is [psobject]) { $_.psobject.BaseObject } else { $_ } })
    }
    return $obj.GetType().InvokeMember($name, $flags, $null, $obj, $argv)
}
function Get-MsiValue($db, [string]$sql) {
    $view = Invoke-Com $db 'OpenView' 'InvokeMethod' @($sql)
    [void](Invoke-Com $view 'Execute' 'InvokeMethod' $null)
    $rec = Invoke-Com $view 'Fetch' 'InvokeMethod' $null
    $val = if ($rec) { Invoke-Com $rec 'StringData' 'GetProperty' @(1) } else { $null }
    [void](Invoke-Com $view 'Close' 'InvokeMethod' $null)
    return $val
}
$installer = New-Object -ComObject WindowsInstaller.Installer
$refDb = Invoke-Com $installer 'OpenDatabase' 'InvokeMethod' @($ovpnMsi, 0)
$OpenVPNFileVersion = Get-MsiValue $refDb "SELECT Version FROM File WHERE File = 'bin.openvpn.exe'"
if ($OpenVPNFileVersion -notmatch '^\d+\.\d+\.\d+\.\d+$') { throw "unexpected openvpn.exe version '$OpenVPNFileVersion' in $OpenVPNMsiName" }
foreach ($f in 'OpenVPN', 'OpenVPN.GUI', 'Drivers', 'Drivers.OvpnDco', 'Drivers.Wintun', 'Drivers.TAPWindows6') {
    if ($null -eq (Get-MsiValue $refDb "SELECT Feature FROM Feature WHERE Feature = '$f'")) {
        throw "feature $f not found in $OpenVPNMsiName; review the feature selection"
    }
}
$ovpnMst = Join-Path $ObjDir 'tunnelkey-openvpn.mst'
$work = Join-Path $ObjDir 'openvpn-work.msi'
Copy-Item $ovpnMsi $work -Force
Set-ItemProperty $work -Name IsReadOnly -Value $false
$workDb = Invoke-Com $installer 'OpenDatabase' 'InvokeMethod' @($work, 1)   # transact
$v = Invoke-Com $workDb 'OpenView' 'InvokeMethod' @("UPDATE Feature SET Level = 1 WHERE Feature = 'OpenVPN'")
[void](Invoke-Com $v 'Execute' 'InvokeMethod' $null); [void](Invoke-Com $v 'Close' 'InvokeMethod' $null)
[void](Invoke-Com $workDb 'Commit' 'InvokeMethod' $null)
if (Test-Path $ovpnMst) { Remove-Item $ovpnMst -Force }
[void](Invoke-Com $workDb 'GenerateTransform' 'InvokeMethod' @($refDb, $ovpnMst))
# No error suppression, no validation flags: plain table edit.
[void](Invoke-Com $workDb 'CreateTransformSummaryInfo' 'InvokeMethod' @($refDb, $ovpnMst, 0, 0))
$workDb = $null; $refDb = $null; $v = $null
[System.GC]::Collect(); [System.GC]::WaitForPendingFinalizers()
Remove-Item $work -Force -ErrorAction SilentlyContinue
Step "OpenVPN transform written (openvpn.exe $OpenVPNFileVersion)"

# --- MSI -----------------------------------------------------------------------
$licenseTxt = Join-Path $ObjDir 'LICENSE.txt'
$licenseRtf = Join-Path $ObjDir 'license.rtf'
Copy-Item (Join-Path (Split-Path -Parent $Desktop) 'LICENSE') $licenseTxt -Force
Write-LicenseRtf $licenseTxt $licenseRtf

$iconFile = Join-Path $WinDir 'icon.ico'
$msi = Join-Path $OutDir "Tunnelkey-$Version-x64.msi"
Step "wix build -> $(Split-Path -Leaf $msi)"
Invoke-Native wix @(
    'build', (Join-Path $InstDir 'Tunnelkey.wxs'),
    '-arch', 'x64',
    '-ext', "WixToolset.UI.wixext/$WixVersion",
    '-ext', "WixToolset.Util.wixext/$WixVersion",
    '-d', "Version=$Version",
    '-d', "BinDir=$BinDir",
    '-d', "IconFile=$iconFile",
    '-d', "LicenseRtf=$licenseRtf",
    '-d', "LicenseTxt=$licenseTxt",
    '-intermediatefolder', (Join-Path $ObjDir 'msi'),
    '-o', $msi
)
Sign-File $msi

if (-not $SkipValidation) {
    Step 'wix msi validate'
    Invoke-Native wix @('msi', 'validate', '-intermediatefolder', (Join-Path $ObjDir 'validate'), $msi)
}

# --- bundle --------------------------------------------------------------------
$setup = Join-Path $OutDir "Tunnelkey-$Version-setup.exe"
Step "wix build -> $(Split-Path -Leaf $setup)"
Invoke-Native wix @(
    'build', (Join-Path $InstDir 'Bundle.wxs'),
    '-arch', 'x64',
    '-ext', "WixToolset.Bal.wixext/$WixVersion",
    '-ext', "WixToolset.Util.wixext/$WixVersion",
    '-d', "Version=$Version",
    '-d', "TunnelkeyMsi=$msi",
    '-d', "OpenVPNMsi=$ovpnMsi",
    '-d', "OpenVPNMst=$ovpnMst",
    '-d', "OpenVPNFileVersion=$OpenVPNFileVersion",
    '-d', "IconFile=$iconFile",
    '-d', "LogoFile=$(Join-Path $InstDir 'logo.png')",
    '-intermediatefolder', (Join-Path $ObjDir 'bundle'),
    '-o', $setup
)

if ($SignEnabled) {
    # The Burn engine must be signed separately, then the whole bundle.
    $engine = Join-Path $ObjDir 'engine.exe'
    Invoke-Native wix @('burn', 'detach', $setup, '-engine', $engine)
    Sign-File $engine
    Invoke-Native wix @('burn', 'reattach', $setup, '-engine', $engine, '-o', $setup)
    Sign-File $setup
}

# --- summary -------------------------------------------------------------------
$sums = Join-Path $OutDir 'SHA256SUMS.txt'
$lines = foreach ($f in @($msi, $setup)) {
    '{0}  {1}' -f (Get-FileHash -Algorithm SHA256 $f).Hash.ToLowerInvariant(), (Split-Path -Leaf $f)
}
Set-Content -Path $sums -Value $lines -Encoding ASCII
Step 'done'
Get-Item $guiExe, $helperExe, $msi, $setup | ForEach-Object {
    '{0,-34} {1,12} bytes' -f $_.Name, $_.Length
}
if ($usedPlaceholder) { Write-Warning 'Tunnelkey.exe is a PLACEHOLDER: these installers are for testing only' }
if (-not $SignEnabled) { Write-Host 'unsigned build (set SIGN_CERT_SHA1 or SIGN_PFX to sign)' }
