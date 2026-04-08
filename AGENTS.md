# AGENTS.md

## Mission

DevOps Core Automation is a Go web platform for telecom operations automation. It monitors Nokia and Huawei OLTs, stores scan history, manages backups, handles IP team workflows, rotates NOC passwords, polls Elasticsearch syslog alerts, and forwards selected Slack/Ruijie alarm workflows to the right teams.

Use this file as the fast-start guide before changing code.

## Fast Start

1. Copy `.env.example` to `.env` and fill the required secrets.
2. Start Postgres:

```bash
docker compose up -d db
```

3. Run the API server:

```bash
go run ./cmd/api
```

4. Open:

```text
http://localhost:8080
```

Full Docker run:

```bash
docker compose up --build
```

Notes:
- The web app entrypoint is `cmd/api/main.go`.
- Database migrations run on startup in `db/db.go`.
- `cmd/main.go`, when present, is a standalone device/backup utility, not the web app.
- Local Go may be installed at `/usr/local/go/bin/go` in this workspace.

## Core Environment

Required for the app:
- `DB_PASSWORD`
- `JWT_SECRET`
- `OLT_SSH_USER`
- `OLT_SSH_PASS`

Common optional values:
- `HW_SSH_USER`, `HW_SSH_PASS`: Huawei credentials, falling back to Nokia credentials when empty.
- `TLS_CERT`, `TLS_KEY`: enable HTTPS.
- `POWER_SCAN_INTERVAL`, `HEALTH_SCAN_INTERVAL`, `DESC_SCAN_INTERVAL`, `PORT_SCAN_INTERVAL`, `BACKUP_INTERVAL`.
- Elasticsearch syslog polling settings: `ELASTICSEARCH_URL`, credentials, index pattern, poll interval, retention, dedup window.
- Slack settings: `SLACK_BOT_TOKEN`, `SLACK_SIGNING_SECRET`, syslog channel, ticket channel, team mentions, reminder intervals.
- Ruijie mail alarm settings: Microsoft Graph tenant/client/user/folder settings plus `RUIJIE_SLACK_CHANNEL_ID`.

Keep secrets in `.env`; do not commit live credentials.

## Architecture Map

- `cmd/api/main.go`: app wiring for config, DB, repositories, handlers, schedulers, Slack workers, Ruijie mail polling, and HTTP server lifecycle.
- `config/config.go`: environment loading, defaults, feature enablement checks.
- `db/db.go`: PostgreSQL connection, `AutoMigrate`, and index creation.
- `internal/router/router.go`: page/API routes and role guards.
- `internal/handlers/`: HTTP handlers and Slack Events handling.
- `internal/repository/`: GORM data access layer.
- `internal/models/`: GORM models and migration surface.
- `internal/scheduler/`: recurring scans, backups, cleanup, inventory, syslog polling, workflows, and NOC rotation.
- `internal/shell/`: SSH/vendor command execution.
- `internal/extractor/`: vendor command output parsing.
- `internal/Ruijie/`: Microsoft Graph Junk Email polling for Ruijie Cloud alarm messages and Slack reminder posting.
- `internal/SlackReminders/`: Slack ticket/thread reminder helpers and worker.
- `internal/syslog/`: Elasticsearch syslog client, Slack syslog batching, Slack reminder text/signature helpers.
- `templates/`: HTML templates and static assets.

## Role Model

Web/API access is gated by JWT middleware and explicit role guards.

Roles currently used:
- `admin`: user admin plus all operational areas.
- `excess`: Nokia/Huawei monitoring, backups, scans.
- `viewer`: read-only monitoring views.
- `ip`: IP team workflows and syslog alerts.
- `noc`: NOC password utilities.

When adding a route, check both page routes and API routes. Keep authorization explicit in `internal/router/router.go`.

## High-Impact Areas

1. Scheduler changes can increase production polling load and device command volume.
2. Shell/extractor changes can break parsing across Nokia/Huawei firmware variants.
3. Model/repository changes affect migrations, retention, and historical data.
4. Router/middleware changes can accidentally open or block access.
5. Slack/Ruijie/syslog reminder changes affect live operational alerting and escalation noise.

## Dev Commands

```bash
# format changed Go files
gofmt -w path/to/file.go

# full test sweep, when the workspace is healthy
go test ./...

# package-focused compile check
go test ./cmd/api ./config ./db ./internal/handlers ./internal/Ruijie ./internal/repository ./internal/models

# fast search
rg "pattern" internal
```

If `go` or `gofmt` is not on `PATH`, check `/usr/local/go/bin/go` and `/usr/local/go/bin/gofmt`.

## Working Rules

1. Prefer small, scoped patches and follow existing package patterns.
2. Do not revert unrelated user changes in a dirty worktree.
3. Keep scheduler singleton behavior and scan semaphore logic intact.
4. Avoid ad hoc parsing when an existing extractor/helper already handles the format.
5. Keep Slack reminder behavior consistent: post in thread, mention the configured team, and stop reminders on configured resolution reactions where supported.
6. Never add live credentials, `.env` values, tokens, Graph client secrets, Slack secrets, or device passwords to tracked files.
7. Validate with `gofmt` and a focused `go test` when feasible; call out any pre-existing compile failures or hung integration tests.

## First Files To Open

1. `cmd/api/main.go`
2. `config/config.go`
3. `internal/router/router.go`
4. The relevant handler in `internal/handlers/`
5. Matching repository/model files in `internal/repository/` and `internal/models/`
6. `internal/scheduler/`, `internal/shell/`, or `internal/extractor/` if scans/device commands are involved
7. `internal/Ruijie/`, `internal/syslog/`, or `internal/SlackReminders/` if Slack, syslog, email alarms, or reminders are involved
