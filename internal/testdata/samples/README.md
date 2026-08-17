# Local sample media fixtures

Offline stubs for chapter / heuristic golden tests. Each directory holds:

- `sample.mkv` — empty media stub (no real video; no network)
- `detect_input.json` — Detect input (chapters, duration, max windows)
- `detect.golden.json` — expected Detect segments

Regenerate goldens (from `internal/`):

```bash
go test -run TestDetectGoldenFixtures -update-goldens
```
