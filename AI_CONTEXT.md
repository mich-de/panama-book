# AI_CONTEXT — the complete reasoning behind Panama Book / libretto

This file is written so that an AI assistant (or a new maintainer) can understand **why** the project is the way it is, not just what the code does.
It records goals, constraints, measurements, decisions, dead ends and traps. Where a number appears it was measured on the author's **private** corpus
(real scans cannot be published); treat it as an order of magnitude, not a benchmark. The code comments and most older docs are in Italian (the working language of the author).

Contents: 1 Goal · 2 Rules · 3 Scanner and driver knowledge · 4 The document · 5 Pipeline · 6 The OCR story · 7 Measuring correctness · 8 The GUI · 9 How it was built (agents) · 10 Decision log · 11 Traps · 12 Open work · 13 How to continue

---

## 1. Goal

An operator in a crew-documents office has to digitise **Panama Seaman's Books** (*Libreta de embarque*, AMP-issued, ICAO TD3 size 125 × 88 mm, rigid, up to 5 mm thick) for a document-management system that accepts **PDF attachments of at most 1 MB**.
The operator owns a **Fujitsu/PFU/Ricoh fi-800R**. Requirements, in the order the user stated them:

1. Scan **one side only, in one pass, without double feed**, without damaging the booklet (no U-turn ADF path).
2. Extract the **MRZ** and the **printed fields** of the data page; correct mistakes in a GUI and make the program **learn the corrections** for the next scan of the same booklet.
3. Output **only the data page** (upright, no white borders, a few mm above the *REPÚBLICA DE PANAMÁ* heading), as image + JSON + PDF ≤ 1 MB, with safe file names `NAME - SURNAME - BOOK`.
4. JSON field names **in English, identical to the printed labels** (no Italian/Spanish keys).
5. Drivers installable without the vendor's installer; tidy files; real iterative tests.
6. Later: publish it (this repository), with a README and this reasoning file.

The user is not a programmer. They run tests themselves (scanning is physical), paste results, and expect the assistant to analyse files directly. They often correct the assistant tersely; a correction usually means the assistant misunderstood the physical situation, not the code.

## 2. Rules that must not be broken

