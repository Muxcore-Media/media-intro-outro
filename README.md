# Media Intro / Outro

Intro and outro skip-segment detection for MuxCore.

Exposes `muxcore.introoutro.v1.IntroOutroService` in **v0.1.0**:
- Chapter-title classification (`intro` / `outro` / `credits` / `recap`)
- Duration heuristics when chapters are absent
- In-memory segment store + SettingsProvider

## Ports

| Service | Default |
|---------|---------|
| gRPC | `:9710` |
| Health | `:9711` |

## Status

Scaffold — audio fingerprint matching (Chromaprint / series clustering) and playback client integration are follow-ups.
