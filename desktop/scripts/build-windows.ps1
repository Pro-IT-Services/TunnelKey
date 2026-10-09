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

  Code signing is off unless one of these is set:
    ARTIFACT_SIGNING_METADATA  path to metadata.json for Azure Artifact Signing
                               (optional ARTIFACT_SIGNING_DLIB); sign in with
                               `az login` first. The JSON names Endpoint,
                               CodeSigningAccountName and CertificateProfileName.
    SIGN_CERT_SHA1             certificate thumbprint in the certificate store
    SIGN_PFX (+ SIGN_PFX_PASSWORD) Optional:
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

$SignEnabled = [bool]($env:ARTIFACT_SIGNING_METADATA -or $env:SIGN_CERT_SHA1 -or $env:SIGN_PFX)

# Azure Artifact Signing (formerly Trusted Signing): signtool with Microsoft's
# dlib and a metadata.json naming the endpoint, account and certificate
# profile. Sign in first with `az login` (or a service principal / OIDC in CI).
function Get-ArtifactSigningDlib {
    if ($env:ARTIFACT_SIGNING_DLIB) { return $env:ARTIFACT_SIGNING_DLIB }
    # Where the winget package installs it (no x64 subfolder).
    $default = Join-Path $env:LOCALAPPDATA 'Microsoft\MicrosoftArtifactSigningClientTools\Azure.CodeSigning.Dlib.dll'
    if (Test-Path $default) { return $default }
    $roots = @($env:LOCALAPPDATA, $env:ProgramFiles, ${env:ProgramFiles(x86)}) | Where-Object { $_ }
    foreach ($root in $roots) {
        $found = Get-ChildItem $root -Recurse -Filter 'Azure.CodeSigning.Dlib.dll' -ErrorAction SilentlyContinue |
            Sort-Object { $_.FullName -notmatch '\\x64\\' } | Select-Object -First 1
        if ($found) { return $found.FullName }
    }
    throw 'ARTIFACT_SIGNING_METADATA is set but Azure.CodeSigning.Dlib.dll was not found (install with: winget install -e --id Microsoft.Azure.ArtifactSigningClientTools, or set ARTIFACT_SIGNING_DLIB)'
}