| Rule | Why |
|---|---|
| **Never start the scanner** (`libretto scan/read`, `tools\*.cmd`, `twainscan*.exe`) from automated work. | A real document may be in the front path; the fi-800R pulls it in and returns it. Only the user presses the button. Tests use `-demo`, `cmd/panamabook/fakelibretto`, or `extract -i` on copies. |
| **Never kill processes by name.** | A test script doing `Stop-Process -Name PanamaBook` killed the *user's* running instance, twice. It looked like a crash (no log line, empty crash log) and cost hours. Kill only PIDs you started; test with a copy `PanamaBookTest.exe` in a temp folder; build into `_out\` and copy to the root only if no instance runs. |
| **No personal data** in repo/tests/comments/logs/reports. | Scans, JSON, debug folders, PDFs, `dati\` hold real people's data. Use invented values; report counts only ("MRZ 5/5", "13/14 fields"). |
| **Never delete user files without confirmation.** | Leftovers go to a quarantine folder with a manifest; the user deletes. (Temporary files created by the app itself, e.g. discarded previews, are the app's own.) |
| **"Found" ≠ "correct".** | The early metric "14/14 fields" only counted non-empty fields; every distinct 14/14 scan had at least one wrong field. Always compare with hand-made ground truth. |
| **Regression gate.** | A shipped `libretto.exe` is replaced only after `go vet`, `go test` and the regression harness show 0 regressions on the private corpus. Keep a backup of the previous exe. |
| **Windows batch files are CRLF**, and must not need Python or PowerShell. | LF-only `.cmd` files fail with absurd errors; the user's PC has no Python (only a Store stub), and PowerShell processes are intermittently killed there (§6). |

## 3. Scanner and driver knowledge

### 3.1 Hardware facts (fi-800R)
- Two feed paths: **top ADF (U-turn)** — bends the document; **front "Return Scan"** — document is pulled in through the front opening and comes back out the same way; takes up to 5 mm. Booklets must use the front path. The software cannot choose the path; it is decided by where the user inserts the document. WIA exposes a single "feeder".
- The driver reports "feeder ready" even with no paper (`Document Handling Status` bit 1), so *paper present* cannot be tested.
- Scan area at 300 dpi: 216 × 356 mm frame (2550 × 4200 px); background of the empty scan is light grey (~237–244), not white; with TWAIN the outside is pure white (255).
- If the booklet jams: do not pull; open the cover.

### 3.2 WIA 2.0 (default of the CLI)
- One item "Scan". **No "front only" flag**: `Document Handling Select = 1` (feeder, without the duplex bit 4) is set and the scan is refused if duplex still reads as active. `Pages = 1` is mandatory.
- Area: `Vertical Extent` (length) **works** (`-len N` → 216×N mm). **`Vertical Start` is accepted but ignored** (measured displacement 0.0 mm). Changing the start makes the driver recompute the extent, so set start then extent and **re-read**.
- `Item` in WIA collections is a *property get* in go-ole (`GetProperty(..., "Item", i)`), not a method. Each WIA operation runs on a locked OS thread in an STA apartment.
- Because the start offset is ignored, the only way to include the data page is to read a long stretch from the reference edge: **190 mm** (booklet 176 mm + margin). This was the "recipe that worked" (`tools\scan-wia.cmd`).

### 3.3 TWAIN (PaperStream IP)
- Installed data source is **32-bit** (`C:\Windows\twain_32\Fjicube\Fjic*.ds`, DSM only in `SysWOW64`) → a separate 32-bit helper `twainscan32.exe` (Go `GOARCH=386`, no cgo, raw `DSM_Entry` calls with hand-built `TW_*` structs; layouts verified for 32 and 64 bit). The 64-bit driver ("PaperStream IP (TWAIN x64)", separate PFU installer) was later installed; the code is bitness-agnostic and `twainscan64.exe` is preferred when `C:\Windows\twain_64` exists.
- **Drivers are not redistributed.** PaperStream IP is proprietary (PFU allows *installing* it on any number of PCs since 2020-06-16, not redistributing it; the two installers are 257 MB and 278 MB, above GitHub's 100 MB file limit anyway). The repo ships `drivers/Get-PaperStreamIP.ps1`, which downloads them from PFU's own server (`origin.pfultd.com`), checks exact size and the *PFU Limited* Authenticode signature, and can start the installer. During the project a driver-only extractor for PFU's installer archive (a reverse-engineered "Arcmanager" self-extractor format) was written to install just the MSI; it is **deliberately not published** (reverse-engineering/EULA grey area) — the official installer works fine.
- `ICAP_FRAMES` **accepts the top offset** once the frame fits the current paper size (A4 = 8.268 × 11.693 in; a frame wider than the paper gives `BADVALUE` — the first probe wrongly blamed the offset). `libretto` tries "recipes" in order (default frame, `SUPPORTEDSIZES` variants, custom paper, cm/pixel units, …), re-reads the frame and **aborts before starting the scanner** if the driver changed it.
- Measured behaviour: the offset **is** honoured, but it is measured from the **edge fed first** for the printed sheets used in the tests (data page at 0–90 mm from the MRZ edge); for an open booklet WIA counts from the edge you hold. Rather than guess per object, the final recipe reads **190 mm** with either driver, and the data page is cut afterwards from the MRZ position.
- The TWAIN image arrives **rotated 180°** → `-rotate 180` (applied after the auto-crop, so the crop works on the raw image and the rotation is applied last). WIA images are upright: no rotate.
- Traps found with the vendor test tool (`ScanTest.exe`): driver defaults persist in the "current selection" (duplex on, automatic size detection, automatic rotation, blank-page removal). The helper sets what it needs explicitly (single page, simplex, units, frame, resolution) and never retries a transfer.
- `ICAP_EXT_*` capabilities (frame mode, trim, paper protection, synchronous ejection…) were catalogued from the driver tables (`capnames.go`, 352 entries); `CAP_EXT_PAPERPROTECTION`, `MANUALFEEDERENABLED`, `SYNCHRONOUSEJECTION` are **not** supported by this driver. There is no "stop in the middle" setting: the document always passes through physically.
- Error code `0xC0400065` appeared on processes killed externally (see §6).

### 3.4 Image post-processing after the scan
`AutoCrop` (reduce to ~800 px, background = median of a frame, "document" = deviates from the background **and** has local texture or strong contrast; connected components; skew estimate ±6°) → `TrimWhite` (luminance profile of rows/columns over the central 60 %, cut where 8 consecutive lines fall below 236; max 20 % per side; no-op on grey WIA backgrounds) → `-rotate`.
A white-on-white document is undetectable and is treated as empty.

## 4. The document: Panama Seaman's Book

(Full layout notes: `docs/knowledge/panama-seamans-book.md`, Italian.)

- Open booklet = two pages: the **inside cover (legal notice, "informativa")** and the **data page** (VIZ on top, MRZ at the bottom). Open flat the pair is ~125 × 176 mm; the data page is 88 mm.
- Models: 2017 (PDF417), 2019, 2020 (QR), 2026 (QR; some holders have an extra number field). Layouts for 2017/2019 were only seen with redacted data.
- **MRZ = ICAO TD3, 2 × 44 characters**, weights 7-3-1, composite check digit. Panama specifics (verified on 2020 and 2026 samples and on real scans):
  - **Line 1 = `IS` + `PAN` + NAMES `<<` SURNAMES** — given names **first**, the opposite of ICAO. A generic parser swaps `surname`/`given_names`. The parser is kept ICAO-ordered internally; `cmd/libretto/output.go:panamaNameOrder` swaps them for `IS`+`PAN` when both parts exist, in `fields` and in the `mrz` block. (This bug was found late, while building the file-name feature; the user's reminder "this is a Panama booklet, not an ordinary passport" was the key hint.)
  - Document code **`IS`** in the MRZ, **`SI`** printed on the page (do not "fix" it).
  - Line 2: book number (9 chars, `PA` + 7 digits observed) + check + holder's nationality (3 letters, may be `PAN`, `DEU`, `NIC`…) + birth `YYMMDD` + check + sex + expiry `YYMMDD` + check + **optional field = regional office name** (`PANAMA`, `NAPOLES`…, not a personal number) + check + composite.
  - Line 1 has **no check digit** → its correctness is verified only by length (44 exactly, `ISPAN` prefix), by the template reader, and by comparing with the printed name.
- Printed `Nationality` is an English adjective ("PANAMANIAN"), not an ISO code; use the MRZ ISO3 code (a 199-entry ISO3→adjective table is in `viz_nationality.go`).
- Dates are `DD-MM-YYYY` printed, `YYMMDD` in the MRZ. Validity is variable (≈5 years on 2017/2020 samples, ≈5 months on one 2026 sample): never assume.
- **Field name correction:** the extra 9-digit field was first named `endorsement` (assumed *Nº de Refrendo/Endorsement*). A zoom on the official AMP facsimile showed the printed label is **`Nº Título/Certificate`** → JSON key **`certificate`** (the user's rule: JSON names equal printed labels). It exists only for some holders (officers with a graded capacity).
- `Cargo/Capacity` can be long, multi-word, with a grade code (`MASTER II/2`, `ORDINARY SEAMAN`, `HOTEL STAFF`); it is printed small and bold over the *PANAMA* watermark — the hardest field.
- A restriction text (`ONLY VALID ON BOARD <company>`) is overprinted on the hologram: not OCR noise.

## 5. Pipeline

```
scan (WIA or TWAIN, 1 side, -len 190) → AutoCrop → TrimWhite → [-rotate 180 for TWAIN] → PNG (+ raw)
extract:  Load → CropContent → Deskew (>2°, ±12°) → try rotations 0/180/90/270 →
          readMRZBand (OCR)  +  readMRZCells (template, OCR-free)  → arbitrated by check digits (mrzmerge)
          → printed fields (VIZ: crops → OCR → per-character vote) → reconcile with MRZ → warnings → JSON
