<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="apps/web/public/seer-logo-white.svg">
    <img src="apps/web/public/seer-logo.svg" alt="" width="96">
  </picture>
</p>

# Seer

Highly available, self-hosted status pages and uptime monitoring.

Seer checks your endpoints (HTTP, TCP, DNS, ICMP) and serves a public status
page, plus private dashboards for your team. It's configured from one YAML
file, like [Gatus](https://github.com/TwiN/gatus), but every replica is
stateless, so you can run as many as you want:

- **Bring your own Postgres.** All state lives there; no SQLite, no local disk.
- **Checks are sharded across workers.** Kill one and its endpoints move to
  the others within ~15s. A check never runs twice for the same interval, and
  an alert never fires twice.
- **Public and private dashboards** from one deployment. Private ones sit
  under `/internal`, behind OIDC or an admin login.
- **Several status pages, one deployment.** Each site has its own hostnames,
  branding and dashboards; endpoints are shared and checked once.
- **Live updates** streamed to the browser, from any replica.
- **Long history without a time-series DB.** Rollups keep 2 years of hourly
  and unlimited daily history cheaply. Prometheus metrics are exported too.
- **Custom CSS** through a documented set of CSS variables and class names
  (the UI is Tailwind + shadcn/ui, themed from those variables).
- **Helm chart** with Gateway API routing.

## Quick start

```sh
cd deploy/compose
docker compose up -d --build
```

Open http://localhost:3000 for the public page and http://localhost:3000/internal
for the private one (user `admin`, password `admin`). The demo runs three Seer
replicas; `docker kill` one and the page keeps updating.

On Kubernetes:

```sh
helm install seer deploy/helm/seer \
  --set database.dsn='postgres://user:pass@db:5432/seer' \
  --set httpRoute.enabled=true \
  --set 'httpRoute.parentRefs[0].name=my-gateway' \
  --set 'httpRoute.hostnames[0]=status.example.com' \
  -f my-endpoints.yaml
```

## Configuration

```yaml
dashboards:
  - { key: status, title: Status, visibility: public, groups: [web, api] }
  - {
      key: ops,
      title: Operations,
      visibility: private,
      allowedGroups: [sre],
      groups: [web, api, infra],
    }

endpoints:
  - key: api
    group: api
    url: https://api.example.com/health
    interval: 30s
    conditions:
      - "[STATUS] == 200"
      - "[BODY].status == UP"
      - "[RESPONSE_TIME] < 500"
      - "[CERTIFICATE_EXPIRATION] > 7d"
```

See the [docs](https://seer.arnovannieuwenhuyzen.com/docs) for [every option](https://seer.arnovannieuwenhuyzen.com/docs/configuration),
[custom CSS](https://seer.arnovannieuwenhuyzen.com/docs/theming) and [how HA works](https://seer.arnovannieuwenhuyzen.com/docs/architecture). They're
built from `apps/docs`.

## Development

Needs Go 1.27+, Node 24+, pnpm, Docker, and `protoc-gen-go` /
`protoc-gen-connect-go` on your PATH.

```sh
pnpm install
pnpm gen                                     # protobuf (Go + TS) and ent code, after any schema change
cp apps/web/.env.example apps/web/.env.local # once
pnpm dev
```

`pnpm dev` starts Postgres (docker, :5433), applies migrations, seeds a year
of fake history for the `demo-api` endpoint, then runs the Go server
(`seer all` with `seer.example.yaml`), the Next.js dev server, the protos
watcher and the docs site together. Regenerate the demo history with
`cd apps/server && go run ./cmd/seed -force`; skip seeding with
`scripts/dev-db-up.sh --no-seed`. Dev ports avoid the usual 3000/8080/9090:

|                      |                                                |
| -------------------- | ---------------------------------------------- |
| Status page          | http://localhost:3100                          |
| Private dashboards   | http://localhost:3100/internal (admin / admin) |
| Docs                 | http://localhost:3200                          |
| Public / private API | :18080 / :18081                                |
| Health, metrics      | http://localhost:19090/metrics                 |

| Task                                      | Command                                      |
| ----------------------------------------- | -------------------------------------------- |
| Unit tests                                | `cd apps/server && go test ./...`            |
| All tests incl. Postgres (HA, visibility) | `scripts/test-integration.sh`                |
| New migration after editing `ent/schema`  | `pnpm gen && scripts/migrate-diff.sh <name>` |

Layout:

```
apps/server        Go: checks, scheduler, API (ConnectRPC), ent schema, migrations
apps/web           Next.js status pages
apps/docs          Documentation site (Fumadocs)
packages/protos    Protobuf API definitions, generated for Go and TypeScript
deploy/helm/seer   Helm chart
deploy/compose     docker compose HA demo
```

## Releases

`main` is production. Squash-merge feature branches with a
[Conventional Commits](https://www.conventionalcommits.org/) PR title (`feat:`,
`fix:`, `feat!:` …). [monotrack](https://github.com/ArnoldVanN/monotrack) then
keeps a release PR open with the next version and `CHANGELOG.md`; merging it
tags `vX.Y.Z` and publishes the images, the Helm chart
(`oci://ghcr.io/arnoldvann/charts/seer`) and a GitHub release. See
`.github/workflows/release.yml`.

## To do

- [ ] Notification providers (webhook, Discord, Slack, email); alerts only log for now
- [ ] Maintenance windows
- [ ] Announcements / incident notes on status pages
- [ ] Push endpoints (heartbeats from cron jobs) and status badges
- [ ] More checkers: SSH, STARTTLS, WebSocket, gRPC; HTTP client options (mTLS, proxy, DNS resolver)
- [ ] Per-dashboard description and default range, per-endpoint descriptions and links, footer options
- [ ] TimescaleDB mode
- [ ] MySQL support
- [ ] Config editor in the UI
- [ ] Tests: OIDC against a real provider, Gateway API on kind, Playwright, scale
- [ ] License and final module path
