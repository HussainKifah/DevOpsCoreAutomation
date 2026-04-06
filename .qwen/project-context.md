# DevOps Core Automation — Project Context

> Last reviewed: 2026-04-04

## What This Project Is

A Go-based web application for monitoring and managing **Nokia + Huawei OLT** (Optical Line Terminal) devices in telecommunications networks. Provides real-time monitoring, health checking, automated backups, role-based dashboards, and Slack/Elasticsearch integrations.

**Module**: `github.com/Flafl/DevOpsCore`
**Go version**: 1.24.4
**Entry point**: `cmd/api/main.go`

---

## Tech Stack

| Layer | Technology |
|---|---|
| Web Framework | Gin-gonic |
| Database | PostgreSQL + GORM |
| Auth | JWT (golang-jwt) + HTTP-only cookies |
| Scheduling | gocron v2 + robfig/cron |
| SSH | scrapligo (for OLT device communication) |
| WebSocket | gorilla/websocket |
| Frontend | Go HTML templates + Tailwind CSS + Alpine.js |
| Elasticsearch | go-elasticsearch v9 (syslog polling) |
| Slack | slack-go/slack (Events API + bot) |
| Excel Export | excelize |

---

## Project Structure

```
DevOpsCoreAutomation/
├── cmd/api/main.go              # Application entry point (server, graceful shutdown)
├── cmd/main.go                  # Standalone Nokia backup test (not used in main app)
├── config/config.go             # Config loader from env vars (.env support)
├── db/db.go                     # PostgreSQL connection + AutoMigrate + indexes
├── internal/
│   ├── Auth/                    # JWT manager + RBAC helpers
│   ├── crypto/creds.go          # AES-GCM encryption for stored credentials
│   ├── excessCommands/          # SSH command execution for OLT scans
│   │   ├── Nokia/               # Nokia-specific scan commands (power, health, desc, ports, backup, inventory)
│   │   └── Huawei/              # Huawei equivalents
│   ├── extractor/               # Regex-based parsers for OLT CLI output
│   │   ├── extractPower.go / extractPowerHuawei.go
│   │   ├── extractHealth.go / extractHealthHuawei.go
│   │   ├── extractInventory.go / extractInventoryHuawei.go
│   │   ├── extractPortProtectaion.go / extractProtectGroupHuawei.go
│   │   ├── extractDesc.go, extarctOnt-idx.go, cleanBackup.go
│   ├── handlers/                # HTTP handlers (API + page rendering)
│   ├── middleware/              # Auth middleware + role guards
│   ├── models/                  # GORM models (14 model files)
│   ├── nocpass/                 # NOC password rotation logic + SSH commands
│   ├── repository/              # Data access layer for all models
│   ├── router/router.go         # Gin route setup + middleware wiring
│   ├── scheduler/               # Background jobs
│   │   ├── scheduler.go         # Main OLT scan scheduler (power, health, ports, backup, inventory)
│   │   ├── WorkflowScheduler.go # IP-team workflow jobs (arbitrary commands/backups)
│   │   ├── noc_pass_rotator.go  # NOC credential rotation (15-min tick)
│   │   └── es_syslog_poller.go  # Elasticsearch syslog polling + dedup
│   ├── shell/                   # SSH connection pool + OLT device list + command dispatch
│   ├── syslog/                  # Slack syslog batching, reminders, message blocks
│   └── webSocket/               # WebSocket hub for live notifications
├── templates/                   # Go HTML templates + static assets (Tailwind, Alpine.js)
├── ticket-slack-reminder/       # Separate module/tool for Slack ticket reminders
├── utils/                       # Utility functions
├── docker-compose.yml           # Postgres + app services
├── Dockerfile                   # Multi-stage build (golang:1.24-alpine → alpine:3.21)
└── .env.example                 # Environment variable template
```

---

## Configuration (config.go)

Loaded via `godotenv` + environment variables. Key groups:

| Group | Variables |
|---|---|
| Database | `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE` |
| Server | `PORT`, `JWT_SECRET`, `TLS_CERT`, `TLS_KEY` |
| Nokia OLT | `OLT_SSH_USER`, `OLT_SSH_PASS` |
| Huawei OLT | `HW_SSH_USER`, `HW_SSH_PASS` (fallback to Nokia) |
| Elasticsearch | `ELASTICSEARCH_URL`, `ELASTICSEARCH_USER`, `ELASTICSEARCH_PASSWORD`, `ELASTICSEARCH_SKIP_TLS_VERIFY`, `ELASTICSEARCH_INDEX_PATTERN`, `ES_SYSLOG_POLL_INTERVAL`, `ES_SYSLOG_RETENTION_DAYS`, `ES_SYSLOG_DEDUP_WINDOW` |
| Slack | `SLACK_SYSLOG_ENABLED`, `SLACK_BOT_TOKEN`, `SLACK_CHANNEL_ID`, `SLACK_SIGNING_SECRET`, `SLACK_REMINDER_INTERVAL`, `SLACK_SYSLOG_BATCH_WINDOW`, `SLACK_SYSLOG_DISPLAY_OFFSET`, `SLACK_SYSLOG_TEAM_MENTION` |
| Scan Intervals | `POWER_SCAN_INTERVAL`(6h), `HEALTH_SCAN_INTERVAL`(0.5h), `DESC_SCAN_INTERVAL`(6h), `PORT_SCAN_INTERVAL`(0.5h), `BACKUP_INTERVAL`(24h) |

---

## OLT Device List (shell/OLTs.go)

OLTs are loaded from an external API (`OLTS_API_ENV` env var). Vendor classification:
- **Huawei**: IPs starting with `10.90.3.` or `10.250.0.178`, `10.202.160.3`, `10.80.2.161`
- **Nokia**: all others

