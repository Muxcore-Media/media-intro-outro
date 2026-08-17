# Media Intro / Outro

Intro and outro skip-segment detection for MuxCore.

Exposes `muxcore.introoutro.v1.IntroOutroService` in **v0.2.0**:
- Chapter-title classification (`intro` / `outro` / `credits` / `recap`)
- Duration heuristics when chapters are absent
- `Skip` RPC for player seek hints (optional UI hook later)
- In-memory segment store + SettingsProvider
- Offline golden fixtures under `internal/testdata/samples`

## Ports

| Service | Default |
|---------|---------|
| gRPC | `:9710` |
| Health | `:9711` |

## Status

Chapter + heuristic detection with golden fixtures and a `Skip` API for players. Audio fingerprint matching (Chromaprint / series clustering) remains a follow-up.