function Sign-File([string]$path) {
    if (-not $SignEnabled) { return }
    $tool = Get-SignTool
    if ($env:ARTIFACT_SIGNING_METADATA) {
        $ts = if ($env:SIGN_TIMESTAMP_URL) { $env:SIGN_TIMESTAMP_URL } else { 'http://timestamp.acs.microsoft.com' }
        Step "signing $(Split-Path -Leaf $path) (Artifact Signing)"
        Invoke-Native $tool @('sign', '/v', '/fd', 'SHA256', '/tr', $ts, '/td', 'SHA256', '/d', 'Tunnelkey',
            '/dlib', (Get-ArtifactSigningDlib), '/dmdf', $env:ARTIFACT_SIGNING_METADATA, $path)
        return
    }
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

# --- Group Policy package --------------------------------------------------------
# GPO software installation deploys MSIs + transforms only (no setup.exe, no
# command-line properties), so everything setup.exe passes is baked into
# transforms here:
#   openvpn-gpo.mst   - core + drivers only (feature levels and INSTALLLEVEL=1)
#   tunnelkey-gpo.mst - SKIPOPENVPNCHECK=1, so the two packages may install in
#                       either order (the app finds OpenVPN when connecting)
function New-Transform([string]$sourceMsi, [string]$mst, [string[]]$sql) {
    $work = Join-Path $ObjDir ('transform-work-' + [IO.Path]::GetFileName($sourceMsi))
    Copy-Item $sourceMsi $work -Force
    Set-ItemProperty $work -Name IsReadOnly -Value $false
    $ref = Invoke-Com $installer 'OpenDatabase' 'InvokeMethod' @($sourceMsi, 0)
    $db = Invoke-Com $installer 'OpenDatabase' 'InvokeMethod' @($work, 1)
    foreach ($q in $sql) {
        $view = Invoke-Com $db 'OpenView' 'InvokeMethod' @($q)
        [void](Invoke-Com $view 'Execute' 'InvokeMethod' $null)
        [void](Invoke-Com $view 'Close' 'InvokeMethod' $null)
    }
    [void](Invoke-Com $db 'Commit' 'InvokeMethod' $null)
    if (Test-Path $mst) { Remove-Item $mst -Force }
    [void](Invoke-Com $db 'GenerateTransform' 'InvokeMethod' @($ref, $mst))
    [void](Invoke-Com $db 'CreateTransformSummaryInfo' 'InvokeMethod' @($ref, $mst, 0, 0))
    $db = $null; $ref = $null; $view = $null
    [System.GC]::Collect(); [System.GC]::WaitForPendingFinalizers()
    Remove-Item $work -Force -ErrorAction SilentlyContinue
}
function Set-PropertySql($db, [string]$name, [string]$value) {
    if ($null -eq (Get-MsiValue $db "SELECT Value FROM Property WHERE Property = '$name'")) {
        return "INSERT INTO Property (Property, Value) VALUES ('$name', '$value')"
    }
    return "UPDATE Property SET Value = '$value' WHERE Property = '$name'"
}

$GpoDir = Join-Path $OutDir 'gpo'
if (Test-Path $GpoDir) { Remove-Item $GpoDir -Recurse -Force }
New-Item -ItemType Directory -Path $GpoDir | Out-Null
$gpoOpenVPNMsi = Join-Path $GpoDir $OpenVPNMsiName
$gpoTunnelkeyMsi = Join-Path $GpoDir (Split-Path -Leaf $msi)
Copy-Item $ovpnMsi $gpoOpenVPNMsi -Force
Copy-Item $msi $gpoTunnelkeyMsi -Force

$ovpnRef = Invoke-Com $installer 'OpenDatabase' 'InvokeMethod' @($ovpnMsi, 0)
$ovpnLevelSql = Set-PropertySql $ovpnRef 'INSTALLLEVEL' '1'
$ovpnRef = $null
New-Transform $ovpnMsi (Join-Path $GpoDir 'openvpn-gpo.mst') @(
    "UPDATE Feature SET Level = 1 WHERE Feature = 'OpenVPN'",
    $ovpnLevelSql
)
$tkRef = Invoke-Com $installer 'OpenDatabase' 'InvokeMethod' @($msi, 0)
$tkSkipSql = Set-PropertySql $tkRef 'SKIPOPENVPNCHECK' '1'
$tkRef = $null
New-Transform $msi (Join-Path $GpoDir 'tunnelkey-gpo.mst') @($tkSkipSql)

$gpoReadme = @"
Tunnelkey $Version - Group Policy deployment
=============================================

Copy this folder to a share that domain computers can read (UNC path, e.g.
\\fileserver\deploy\Tunnelkey\). In Group Policy Management, edit a GPO linked
to the computers' OU:

Computer Configuration > Policies > Software Settings > Software installation
  > New > Package...

1. $OpenVPNMsiName
   Deployment method: Advanced. Modifications tab > Add > openvpn-gpo.mst
   (installs only the OpenVPN core and its network drivers, without the
   OpenVPN GUI).

2. $(Split-Path -Leaf $gpoTunnelkeyMsi)
   Deployment method: Advanced. Modifications tab > Add > tunnelkey-gpo.mst
   (lets it install before OpenVPN; the app finds OpenVPN when connecting).

Both packages are per-machine and install at the next restart. Transforms can
only be added when a package is created, not later.

Updates: add the new Tunnelkey MSI as a package and, on its Upgrades tab,
mark it as upgrading the previous Tunnelkey package. OpenVPN only needs a new
package when Tunnelkey ships a newer OpenVPN.

Silent install without Group Policy (e.g. Intune, SCCM, scripts):
  Tunnelkey-$Version-setup.exe /quiet /norestart
or the two MSIs:
  msiexec /i $OpenVPNMsiName TRANSFORMS=openvpn-gpo.mst /qn /norestart
  msiexec /i $(Split-Path -Leaf $gpoTunnelkeyMsi) /qn /norestart
"@
Set-Content -Path (Join-Path $GpoDir 'README-GPO.txt') -Value $gpoReadme -Encoding UTF8
Step "Group Policy package -> $GpoDir"

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
if (-not $SignEnabled) { Write-Host 'unsigned build (set ARTIFACT_SIGNING_METADATA, SIGN_CERT_SHA1 or SIGN_PFX to sign)' }
