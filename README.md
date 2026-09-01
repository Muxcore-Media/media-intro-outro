# Media Intro / Outro

Intro and outro skip-segment detection for MuxCore.

Exposes `muxcore.introoutro.v1.IntroOutroService` in **v0.2.0**:
- Chapter-title classification (`intro` / `outro` / `credits` / `recap`)
- Duration heuristics when chapters are absent (gated by `min_confidence`)
- Series segment reuse across episodes with the same show prefix
- `Skip` RPC for player seek hints
- JSON file persistence for segments and settings under `INTRO_OUTRO_DATA_DIR`
- SettingsProvider (`intro_max_seconds`, `outro_max_seconds`, `heuristic_enabled`, `min_confidence`)
- Offline golden fixtures under `internal/testdata/samples`

## Ports

| Service | Default |
|---------|---------|
| gRPC | `:9710` |
| Health | `:9711` |

## Environment

| Variable | Default | Description |
|----------|---------|-------------|
| `MUXCORE_GRPC_ADDR` | `:9710` | gRPC listen address |
| `MUXCORE_HTTP_ADDR` | `:9711` | HTTP health listen address |
| `INTRO_OUTRO_DATA_DIR` | _(none)_ | Directory for `segments.json` and `settings.json` persistence |
| `INTRO_MAX_SECONDS` | `180` | Heuristic intro window (seconds) |
| `OUTRO_MAX_SECONDS` | `240` | Heuristic outro window (seconds) |
| `MVP_ENABLE_MEDIA_INTRO_OUTRO` | `0` | Enable module in `_mvp/run-host.sh` |

When `INTRO_OUTRO_DATA_DIR` is unset, segments and settings are memory-only (lost on restart).

## Status

Chapter + heuristic detection with golden fixtures, series reuse without Chromaprint, and a `Skip` API for players. Audio fingerprint matching (Chromaprint / live series clustering) remains a follow-up.
