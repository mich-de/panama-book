# drivers/ — PaperStream IP for the fi-800R

`libretto` talks to the scanner through **WIA** (works with the Windows/PFU USB driver) or **TWAIN** (PFU's **PaperStream IP** data source).
For TWAIN you need PaperStream IP. It is proprietary software, so it is **not in this repository** and not in the release zips: PFU lets
*fi-series customers install it on any number of computers* (since 2020-06-16) but does not allow redistribution. Get it from PFU:

| Driver | Official page | Direct file (checked: HTTP 200, exact size) |
|---|---|---|
| PaperStream IP (TWAIN) 3.40.2 — **32-bit** data source, the one `libretto` uses by default | https://www.pfu.ricoh.com/global/scanners/fi/dl/setup/psip-twain32-3402.html | `PSIPTWAIN-3_40_2.exe`, 257,447,688 bytes |
| PaperStream IP (TWAIN x64) 3.40.2 — for 64-bit TWAIN applications (`twainscan64.exe`) | https://www.pfu.ricoh.com/global/scanners/fi/dl/setup/psip-twain64-3402.html | `PSIPTWAIN64-3_40_2.exe`, 278,513,672 bytes |

Both installers carry a valid **PFU Limited** Authenticode signature.

## Easiest way

```powershell
.\Get-PaperStreamIP.cmd                  # or:  powershell -File .\Get-PaperStreamIP.ps1 -Arch both
.\Get-PaperStreamIP.cmd -Arch 64 -Install
```

The script downloads from PFU's server into `drivers\download\` (git-ignored), checks **size and signature**, prints the SHA-256, and optionally starts the installer (administrator rights; unplug the USB cable first and plug it back after the install, as PFU recommends).
If PowerShell is restricted on your PC, download the two files from the pages above and run them by hand.

## Notes

- The scanner shows up as `USB\VID_04C5&PID_15FC` (fi-800R). The 32-bit and the x64 driver are separate products and can stay installed together (different product codes).
- Without PaperStream IP, WIA still works: install PFU's USB/WIA driver (Windows Update usually finds it) and use `-driver wia`.
- Windows 11 ARM is not supported by PFU for this scanner.
- Install the **software first, then connect the USB cable**.
- Use `libretto info` (WIA, scans nothing) and `_out\twainprobe32.exe` / `tools\check-twain64.cmd` (TWAIN, read-only) to verify the installation.
