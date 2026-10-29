<#
  Downloads the official PaperStream IP (TWAIN) installers for the fi-800R straight from PFU's server,
  verifies size and digital signature (PFU Limited), and optionally starts the installer.
  The proprietary installers are NOT part of this repository: PFU grants the right to INSTALL them on any number
  of computers (since 2020-06-16) but not to redistribute them.

  .\Get-PaperStreamIP.ps1                 download both (32-bit and x64) into .\download
  .\Get-PaperStreamIP.ps1 -Arch 32        only the 32-bit driver (the TWAIN data source that libretto uses by default)
  .\Get-PaperStreamIP.ps1 -Arch 64 -Install   download the x64 driver and start its installer (asks for administrator rights)
#>
param(
    [ValidateSet('32', '64', 'both')][string]$Arch = 'both',
    [string]$Dest = (Join-Path $PSScriptRoot 'download'),
    [switch]$Install
)
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'   # Invoke-WebRequest is much faster without the progress bar

$packages = @(
    @{ Arch = '32'; Name = 'PSIPTWAIN-3_40_2.exe';    Size = 257447688
       Url = 'https://origin.pfultd.com/downloads/IMAGE/fi/psip-twain32/3402/PSIPTWAIN-3_40_2.exe'
       Page = 'https://www.pfu.ricoh.com/global/scanners/fi/dl/setup/psip-twain32-3402.html' },
    @{ Arch = '64'; Name = 'PSIPTWAIN64-3_40_2.exe';  Size = 278513672
       Url = 'https://origin.pfultd.com/downloads/IMAGE/fi/psip-twain64/3402/PSIPTWAIN64-3_40_2.exe'
       Page = 'https://www.pfu.ricoh.com/global/scanners/fi/dl/setup/psip-twain64-3402.html' }
)
New-Item -ItemType Directory -Force $Dest | Out-Null

foreach ($p in $packages) {
    if ($Arch -ne 'both' -and $Arch -ne $p.Arch) { continue }
    $file = Join-Path $Dest $p.Name
    if (Test-Path $file) {
        Write-Host "$($p.Name) already downloaded."
    } else {
        Write-Host "Downloading $($p.Name) ($([int]($p.Size / 1MB)) MB) from PFU..."
        Invoke-WebRequest -UseBasicParsing -Uri $p.Url -OutFile $file
    }
    $len = (Get-Item $file).Length
    if ($len -ne $p.Size) { throw "$($p.Name): size $len, expected $($p.Size). Delete it and retry, or download it from $($p.Page)" }
    $sig = Get-AuthenticodeSignature $file
    Write-Host "  signature: $($sig.Status) - $($sig.SignerCertificate.Subject)"
    if ($sig.Status -ne 'Valid' -or $sig.SignerCertificate.Subject -notmatch 'PFU Limited') {
        throw "$($p.Name): the digital signature is not a valid PFU Limited signature. Do not run it."
    }
    Write-Host "  sha256:    $((Get-FileHash $file -Algorithm SHA256).Hash)"
    if ($Install) {
        Write-Host "Starting the installer (administrator rights required). Unplug the scanner USB cable first; plug it back after the install."
        Start-Process -FilePath $file -Verb RunAs -Wait
    }
}
Write-Host 'Done.'
