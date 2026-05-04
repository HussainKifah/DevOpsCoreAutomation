# DevOps Core Automation

DevOps Core Automation is a Go web platform for telecom operations. It monitors Nokia and Huawei OLTs, stores scan history, manages backups, runs IP team workflows, rotates NOC passwords, polls Elasticsearch syslog alerts, and sends Slack reminders for operational alarms.

The main application entrypoint is `cmd/api/main.go`.

## What It Does

- Nokia and Huawei OLT monitoring for power, health, descriptions, port state, inventory, and backups.
- Historical storage in PostgreSQL through GORM models and auto-migrations.
- Role-based web UI and API access for `admin`, `excess`, `viewer`, `ip`, and `noc` users.
- IP team workflow jobs with encrypted device credentials and run output history.
- NOC password device management and rotation.
- Elasticsearch syslog alert polling with deduplication, Slack batching, reminders, and checkmark resolution.
- Slack latest request/incident reminders for structured channel messages.
- Ruijie Cloud alarm forwarding from Microsoft 365/Outlook Junk Email to Slack with reminder tracking.

## Tech Stack

- Go 1.24
- Gin HTTP router
- PostgreSQL 16
- GORM
- Go HTML templates with static assets under `templates/`
- Slack API through `github.com/slack-go/slack`
- Elasticsearch Go client
- Microsoft Graph REST calls for Ruijie mail polling
- Docker and Docker Compose for local/full deployment

## Project Layout

```text
cmd/api/                 Main web app entrypoint
config/                  Environment loading and feature flags
db/                      Database connection and migrations
internal/Auth/           JWT auth
internal/handlers/       Page/API handlers and Slack Events endpoint
internal/middleware/     Auth and role guards
internal/models/         GORM models
internal/repository/     Database access layer
internal/router/         Routes and authorization
internal/scheduler/      Background scan, backup, cleanup, syslog, workflow, and NOC jobs
internal/shell/          SSH connection and command execution
internal/extractor/      Vendor output parsing
internal/syslog/         Elasticsearch syslog polling helpers and Slack formatting
internal/Ruijie/         Ruijie Cloud mail polling and Slack reminders
templates/               HTML templates and static files
utils/                   Shared utilities
```

## Quick Start

1. Create your environment file:

```bash
cp .env.example .env
```

2. Fill the required values in `.env`:

```bash
DB_PASSWORD=changeme
JWT_SECRET=replace_me_with_a_long_random_secret
OLT_SSH_USER=your_nokia_user
OLT_SSH_PASS=your_nokia_password
```

3. Start Postgres:

```bash
docker compose up -d db
```

4. Run the app:

```bash
go run ./cmd/api
```

5. Open:

```text
http://localhost:8080
```

If `go` is not on `PATH` in the local workspace, try:

```bash
/usr/local/go/bin/go run ./cmd/api
```

## Full Docker Run

```bash
docker compose up --build
```

Compose starts:
- `db`: PostgreSQL 16 with the `devopscore` database.
- `app`: the Go web app, exposed on host ports `80` and `443`, with app `PORT=8080` inside the container.

Persistent volumes:
- `pgdata`: PostgreSQL data.
- `backups`: generated backup files.

## Configuration

The app loads `.env` with `godotenv` and falls back to process environment variables.

Core variables:

```bash
DB_HOST=localhost
DB_PORT=5432
DB_USER=devopscore
DB_PASSWORD=changeme
DB_NAME=devopscore
DB_SSLMODE=disable
PORT=8080
JWT_SECRET=replace_me
OLT_SSH_USER=
OLT_SSH_PASS=
```

Huawei credentials are optional and fall back to Nokia credentials:

```bash
HW_SSH_USER=
HW_SSH_PASS=
```

Scan intervals are optional:

```bash
POWER_SCAN_INTERVAL=6h
HEALTH_SCAN_INTERVAL=0.5h
DESC_SCAN_INTERVAL=6h
PORT_SCAN_INTERVAL=0.5h
BACKUP_INTERVAL=24h
```

HTTPS is enabled only when both files are set:

```bash
TLS_CERT=/app/certs/server.crt
TLS_KEY=/app/certs/server.key
```

