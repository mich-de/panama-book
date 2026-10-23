# Panama Book

Scan one side of a **Panama Seaman's Book** (*Libreta de embarque*) with a **Fujitsu / PFU / Ricoh fi-800R** scanner, read its data
(**ICAO TD3 MRZ** with check digits + the **14 printed fields**), correct what the OCR got wrong, and export a clean image and a
PDF small enough for a document-management system (≤ 1 MB).

Two programs, one engine:

| Program | What it is |
|---|---|
| **`libretto`** (CLI) | `scan`, `extract`, `read`, `pdf`, `rename`, `summary`: scanning, OCR, JSON output. Go, Windows only, no cgo. |
| **`Panama Book`** (GUI) | Local web app in its own window: preview → read → review → confirm and save. Learns your corrections per booklet. |

> **Privacy first.** A seaman's book is an identity document. Scans, JSON files, debug folders and PDFs contain **personal data of real people**.
> Nothing in this repository is real data (all examples and tests are synthetic), and the program never sends anything over the network.
> Only scan documents you are allowed to process, and read [Privacy and safety](#privacy-and-safety).

---

## Features

- **One side, one pass.** Single-sided scan through the front *Return Scan* path (the only path that takes a rigid booklet up to 5 mm thick). No duplex, no double pass.
- **Two scanner drivers:** WIA 2.0 (COM, no install beyond the Windows driver) and TWAIN (PaperStream IP, through a small helper executable).
- **MRZ reading that does not trust the OCR alone.** Windows' built-in OCR is sometimes silent on perfectly clean OCR-B lines, so the MRZ is read by
  (1) the OCR on whole lines at several scales, (2) the OCR on short pieces, and (3) an **OCR-free glyph-template reader** with a Panama-specific grammar;
  all three are arbitrated by the ICAO check digits.
- **Printed fields** (type, country, nationality, place of birth, dates, level, capacity, book number, ID number, certificate, seafarer's ID, regional office),
  read crop by crop with per-character voting, then **reconciled with the MRZ** (the MRZ wins when its check digits are valid).
- **Panama-specific rules** the generic passport code gets wrong: names come **first** in MRZ line 1, document code `IS` is printed as `SI`, the optional MRZ field holds the regional office, etc.
- **Native OCR, no PowerShell.** `Windows.Media.Ocr` is called directly through hand-written WinRT/COM vtables (about 3.5× faster; identical results).
- **Page crop.** The data page (125 × 88 mm) is cut from the MRZ position, upright, without white borders, 2 mm above the *REPÚBLICA DE PANAMÁ* heading.
- **PDF ≤ 1 MB.** One page, embedded JPEG at the highest quality that fits the limit (typically ~930 KB for a full data page at 300 dpi).
- **Safe file names.** `NAME - SURNAME - BOOK NUMBER.png/.json/.pdf` (ASCII letters, digits and spaces only), only after the operator confirms.
- **Verification tooling:** ground-truth files, a per-field accuracy checker (`verifyjson`), and a regression harness (`regress`).

## Status

Working end to end with a real fi-800R and real booklets. Accuracy on the author's **private** test corpus (63 images with hand-made ground truth;
not included here because it is personal data):

| Measure | Result |
|---|---|
| MRZ found with all 5 check digits valid | 43 / 64 images (all the others are cropped, rotated strips or covers) |
| MRZ-derived names correct | 41 / 41 |
| Printed fields correct (images with ground truth) | 540 / 598 (90 %); 93 % when the MRZ is valid |
| Weakest field | `capacity` when it carries a grade code such as `II/2` (36 / 44) |
| Time per read | 2–4 s (native OCR) |

"Found" is not "correct": see [docs/AI_CONTEXT.md](AI_CONTEXT.md) for how this was measured and what is still open.

## Download

Pre-built, **unsigned** Windows binaries are attached to each [Release](../../releases) (built from this source with `-trimpath`, no personal paths inside):

| File | For | Contains |
|---|---|---|
| `panama-book-win64.zip` | 64-bit Windows (**recommended**) | `libretto.exe`, `PanamaBook.exe`, `twainscan32.exe` + `twainscan64.exe`, `tools\`, `drivers\` (downloader), README, LICENSE |
| `panama-book-win32.zip` | 32-bit Windows | the same programs built for x86, with `twainscan32.exe` only |

Both give identical results (checked byte for byte on the JSON output); the 32-bit build is about 2.5× slower. Windows SmartScreen or an antivirus may warn about unsigned programs: check `SHA256SUMS.txt`, or build from source. The PaperStream IP drivers are **not** included (proprietary, not redistributable): `drivers\Get-PaperStreamIP.cmd` downloads and verifies them from PFU.

## Requirements

- Windows 10/11 (not ARM), a **fi-800R**. Other PFU `fi`/`SP`-series scanners that use PaperStream IP may work (untested). **Other brands (e.g. Epson):** PaperStream IP is PFU-only and does not drive them. The WIA path (`-driver wia -device <part of the scanner name>`) can in principle talk to any scanner with a WIA driver, but the feeder logic, the area handling and the 190 mm recipe are tuned for the fi-800R front *Return Scan* path (a flatbed would need its own acquisition code): untested, contributions welcome.
- **Go 1.22+** to build (the author used 1.27).
- The scanner must be recognized by Windows. For TWAIN: **PaperStream IP (TWAIN) 3.40.2** (32-bit) and optionally **PaperStream IP (TWAIN x64)** from PFU.
  They are proprietary and **not redistributed here**, but `drivers\Get-PaperStreamIP.cmd` downloads them from PFU's own server, verifies size and PFU signature, and can start the installer. See [drivers/README.md](drivers/README.md).
- At least one **Windows OCR language** installed (Settings → Time & language → Language & region → *Optical character recognition*).
- Edge (WebView2 runtime) for the GUI window; any browser works as a fallback.

## Build

```powershell
go build -o libretto.exe ./cmd/libretto
go build -o PanamaBook.exe ./cmd/panamabook
# TWAIN helpers (the PaperStream data source is 32-bit; the x64 build is optional)
$env:GOARCH='386'; go build -o twainscan32.exe ./cmd/twainscan; Remove-Item Env:GOARCH
go build -o twainscan64.exe ./cmd/twainscan
# 32-bit versions of everything
$env:GOARCH='386'; go build -o libretto32.exe ./cmd/libretto; go build -o PanamaBook32.exe ./cmd/panamabook; Remove-Item Env:GOARCH
go vet ./... ; go test ./...
```

Keep `libretto.exe` (and `twainscan32/64.exe`) next to `PanamaBook.exe`.

## Use

### GUI

```powershell
.\PanamaBook.exe              # or tools\start-panamabook.cmd (writes logs to dati\)
.\PanamaBook.exe -demo some-scan.png   # try everything WITHOUT a scanner
```

1. **Preview.** Scans with the recommended settings into a temporary folder (`dati\anteprima\`) — **nothing is written to `scans\` yet**. You see the cropped data page, or the full/raw scan with a diagnosis ("booklet off-centre to the right", "inserted too little", "tilted 1.5°").
2. **Read data.** Fields with a status: *OK*, *To check*, *Not read*, *Corrected from memory*, *Edited by you*, *Checked*. The two MRZ lines and their 5 check digits are shown.
3. **Review.** Edit a field, or press *Checked* on fields marked *To check*.
4. **Confirm and save.** Only now are `NAME - SURNAME - BOOK.png/.json` (and the ground-truth file) created in `scans\`. Export a PDF ≤ 1 MB any time.
5. **Memory.** Corrections are remembered **per booklet** (key = MRZ book number + birth date, only when all check digits are valid) and applied automatically next time.

### CLI

```powershell
.\libretto.exe info                                    # scanner diagnostics, scans nothing
.\libretto.exe scan -yes -o scan.png -driver wia -dpi 300 -len 190   # one side, 190 mm (see below)
.\libretto.exe extract -i scan.png -page-out page.png -top-margin 2  > result.json
.\libretto.exe read -yes -o page.png -len 190 -page-only > result.json  # scan + read in one go
.\libretto.exe pdf -i page.png -o page.pdf -max 1000000
.\libretto.exe rename -json result.json                # NAME - SURNAME - BOOK
.\libretto.exe summary result.json                     # one-line summary, no personal data
```

`scan` and `read` ask for confirmation before the scanner starts (a document already in the front path is pulled in); scripts need `-yes`.
Exit codes: 0 ok, 1 error, 2 usage, 3 no paper, 4 jam, 130 interrupted.

### How to insert the booklet (what 190 mm is about)

Open the booklet on the **photo page**, insert it through the **front opening**, **MRZ edge first**, centred, straight, pushed in fully. The open booklet is ~176 mm long; reading **190 mm**
makes the data page fall inside the scanned area whichever end the driver counts from (WIA counts from the edge you hold; TWAIN from the edge fed first for sheets). The program then finds the MRZ in any orientation and cuts the page.
Details and measurements: [docs/AI_CONTEXT.md](AI_CONTEXT.md#3-scanner-and-driver-knowledge).

## JSON output (`schema_version` 1)

Field names are English and equal to the printed labels. All values below are invented.

```json
{
  "schema_version": 1,
  "rotation_deg": 0,
  "mrz": {
    "doc_code": "IS", "issuer": "PAN",
    "surname": "ROSSI", "given_names": "MARIO",
    "doc_number": "PA0123456", "nationality": "ITA",
    "birth_date": "1990-01-01", "sex": "M", "expiry_date": "2030-12-31",
    "optional": "PANAMA",
    "line1": "ISPANMARIO<<ROSSI<<<<<<<<<<<<<<<<<<<<<<<<<<<<",
    "line2": "...44 characters...",
    "checks": { "doc_number": true, "birth": true, "expiry": true, "optional": true, "composite": true }
  },
  "fields": {
    "type": "SI", "country_code": "PAN", "surname": "ROSSI", "given_names": "MARIO",
    "nationality": "ITALIAN", "nationality_iso": "ITA", "sex": "M",
    "place_of_birth": "GENOVA ITA",
    "date_of_issue": "31-12-2025", "date_of_expiry": "31-12-2030", "date_of_birth": "01-01-1990",
    "level": "SUPPORT", "capacity": "ORDINARY SEAMAN",
    "seamans_book": "PA0123456", "id_number": "C12345678", "seafarers_id": "987654321",
    "certificate": "123456789", "regional": "PANAMA"
  },
  "warnings": [ { "code": "UNCERTAIN_FIELD", "field": "capacity" } ]
}
```

`warnings` carry stable codes (`MRZ_NOT_FOUND`, `MRZ_CHECK_DIGITS_INVALID`, `UNCERTAIN_FIELD`, `PRINTED_FIELD_DIFFERS_FROM_MRZ`, `MRZ_LINE1_UNCERTAIN`, `MRZ_NAME_DIFFERS_FROM_PRINTED`, …).
**A non-empty `warnings` means a human must check the data.** The complete field table is in [docs/campi.md](docs/campi.md) (Italian).

### Helper scripts

`tools/` has Windows `.cmd` helpers (`scan-wia`, `scan-twain`, `extract`, `make-pdf`, `start-panamabook`, `check-twain64`) for people who prefer the command line. See [tools/README.md](tools/README.md).

## Privacy and safety

- **Personal data.** Scans, `-debug` folders, `-page-out/-page-only/-raw` files, JSON, PDFs and `dati\` contain names, photos, dates of birth and document numbers. They are git-ignored; keep them out of cloud sync and issue trackers.
- **Consent / GDPR.** Process only documents you are authorised to handle, and delete what you no longer need. Use synthetic data for tests.
- **Never use the top ADF (U-turn path) for booklets.** It bends and can jam them. If a jam happens: **do not pull**; open the scanner cover, then remove.
- **Nothing in tests touches a scanner.** Tests use a fake scanner and synthetic data; keep it that way.
- The program is **not** an authenticity checker: official verification is via the QR / the Panama Maritime Authority (AMP).

## Architecture

```
cmd/libretto      CLI (scan / extract / read / pdf / rename / summary)
cmd/panamabook    GUI: Go HTTP server on 127.0.0.1 + embedded web UI, runs libretto.exe as a subprocess
cmd/twainscan     TWAIN acquisition helper (32-bit)      cmd/twainprobe  read-only TWAIN capability probe
cmd/regress       regression harness                     cmd/verifyjson  per-field correctness against ground truth
internal/wia      WIA 2.0 over COM (go-ole)              internal/twain  TWAIN client, 32/64-bit agnostic
internal/imgproc  crop, deskew, binarize, rows, page crop internal/ocr    native WinRT OCR (+ PowerShell fallback)
internal/mrz      TD3 parser, check digits, repair       internal/libreta  extraction orchestration, MRZ band, template reader, printed fields
internal/verify   ground-truth comparison and rules      internal/memoria  per-booklet corrections store
internal/filename safe file names                        internal/pdfout   PDF ≤ N bytes
```

Pipeline: *scan → auto-crop (texture based) → white-border trim → rotate → extract* (*deskew → try 0/180/90/270 → MRZ (OCR + pieces + template reader) → printed fields → reconcile with MRZ → warnings → JSON*).

## Testing

```powershell
go vet ./... ; go test ./...                    # unit tests: synthetic data, fake scanner, no hardware
go test -tags integration ./internal/libreta     # needs your own private images (skipped if missing)
go run ./cmd/regress -exe libretto.exe ...      # regression harness over a private corpus (see cmd/regress/main.go)
```

The corpus (scans + hand-made ground truth) is private by nature, so the integration tests and the regression baseline are for **your own** images.

## Known limitations

- One side only; the data page with the MRZ is required. Barcodes (QR/PDF417), photo and signature are not decoded.
- Printed fields have no validation beyond format and agreement with the MRZ; `capacity` with grade codes (`II/2`) and `place_of_birth` over the watermark are the weakest.
- Windows-only; the TWAIN x64 path is untested with hardware; `-width` is untested.
- Calibrated on a small number of booklets (8 titular holders in the template library): it may not generalise to all print runs and models (2017/2019 models were only seen with redacted data).
- The GUI's narrow layout (< 500 px) is unverified.

## Documentation

- **[AI_CONTEXT.md](AI_CONTEXT.md)** — the complete reasoning: constraints, hardware findings, document rules, algorithms, decisions and dead ends. Start here if you are an AI assistant or a new maintainer.
- [AGENTS.md](AGENTS.md) — short rules for AI coding agents.
- [docs/campi.md](docs/campi.md) — fields, formats, warning codes (Italian). [docs/panamabook.md](docs/panamabook.md) — GUI manual (Italian).
- [docs/knowledge/panama-seamans-book.md](docs/knowledge/panama-seamans-book.md) — layout of the booklet (Italian).
- [docs/agenti/](docs/agenti/) — logs of the specialist agents that built each part (Italian), starting from `docs/agenti/README.md` and `COORDINAMENTO.md`.
- [docs/it/](docs/it/) — the original Italian README and code registry (historical; may be outdated).

## Disclaimer

This is an independent project. It is not affiliated with, endorsed by, or supported by PFU Limited, Ricoh, Fujitsu, or the Panama Maritime Authority (AMP).
PaperStream IP is proprietary software of PFU and is not included. Use at your own risk, and only on documents you are entitled to process.

## License

MIT — see [LICENSE](LICENSE).