page crop: MRZ rows → scale (44 chars × 2.54 mm) → 125×88 mm rectangle, bottom = 7.5 mm below line 2, edge refinement → trim to the title (+2 mm)
```

### 5.1 MRZ band (`internal/libreta/mrzband.go`, `internal/imgproc/rows.go`)
Binarize (threshold between background and ink) → remove dark vertical runs (`ClearLines`) → find text rows by projection → pick the lowest pair with similar height/left margin → crop rows at several target heights (32/45/60 px) → one OCR call → `mrz.Repair` (confusable characters O/0, I/1, S/5… accepted **only if it is the unique fix that also satisfies the composite check**, because a 1-digit check passes by chance 1 time in 10) → if a row is empty or wrong, read **44-cell pieces** with several cuts (prefix of known text for pieces made only of `<`, which the engine otherwise returns empty) and vote per cell.
Fixes that mattered (each found on a specific real failure):
- `ClearLines` deleted whole columns darker than 35 % ink — a dark *photo* above the MRZ erased the first characters "ISPAN"/"PA012". Now only long vertical runs are removed.
- Rows tilted 1.2° (below the 2° deskew threshold) made the engine return nothing → `EstimateRowsSkew`/`ShearRows` straighten the band by integer pixel shifts (no interpolation).
- On one clean scan the engine returned empty text on every whole row (and on rotated/blurred/padded/inverted variants) but read **5–15-character pieces** fine; the piece pass now also runs when both lines are empty.
- `readMRZBand` score: 0 = not found; +1, +2 per valid check, +2 if issuer `PAN` (max 13) **+1 if line 1 is exactly 44 chars with `ISPAN`** (14 = "sure").
- Time cap per band (`maxBandTime`, 8 s) exists to avoid a runaway; under heavy CPU contention (e.g. 12 parallel processes) it gives false misses. Run the regression harness with ≤ 3 processes.

### 5.2 Template reader (`mrzcells*.go`, `glyphs.bin`)
Because the Windows engine is capricious (§6), an OCR-free reader was added: 44-cell grid per row, per-cell normalisation (height, stroke-width compensation), nearest-template classification against **707 glyph bitmaps** (A–Z, 0–9, `<`; 98 KB, bitmaps only, harvested from correctly read scans), exact decoding with the Panama **grammar** (line 1: `IS PAN` + letters/`<`; line 2: digit/letter/`<`/`M|F` per position) and the **5 check digits as hard constraints** (a wrong printed check yields "no reading", never a "fixed" one).
Evaluation was **leave-one-booklet-out** (the library never contains cells from the same holder): 54 images, 8 holders — line 2 100 %, line 1 99.66 % of characters, whole MRZ right 47/54, 0 false positives on 162 wrong rotations, ~79 ms per row. Weak letters (B, Q, Y, W…) have 1–3 glyphs. It registers itself through `init()` into a hook (`mrzCellsReader`) declared in `extract.go`, so the project compiles without it.
`mrzmerge.go`: the cell reader always runs (cheap); where it differs from the OCR on line 1 / nationality / sex **and its differing cells are not flagged uncertain**, its reading wins; otherwise the OCR is kept and `MRZ_LINE1_UNCERTAIN` / `MRZ_FIELD_UNCERTAIN` is raised. (It showed that on 5 of 54 images the OCR line 1 was wrong *even with 5/5 check digits*.)

### 5.3 Printed fields (`viz*.go`)
The layout is fixed, so field positions are constants relative to the MRZ width. Each label+value region is cropped in several scale/window variants, stacked into a "sheet" and read in one OCR call, plus a second pass with one crop per image; the value is chosen by **per-character majority** with weights calibrated on digits known from the MRZ, after format recognition (dates, 9-digit numbers, book number `PA` + 7 digits also reconstructed from the digits alone, level, nationality via the ISO table). Thick-stroke prints (300 dpi laser) get 11 thinned/binarised variants; free-text fields get ~24 jittered crops (±3–5 px) because bold text over the hologram is fragile. `vizFinalize` takes MRZ-derived values when the check digits are valid and emits `UNCERTAIN_FIELD` for weak readings. A **printed-name check** (`namecheck.go`) compares the printed name row with the MRZ names (word-wise, order-insensitive) — currently almost never fires because the confidence of the printed-name reading is not discriminating.
Calibration risk: tuned on ~9 holders; closed lists of values were deliberately **not** used (a first attempt snapped capacity to a closed list and was removed).

### 5.4 Page crop and header trim (`pagecrop.go`, `header.go`)
Geometry comes only from the two MRZ rows (width 44 chars × 2.54 mm − slack → px/mm; line 2 sits `td3BottomMM = 7.5` mm above the page bottom), refined with real page edges. `TrimAboveHeader` finds the blue *REPÚBLICA DE PANAMÁ* text (strong-blue pixels in the central 40 % columns, 3 consecutive rows) and cuts everything above it minus `-top-margin` mm (default 2). The user explicitly asked for "2–3 mm above the heading".

### 5.5 PDF ≤ 1 MB (`internal/pdfout`)
The image is embedded as DCT (JPEG) with no re-encoding; binary search of quality 95→80, then downscale by 7 % steps (never below 50 %). Limit 1,000,000 bytes (not 1,048,576) in case the DMS counts decimal MB. A full data page at 300 dpi lands at ~930–950 KB, quality 92; MRZ/photo/text stay sharp and the extraction results from the PDF render were identical to the PNG. (Experiments: 4:4:4 vs 4:2:0, blur-before-JPEG did not pay off; Go's encoder is 4:2:0 only.)

## 6. The OCR story (why there is a native WinRT OCR)

- Engine: **`Windows.Media.Ocr`** with the user-profile language (only `it-IT` was installed; the engine is not trained on OCR-B). It is **capricious**: the same clean line can return empty text at one scale and perfect text at another (resampling filter matters: Lanczos ×1.3 of the whole image worked where the program's resampling did not); thick strokes fill the gaps between `<` and produce nothing; whole-page OCR drops `<` symbols.
- Mitigations in order of discovery: multi-scale row crops → thinning variants (adaptive ink fraction + check-digit oracle) → cell/piece reading → straightening → OCR-free template reader.
- **PowerShell was the weak link.** Originally OCR ran as `powershell.exe -ExecutionPolicy Bypass -File winocr.ps1` (~1 s start-up per call). On the user's managed PC a security control **intermittently terminates PowerShell processes with exit code `0xC0400065`**; inside the GUI's process tree (`PanamaBook.exe` → `libretto.exe` → `powershell.exe`) the whole tree was killed at the second PowerShell of a read, with no log line and an empty Go crash log. Evidence: the same exit code on the Edge window process and on an OCR PowerShell at other times; no killer process visible; identical chains launched from other parents survived. (An earlier suspected cause — the agent's test scripts killing by name — was real *and* fixed, but did not explain the second series of crashes.)
- Fix: `internal/ocr` calls WinRT **directly** (`combase.dll`: `RoGetActivationFactory`, `OcrEngine`, `SoftwareBitmap` from raw Bgra8 pixels, `IAsyncOperation` polled via `IAsyncInfo`, one engine per in-flight image because an engine rejects overlapping `RecognizeAsync` with `E_ABORT`, one locked OS thread per call, cancellation through `IAsyncInfo::Cancel`). Verified identical text/words/boxes (0 px difference) on 72 pages and 309 crops; JSON identical on 10 images; one OCR call 946 ms → 25–39 ms; whole `extract` ≈ 3.5× faster. PowerShell stays as a fallback; `LIBRETTO_OCR=powershell|native` forces one.
- Corollary: `tools\start-panamabook.cmd` shows log tails in pure batch (a `:coda` routine with delayed expansion so `->` or `&` in log lines cannot redirect or run).

## 7. Measuring correctness

1. **Ground truth by eye**: `scans\verita\NAME.json` (same English field names; empty = not printed/illegible; `_note` explains). Written by a verifier agent looking at the images (zoomed crops for small print) and cross-checked with valid MRZ values (0 discrepancies for birth/expiry/book number). Stored locally, git-ignored (personal data).
2. `cmd/verifyjson`: per field `OK / SBAGLIATO / MANCANTE / NON_STAMPATO / IN_PIU`, without printing values, plus value-free consistency rules (formats, date order, nationality adjective vs ISO, MRZ vs printed, odd characters). `certificate` is accepted under its old name `endorsement` as an alias.
3. `cmd/regress`: runs an exe over the private corpus (3 processes), prints a value-free TSV (MRZ yes/no, checks n/5, fields found, fields **correct** with `-truth`, names, warnings, seconds), compares with a baseline and exits 1 on **REGRESSION**; remaps paths when files move. Every candidate build is measured before installation.
4. Headline history (private corpus): MRZ valid 33 → 43 of 64; correct fields 477 → 540 of 598; `capacity` with grade codes 0/29 → 36/44 overall.
5. Lessons: report **correct**, not **found**; a leave-one-out protocol for any learned component; keep the corpus fixed while comparing builds (the corpus changed once and the comparison had to be remapped); never judge by a single image — the same booklet scanned twice behaved differently.

## 8. The GUI (Panama Book)

- **Architecture decision:** a small **Go HTTP server on 127.0.0.1 (random port, per-session token, Host/Origin/Sec-Fetch-Site checks, CSP, images only from `scans\`)** + embedded static web UI, opened in **Edge `--app`** with a private profile (`dati\edge`). No cgo, no external dependency. It runs `libretto.exe` as a subprocess (`scan`, `extract -i`, `pdf`) instead of re-implementing anything; it can be tested with a **fake `libretto`** (`cmd/panamabook/fakelibretto`) and `-demo FILE`.
  Rejected: Fyne/Walk (cgo or heavy), a TWAIN-in-process GUI (32-bit data source), a pure console tool (the operator needs a preview and corrections).
- **Flow decisions (all from user corrections):**
  - *Preview before saving.* `Preview` scans into `dati\anteprima\` (temp, with the raw scan and a diagnosis JSON) — **nothing in `scans\` until "Confirm and save"**. Reason: wrong scans (booklet inserted off-centre or not far enough) kept littering `scans\`. The preview auto-runs `extract -page-out` so the default view is the **data page only**; views: *Page / Full / Raw*; a strip keeps the last 5 previews.
  - *Diagnosis from the raw scan* (no OCR): off-centre left/right with how many cm, inserted too little (< 60 % of the expected length), tilt > 1.5°, no document. (Left/right sides still unverified against hardware.)
  - *Rename only after confirmation*, from the confirmed form values: `NAME - SURNAME - BOOK` via `internal/filename` (ASCII letters/digits/spaces, ≤ 45 chars per part, `(2)` instead of overwriting, all-or-nothing rename with rollback). Renaming right after the read was wrong: the operator may still correct the names.
  - *Statuses and review:* OK / To check / Not read / Corrected from memory / Edited by you / **Checked** (button, Enter on an unmodified field, Alt+C undo). A counter shows how many are open; confirming with open fields asks, and the ground-truth file then omits those fields.
  - *Learning:* corrections are stored **per booklet**; identity key = MRZ book number + MRZ birth date **only if all check digits are valid** (never an OCR key). Confirmed-as-correct values are remembered too ("Checked from memory"). File `dati\memoria.json` (atomic write, 0600, git-ignored). Learning across *different* booklets (alias of recurring misreads) is **not** implemented.
  - "Start over" (Alt+N; Ctrl+N is reserved by the browser) clears the screen without deleting anything. Whether the app should reset automatically after saving was asked and **left undecided** (the request was interrupted).
  - Defaults finally match the proven recipe: **WIA, 300 dpi, 190 mm, 2 mm**; TWAIN selectable (then `-from 0 -rotate 180`).
- **Reliability:** `dati\panamabook.log` (every event, subprocess codes, "alive" each minute, `USCITA: reason`), `dati\panamabook-crash.log` (`debug.SetCrashOutput`), `recover` in every goroutine, exit only when no job is running and no heartbeat for 60 s (3 min safety net with our own Edge), no navigation outside the single page. A missing `USCITA` line means *killed from outside* (`TerminateProcess` cannot be intercepted).

## 9. How it was built: specialist agents with file ownership

The work was split among Claude agents with **disjoint file ownership**, coordinated through `docs/agenti/COORDINAMENTO.md` (owners, contracts, status, open requests) and one log per agent in `docs/agenti/`:

| Role | Owns | Notes |
|---|---|---|
| MRZ (OCR path) | `mrzband.go`, `rows.go`, `pagecrop.go`, `deskew.go` | root causes of the three "clean scan but no MRZ" cases |
| MRZ template reader | `mrzcells*.go`, `glyphs.bin` | leave-one-booklet-out evaluation |
| VIZ (printed fields) | `viz*.go` | per-character voting, ISO table, jitter |
| WIA / TWAIN | `internal/wia`, `internal/twain`, `autocrop.go`, `trimwhite.go` | driver probes without hardware |
| Native OCR | `internal/ocr` | WinRT vtables |
| Verifier | `internal/verify`, `cmd/verifyjson`, ground truth | measured the real accuracy |
| Integrator | `extract.go`, `cmd/libretto`, `cmd/regress`, builds, `tools\` | **only it replaces `libretto.exe`**, after the regression gate |
| GUI | `cmd/panamabook`, `internal/memoria` | |
| Docs | README/REGISTRO/campi | |

What this taught (useful if you repeat the method): give each agent an explicit file list and forbid touching others'; extension points are **hooks registered in `init()`** so nobody edits foreign files; candidates are built to `_out\` and measured by the integrator; agents must never run hardware or kill by name; stop an agent when the user changes their mind (an interrupted agent leaves half-finished edits — rebuild and re-test before trusting the tree); transient compile errors in the IDE during parallel edits are normal — rely on `go build`/`go vet`.

## 10. Decision log

| # | Decision | Reason | Rejected / history |
|---|---|---|---|
| D1 | Go, Windows only, no cgo | single exe, no toolchain on the PC | Python (not installed on the user's PC) |
| D2 | WIA first, TWAIN added | WIA needs no extra install; TWAIN offered the area offset | TWAIN-only |
| D3 | Read **190 mm** and crop the page afterwards | start offset is driver- and object-dependent (WIA ignores it; TWAIN counts from the fed edge) | trying to read only 88–100 mm ("the user wants the engine to stop at the fold") — physically impossible, the booklet always passes through |
| D4 | `-rotate` applied **after** auto-crop; only for TWAIN | TWAIN delivers 180° rotated | rotating first (no benefit) |
| D5 | MRZ arbitrated by check digits; line 1 needs a second opinion | line 1 has no check digit | trusting the OCR line 1 |
| D6 | Template reader independent of the OCR | the engine can be silent on perfect lines | more OCR tricks only |
| D7 | Native WinRT OCR | PowerShell processes get killed on the managed PC; 3.5× faster | whitelisting/signing only (needs IT; kept as an extra option) |
| D8 | JSON keys = English printed labels, `schema_version` 1 | user rule | Italian/Spanish keys (removed) |
| D9 | `certificate` not `endorsement` | printed label is *Nº Título/Certificate* | `endorsement` (kept as alias when reading) |
| D10 | Names swapped in the **output layer** for `IS`+`PAN` | internal ICAO parser and synthetic tests stay valid | changing `mrz.Parse` semantics |
| D11 | Preview-without-saving; rename after confirmation | user corrections | immediate save/rename |
| D12 | GUI = local web app, not a native toolkit | no cgo, easy to test with a fake backend | Fyne/Walk |
| D13 | Memory keyed by MRZ-valid identity | avoids applying corrections to the wrong person | key from OCR text |
| D14 | PDF as embedded JPEG, limit 1,000,000 B | DMS limit 1 MB | re-rendering, grayscale, denoise |
| D15 | Ground truth by eye + value-free tooling | privacy, honest accuracy | counting non-empty fields |
| D16 | No closed lists of capacities/places | would overfit 9 holders | snap-to-list (tried, removed) |

## 11. Traps (things that cost time)

- **Git Bash path conversion:** `cmd.exe /c ...` becomes `cmd.exe C:/ ...` (interactive shell, hangs). Use PowerShell `Start-Process` with a timeout, or `MSYS_NO_PATHCONV=1`.
- `NoDefaultCurrentDirectoryInExePath` is set in the harness: call `.\libretto.exe`, not `libretto.exe`; Python subprocesses need absolute exe paths.
- LF-only `.cmd` files; `.cmd` redirection (`>`) writes **no BOM**, PowerShell `>` does — `libretto rename` cannot read a BOM JSON.
- Python `\t`, `\N`, `\U` in strings inside heredocs silently corrupt Windows paths (`C:\Windows\twain_64` became a TAB); use raw strings or the editor tool.
- gopls shows stale "undefined"/"redeclared" errors while several agents edit; trust `go build ./...`. `go test -race` needs cgo (not available here): stress tests were used for concurrency.
- Orientation reasoning errors cost many iterations: *which edge is the reference* depends on driver **and** object (sheet vs booklet). When in doubt, read longer (190 mm) and let the MRZ decide.
- A flaky engine result on **one** image is not a bug in the geometry: check scale/skew/resampling first.
- `provamrz`-style tests used a **printed A4 facsimile**; real booklets behave differently in the feeder. Always test the real object.
- The tests print `avviso: lettore MRZ a celle in errore (indice fuori dai limiti)`: a recovered index-out-of-range in the template reader on a synthetic input (falls back to the OCR reading). Harmless but worth fixing.

## 12. Open work

1. `capacity` with grade codes (36/44 correct) and `place_of_birth` (35/43) — VIZ.
2. Fill printed fields from a **valid MRZ** wherever they are derivable (≈11 fields recoverable) — changes field semantics, deliberately not started.
3. Make the printed-name confidence discriminating so `MRZ_NAME_DIFFERS_FROM_PRINTED` actually fires.
4. About 10 wrong fields still carry no `UNCERTAIN_FIELD` (silent errors).
5. GUI: auto-reset after "Confirm and save" (undecided), cross-booklet alias learning, manual crop, installer/signing, narrow layout, hardware check of the left/right diagnosis.
6. Hardware checks not done: TWAIN x64 with a booklet, `-width`, long-run stability, Ctrl+C during a physical scan.
7. `internal/libreta/viz.go` is ~2,400 lines: split it (a refactor plan is in `docs/agenti/08-piano-rifattorizzazione.md`).
8. If security software is confirmed, ask IT to allow-list/sign `PanamaBook.exe`, `libretto.exe`, `twainscan*.exe`.

## 13. How to continue

```powershell
go build -o libretto.exe ./cmd/libretto ; go vet ./... ; go test ./...
go build -o PanamaBook.exe ./cmd/panamabook     # never overwrite a running exe
# regression over YOUR private corpus
go run ./cmd/regress -exe .\candidate.exe -baseline baseline.tsv -truth path\to\truth
```

Conventions: Italian comments/identifiers in the older code (new code may be English), `gofmt` + `go vet` clean, one logical change per commit, no personal data anywhere, update `docs/agenti/COORDINAMENTO.md` when ownership/contracts change.
If you are an AI agent continuing the work: read §2 and `AGENTS.md` first, reproduce with `-demo`/fake backends, ask the user to perform any physical scan, and report **counts**, never values.
