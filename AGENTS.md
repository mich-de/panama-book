# Instructions for AI coding agents

Read **[AI_CONTEXT.md](AI_CONTEXT.md)** before changing anything: it records why the code is the way it is, what was measured, and what was tried and rejected.

Hard rules (each one was learned the hard way):

1. **Never start the scanner** from tests, scripts or tools you run (`libretto scan/read`, `tools\*.cmd`, `twainscan*.exe`). A real document may be in the feeder and the scanner pulls it in. Use `-demo`, the fake scanner, or `extract -i` on copies of images.
2. **Never terminate processes by name** (`Stop-Process -Name`, `taskkill /IM`). Kill only the PIDs you started. A name-based kill in a test script once killed the user's running instance and looked like a crash for hours.
3. **No personal data** in the repository, tests, comments, logs or reports: use invented names/numbers (ROSSI, MARIO, `PA0123456`, `123456789`) and report only counts ("MRZ 5/5", "14/14 fields").
4. **Do not delete user files.** Move leftovers to a quarantine folder and say so; the user deletes.
5. **"Found" is not "correct".** Never claim accuracy from non-empty fields; compare against hand-made ground truth (`cmd/verifyjson`, `cmd/regress -truth`).
6. **Regression gate.** Replace a shipped `libretto.exe` only if `go vet`, `go test` and the regression harness show no regressions on a private corpus.
7. Panama booklets are **not** ordinary passports: MRZ line 1 is `ISPAN` + **NAMES**`<<`**SURNAMES**, document code `IS` ≠ printed `SI`, the optional field is the regional office. See AI_CONTEXT.md §4.
8. Windows `.cmd` files must be **CRLF** and must not depend on Python; `go build` before claiming anything works, and keep `gofmt`/`go vet` clean.
