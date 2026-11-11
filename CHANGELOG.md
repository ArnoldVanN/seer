# Changelog

## [seer v0.1.3] - 2026-11-10

### Bug Fixes

- **deps:** patch vulnerable dev dependencies (a1aa801)

### Other

- **protos:** commit generated protobuf and ent code (4e353da)

## [seer v0.1.2] - 2026-11-09

### Features

- custom domain check (9398687)
- per-site alert targets through the admin API (56f6a77)
- admin API and DB-backed sites, dashboards and endpoints (e4d8814)
- add apache 2.0 license (03b3a4a)

### Other

- rename module path (3a98375)

## [seer v0.1.1] - 2026-11-03

### Features

- push endpoint (0d498b7)

## [seer v0.1.0] - 2026-11-03

### ⚠ Breaking Changes

- harden login redirects, public check errors, secrets and SMTP TLS (5a5ba88)

### Features

- keep sessions in Postgres (ff569a1)
- **web:** add status and uptime SVG badges for public endpoints (b7003fa)

## [seer v0.0.6] - 2026-10-29

### Features

- **server:** show failed conditions, URL and downtime in alerts (2fdac7a)

## [seer v0.0.5] - 2026-10-29

### Features

- **server:** send alerts to webhook, Slack, Discord and email from a Postgres queue (0b51007)
- dashboard/endpoint descriptions, default ranges, footer options and consistent history bars (52a60d8)

### Bug Fixes

- **web:** add SEER_FORCE_HTTPS for proxies that don't forward the https scheme (512fed2)

## [seer v0.0.4] - 2026-10-29

### Bug Fixes

- **web:** run as numeric uid (bece6af)

## [seer v0.0.3] - 2026-10-29

### Bug Fixes

- **chart:** sanitize build metadata in helm.sh/chart label (b642ee2)

## [seer v0.0.2] - 2026-10-29

### Bug Fixes

- **chart:** duplicate instance label on deployments (e800202)
- **server:** slow build (2d0b13b)

### Other

- **docs:** update development page (490a4a0)

## [seer v0.0.1] - 2026-10-29

### Other

- Initial release

