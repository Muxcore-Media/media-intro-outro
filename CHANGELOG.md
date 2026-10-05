# Changelog

## [0.2.3] - 2026-10-05


### Security
- gRPC server and peer dials use mesh TLS (meshtls, sdk/go/module v0.6.5) unless the dev insecure flag is set (ADR-0016/0017).

## [0.2.2] - 2026-10-05

### Changed
- Built on core v0.6.14 / sdk/go/module v0.6.4: unregisters on shutdown and re-registers after core restarts (ADR-0022).

## [0.2.1] - 2026-10-05


### Changed
- Reported version comes from muxcore.json (ADR-0021); built on core v0.6.12 / sdk/go/module v0.6.3 (mesh enrollment, ADR-0017).

## [0.2.0] - 2026-10-05

### Changed
- CI runs on GitHub-hosted runners from the umbrella template; retired-origin workflows removed.
- Dependencies resolve from published GitHub tags (no filesystem `replace`); requires core v0.6.0.

## [v0.2.0] — 2026-08-10

### Added
- `Skip` RPC: seek target when playback is inside intro/outro/credits/recap
- Offline golden tests on local sample media fixtures (`internal/testdata/samples`)

## [v0.1.0] — 2026-08-10

### Added
- `IntroOutroService` (Detect / Get / Set / Delete / List)
- Chapter + duration heuristic detection
- SettingsProvider (`intro_max_seconds`, `outro_max_seconds`)
- Health `:9711`
