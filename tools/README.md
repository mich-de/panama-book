# tools/ — helper scripts (Windows `.cmd`, CRLF, no Python needed)

Run them from the repository root (they `cd` there themselves). `libretto.exe` (and `twainscan32/64.exe` for TWAIN) must sit in the root.
Scripts marked **scanner** start the scanner after a `pause`: put the booklet in first, MRZ edge first, centred and straight.

| Script | Scanner | What it does |
|---|---|---|
| `scan-wia.cmd [NAME] [DPI] [LEN_MM]` | **yes** | WIA scan + read in one go (`read -len 190 -page-only`): `scans\NAME.png` is the cropped data page, `NAME.json` the result. The recipe that works on real booklets. |
| `scan-twain.cmd [NAME] [LEN_MM]` | **yes** | TWAIN scan of one side (`-len 97` by default, rotated 180°), cropped to the document, no reading. |
| `extract.cmd NAME` | no | Reads `scans\NAME.png` → `NAME.json` and, if the MRZ is valid, renames the files to `NAME - SURNAME - BOOK`. |
| `make-pdf.cmd NAME` | no | `scans\NAME.png` → `scans\NAME.pdf`, one page, ≤ 1,000,000 bytes. |
| `start-panamabook.cmd` | no (the GUI asks before scanning) | Starts `PanamaBook.exe` and writes its logs to `dati\`; at the end shows the log tails (pure batch). |
| `check-twain64.cmd` | no | Read-only check of the 64-bit TWAIN driver (`twainprobe64`); writes `scans\twain64-probe.txt`. |

Every scanning script refuses to overwrite an existing `scans\NAME.*` and, when it finishes, stays open and asks for the next name (empty line = exit).

Earlier development names (they appear in the historical logs under `docs/agenti/`): `prova-libretto.cmd` → `scan-wia.cmd`, `scansiona.cmd` → `scan-twain.cmd`,
`estrai.cmd` → `extract.cmd`, `pdf.cmd` → `make-pdf.cmd`, `panamabook.cmd` → `start-panamabook.cmd`, `verifica-twain64.cmd` → `check-twain64.cmd`.
The experiment scripts (`prova-twain*.cmd`, `prova-pagina.cmd`, `prova.cmd`) and `summary.py` were dropped: they were superseded by the GUI and `libretto summary`.