Fallback Huawei IPs are hardcoded when API is unavailable.

---

## Data Models (internal/models/)

| Model | Table | Purpose |
|---|---|---|
| `User` | users | App users with bcrypt passwords + roles (viewer, admin, excess, noc, ip) |
| `PowerReading` | power_readings | ONT optical power (OLT Rx, ONT Rx) |
| `OntDescription` | ont_descriptions | ONT port descriptions/labels |
| `OltHealth` | olt_health | Current OLT health (CPU, temp, uptime) |
| `HealthSnapshot` | health_snapshots | Historical health snapshots |
| `PortProtectionRecord` | port_protection | Port protection/switchover state |
| `PortSnapshot` | port_snapshots | Historical port snapshots |
| `OltBackups` | olt_backups | Config backup file paths |
| `InventorySummary` | inventory_summaries | Aggregated ONT inventory counts |
| `OltInventory` | olt_inventories | Per-OLT inventory |
| `OntInventoryItem` | ont_inventory_items | Per-ONT inventory (model, serial) |
| `WorkflowDevice` | workflow_devices | Network devices for workflow jobs |
| `WorkflowJob` | workflow_jobs | Scheduled workflow jobs (backup/command) |
| `WorkflowRun` | workflow_runs | Execution records for workflow jobs |
| `WorkflowLog` | workflow_logs | Audit/event logs for workflows |
| `NocPassDevice` | noc_pass_devices | Devices with rotating NOC credentials |
| `EsSyslogFilter` | es_syslog_filters | Elasticsearch query filters for syslog |
| `EsSyslogAlert` | es_syslog_alerts | Deduplicated syslog alerts from ES |
| `EsSyslogSlackIncident` | es_syslog_slack_incidents | Slack incident tracking for alerts |

---

## Schedulers

### Main Scheduler (scheduler.go)
Built on gocron + cron. Runs periodic scans via SSH to OLTs:
- **power-scan** → optical power readings
- **desc-scan** → ONT descriptions
- **health-scan** → CPU, temperature, uptime (+ buffered history)
- **port-scan** → port protection down alerts
- **backup** → daily config backups (21:00)
- **inventory-scan** → monthly ONT inventory (1st of month, 02:00)
- **history-cleanup** → daily cleanup of old snapshots (01:00)

All jobs run for both Nokia and Huawei vendors. Semaphore ensures only one scan at a time.

### Workflow Scheduler (WorkflowScheduler.go)
Manages user-defined workflow jobs (arbitrary commands/backups) via gocron. Supports cron expressions and duration-based schedules. Credentials stored encrypted (AES-GCM).

### NOC Pass Rotator (noc_pass_rotator.go)
Ticks every 15 minutes, rotates NOC passwords on eligible devices (24h rotation cycle).

### ES Syslog Poller (es_syslog_poller.go)
Polls Elasticsearch for syslog messages matching user-defined filters. Deduplicates by fingerprint. Optionally batches alerts to Slack.

---

## API Routes (router.go)

### Public
- `GET /` → redirects to `/login`
- `GET /login` → login page
- `POST /api/auth/login` → login
- `POST /api/slack/events` → Slack Events API webhook

### Authenticated (any role)
- `POST /api/auth/logout`, `/refresh`, `GET /api/auth/me`

### Read-only (excess, admin, viewer)
- `GET /api/power/*`, `/api/descriptions/*`, `/api/health/*`, `/api/ports/*`, `/api/history/*`, `/api/inventory/*`, `/api/devices`

### Write APIs (excess, admin)
- `GET /api/backups/*`
- `POST /api/scan/{health,power,ports,inventory,backup}`
- `GET/POST/PUT/DELETE /api/admin/users/*`

### IP Team (ip, admin)
- Pages: `/workflows`, `/ip-backups`, `/ip-cmd-output`, `/ip-activity-log`, `/ip-syslog-alerts`
- APIs: `/api/workflows/*` (CRUD devices, jobs, runs, logs), `/api/ip/syslog/*` (alerts, filters)

### NOC Pass (noc, admin)
- Pages: `/noc-pass`
- APIs: `/api/noc-pass/*` (devices, credentials, rotation)

### WebSocket
- `GET /ws` → live notifications (JWT auth inside handler)

---

## Roles

| Role | Access |
|---|---|
| `viewer` | Read-only dashboards |
| `excess` | Read + scans + backups |
| `admin` | Full access + user management |
| `ip` | Workflow jobs, IP backups, syslog alerts |
| `noc` | NOC password management |

---

## Key Details to Remember

1. **SSH connection pooling** — `shell.NewConnectionPool` manages persistent SSH connections to OLTs
2. **Soft deletes** — some tables use soft-delete tombstones; `purgeVolatileSoftDeleted()` cleans them
3. **Health buffer** — health snapshots are buffered and averaged before history insert; `FlushHealthBuffer()` on shutdown
4. **Dedup** — syslog alerts are fingerprinted (SHA256 of host+device+message) with configurable dedup window
5. **Encryption** — workflow and NOC pass credentials encrypted with AES-GCM using JWT secret as key
6. **Vendor support** — Nokia (primary) + Huawei (secondary, falling back to Nokia commands where needed)
7. **Graceful shutdown** — stops schedulers, flushes buffers, drains SSH pool, then shuts down HTTP server
8. **Huawei power scanning** — scans ALL slots (1–15, or 0–15 for Hafriya) × 16 PONs × 128 ONTs per PON. Default chunk=128 means 1 CLI command per PON. Per-command timeout is 30s (not 5min). Tuned via `HW_POWER_*` and `HW_SSH_*` env vars.