See `.env.example` for the full list.

## Slack And Syslog

Elasticsearch syslog polling is enabled by setting the Elasticsearch URL and related credentials. Alerts are deduplicated by host, device, and normalized message within the configured dedup window.

Slack syslog posting requires:

```bash
SLACK_SYSLOG_ENABLED=true
SLACK_BOT_TOKEN=xoxb-...
SLACK_CHANNEL_ID=C0123456789
SLACK_SIGNING_SECRET=...
SLACK_REMINDER_INTERVAL=6h
SLACK_SYSLOG_BATCH_WINDOW=45s
SLACK_SYSLOG_TEAM_MENTION=<!subteam^S01234567|ip-core>
```

Slack Events should point to:

```text
POST https://your-host/api/slack/events
```

The Events endpoint is used to stop syslog and Ruijie reminders when a supported checkmark reaction is added.

Useful Slack scopes/events:
- Scopes: `chat:write`, `channels:history`, `reactions:read`, `users:read`
- Events: `reaction_added`
- Add private-channel equivalents when the channel is private.

## Ruijie Mail Alarms

Ruijie Cloud alarm forwarding reads Microsoft 365/Outlook mail through Microsoft Graph and defaults to the Junk Email folder (`junkemail`). New matching messages are stored by Graph message ID before Slack delivery, so repeat polls do not post duplicates.

Required settings:

```bash
RUIJIE_MAIL_ENABLED=true
RUIJIE_MAIL_TENANT_ID=00000000-0000-0000-0000-000000000000
RUIJIE_MAIL_CLIENT_ID=00000000-0000-0000-0000-000000000000
RUIJIE_MAIL_CLIENT_SECRET=...
RUIJIE_MAIL_USER_ID=user@example.com
RUIJIE_MAIL_FOLDER_ID=junkemail
RUIJIE_MAIL_SUBJECT=Ruijie Cloud Alarm Notification
RUIJIE_MAIL_POLL_INTERVAL=1m
RUIJIE_MAIL_LOOKBACK=10m
RUIJIE_SLACK_CHANNEL_ID=C0123456789
RUIJIE_SLACK_TEAM_MENTION=<!subteam^S01234567|ip-team>
RUIJIE_SLACK_REMINDER_INTERVAL=6h
RUIJIE_SLACK_DISPLAY_OFFSET=3h
```

The Azure app should have Microsoft Graph application permission `Mail.Read` with admin consent for the mailbox being monitored.

## Routes And Roles

Public:
- `GET /login`
- `POST /api/auth/login`
- `POST /api/slack/events` when Slack Events are configured

General monitoring roles:
- Pages: `/dashboard`, `/devices`, `/alerts`
- API: `/api/devices`, `/api/power/*`, `/api/descriptions/*`, `/api/health/*`, `/api/ports/*`, `/api/history/*`, `/api/inventory/*`
- Roles: `excess`, `admin`, `viewer`

Write operations:
- Backups and manual scans
- Roles: `excess`, `admin`

Admin:
- User management
- Role: `admin`

IP team:
- Pages: `/workflows`, `/ip-backups`, `/ip-cmd-output`, `/ip-activity-log`, `/ip-syslog-alerts`
- APIs: `/api/workflows/*`, `/api/ip/syslog/*`
- Roles: `ip`, `admin`

NOC:
- Page/API: `/noc-pass`, `/api/noc-pass/*`
- Roles: `noc`, `admin`

## Development

Format changed Go files:

```bash
gofmt -w path/to/file.go
```

Run tests:

```bash
go test ./...
```

Run a focused package check:

```bash
go test ./cmd/api ./config ./db ./internal/handlers ./internal/Ruijie ./internal/repository ./internal/models
```

Search quickly:

```bash
rg "pattern" internal
```

## Operational Notes

- Auto-migrations run on startup in `db/db.go`.
- Scheduler changes can affect production device command volume.
- Route changes should be checked for both page and API role guards.
- Model/repository changes affect migrations and retention behavior.
- Shell/extractor changes can break vendor-specific parsing.
- Do not commit `.env`, Graph client secrets, Slack tokens, JWT secrets, or device passwords.
