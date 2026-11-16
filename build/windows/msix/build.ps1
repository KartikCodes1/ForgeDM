<#
.SYNOPSIS
  Packages ForgeDM.exe as an MSIX for the Microsoft Store.

.DESCRIPTION
  Needs the Windows 10/11 SDK (for makeappx, makepri and signtool).
  Run from the repository root after `wails build`:

    powershell -File build\windows\msix\build.ps1          # Store package
    powershell -File build\windows\msix\build.ps1 -Test    # signed test package

  The Store package uses the identity in store.json and is left unsigned:
  the Store signs it after certification. Upload it in Partner Center.

  -Test builds with a test identity and signs it with a self-signed
  certificate, so you can install it on your own PC. Before installing,
  trust the certificate once (as administrator):
    Import-Certificate -FilePath build\bin\ForgeDM-test.cer -CertStoreLocation Cert:\LocalMachine\TrustedPeople
#>
param(
  [string]$Exe = "build\bin\ForgeDM.exe",
  [string]$OutDir = "build\bin",
  [switch]$Test
)

$ErrorActionPreference = "Stop"
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$root = (Resolve-Path (Join-Path $here "..\..\..")).Path

function Find-SdkTool([string]$name) {
  $kits = Join-Path ${env:ProgramFiles(x86)} "Windows Kits\10\bin"
  $tool = Get-ChildItem $kits -Directory -ErrorAction SilentlyContinue |
    Where-Object { $_.Name -match '^\d+\.\d+\.\d+\.\d+$' -and (Test-Path (Join-Path $_.FullName "x64\$name")) } |
    Sort-Object { [version]$_.Name } -Descending |
    Select-Object -First 1
  if (-not $tool) { throw "$name not found. Install the Windows 10/11 SDK." }
  return (Join-Path $tool.FullName "x64\$name")
}

function Invoke-Tool([string]$tool, [string[]]$arguments) {
  & $tool @arguments
  if ($LASTEXITCODE -ne 0) { throw "$(Split-Path -Leaf $tool) failed with exit code $LASTEXITCODE" }
}

function Escape-Xml([string]$s) { return [Security.SecurityElement]::Escape($s) }

if (-not (Test-Path $Exe)) { throw "$Exe not found. Run 'wails build' first." }
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

# x.y.z from wails.json becomes x.y.z.0 (the Store needs the last part to be 0).
$wails = Get-Content (Join-Path $root "wails.json") -Raw | ConvertFrom-Json
$version = "$($wails.info.productVersion).0"

$store = Get-Content (Join-Path $here "store.json") -Raw | ConvertFrom-Json
if ($Test) {
  $identity = @{ Name = "ForgeDM.Test"; Publisher = "CN=ForgeDM Test"; Display = "ForgeDM (test build)" }
  $outName = "ForgeDM-$version-test.msix"
} elseif ($store.identityName -and $store.publisher -and $store.publisherDisplayName) {
  $identity = @{ Name = $store.identityName; Publisher = $store.publisher; Display = $store.publisherDisplayName }
  $outName = "ForgeDM-$version.msix"
} else {
  throw "build\windows\msix\store.json has no Store identity yet. Fill it in from Partner Center, or use -Test."
}

$makeappx = Find-SdkTool "makeappx.exe"
$makepri = Find-SdkTool "makepri.exe"

# Stage the package contents.
$stage = Join-Path $OutDir ("msix-" + [IO.Path]::GetFileNameWithoutExtension($outName))
if (Test-Path $stage) { Remove-Item $stage -Recurse -Force }
New-Item -ItemType Directory -Path $stage | Out-Null
Copy-Item $Exe (Join-Path $stage "ForgeDM.exe")
Copy-Item (Join-Path $here "Assets") (Join-Path $stage "Assets") -Recurse

$manifest = Get-Content (Join-Path $here "AppxManifest.xml") -Raw
$manifest = $manifest.Replace("{{IDENTITY_NAME}}", (Escape-Xml $identity.Name))
$manifest = $manifest.Replace("{{PUBLISHER_DISPLAY_NAME}}", (Escape-Xml $identity.Display))
$manifest = $manifest.Replace("{{PUBLISHER}}", (Escape-Xml $identity.Publisher))
$manifest = $manifest.Replace("{{VERSION}}", $version)
[IO.File]::WriteAllText((Join-Path $stage "AppxManifest.xml"), $manifest, (New-Object Text.UTF8Encoding $false))

# Index the scaled logos so Windows picks the right size for each place.
$priConfig = Join-Path $OutDir "priconfig.xml"
Invoke-Tool $makepri @("createconfig", "/cf", $priConfig, "/dq", "en-US", "/pv", "10.0.0", "/o")
Invoke-Tool $makepri @("new", "/pr", $stage, "/cf", $priConfig, "/mn", (Join-Path $stage "AppxManifest.xml"), "/of", (Join-Path $stage "resources.pri"), "/o")
Remove-Item $priConfig

$out = Join-Path $OutDir $outName
Invoke-Tool $makeappx @("pack", "/d", $stage, "/p", $out, "/o")
Remove-Item $stage -Recurse -Force

if ($Test) {
  $signtool = Find-SdkTool "signtool.exe"
  $subject = $identity.Publisher
  $cert = Get-ChildItem Cert:\CurrentUser\My | Where-Object { $_.Subject -eq $subject -and $_.NotAfter -gt (Get-Date) } | Select-Object -First 1
  if (-not $cert) {
    $cert = New-SelfSignedCertificate -Type Custom -Subject $subject -KeyUsage DigitalSignature `
      -FriendlyName "ForgeDM test package signing" -CertStoreLocation Cert:\CurrentUser\My `
      -TextExtension @("2.5.29.37={text}1.3.6.1.5.5.7.3.3", "2.5.29.19={text}")
  }
  Export-Certificate -Cert $cert -FilePath (Join-Path $OutDir "ForgeDM-test.cer") | Out-Null
  Invoke-Tool $signtool @("sign", "/fd", "SHA256", "/sha1", $cert.Thumbprint, "/s", "My", $out)
}

Write-Host "Built $out"
