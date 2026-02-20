# Kimmio Launcher

Kimmio Launcher is a Go web application used to create and manage Docker-based Kimmio instances (called profiles).

## What it does

- Creates Kimmio profiles with isolated Docker Compose stacks.
- Starts, stops, recreates, deletes, and updates profile versions.
- Tracks action progress with async jobs.
- Checks runtime health from each profile's `/health` endpoint.
- Stores profile metadata and secrets locally.

## Stack

- Go 1.22
- Standard `net/http` server
- Embedded HTML templates and static assets
- Docker Compose for instance orchestration

## Project layout

- `cmd/luncher/main.go`: app entrypoint + embedded UI assets
- `internal/config/`: env-driven config defaults (port, data dir, limits, timeouts)
- `internal/launcher/`: HTTP routes, jobs, docker orchestration, storage, templates
- `cmd/luncher/templates/`: UI templates
- `cmd/luncher/static/`: CSS/images/icons
- `scripts/`: local build/packaging scripts
- `data/`: runtime data (profiles, compose files, secrets, launcher port)

## Runtime data

The launcher writes runtime state to `data/`:

- `data/profiles.json`: persisted profile metadata
- `data/compose/<profile-id>/`: generated compose files per profile
- `data/secrets/<profile-id>.env`: secret env vars
- `data/launcher-port`: active launcher port

## Configuration

Config is centralized in `internal/config` and can be overridden with env vars:

- `KIMMIO_PORT` (default: `7331`)
- `KIMMIO_PORT_SEARCH_RANGE` (default: `100`)
- `KIMMIO_DATA_DIR` (default: `data` in dev, user config dir in prod)
- `KIMMIO_MAX_PROFILES` (default: `3`)
- `KIMMIO_ACTION_TIMEOUT` (default: `2m`)
- `KIMMIO_PROFILE_PORT_MIN` (default: `8080`)
- `KIMMIO_PROFILE_PORT_MAX` (default: `9000`)

## Secrets and rotation

- Launcher no longer persists placeholder/default secrets at profile creation time.
- If profile secrets are not set, runtime values are generated for compose execution.
- Use the **Regenerate secrets** action in the UI to rotate `JWT_SECRET` and `FLUMIO_ENC_KEY_V0`.
- Rotating secrets invalidates existing sessions/tokens.

## Windows installer

- CI builds a one-click Windows installer: `Kimmio-Launcher-Setup-windows-amd64.exe`.
- Installer includes a post-install launch step, so after setup finishes it can start Kimmio Launcher automatically.

## Build metadata and checksums

- Builds are version-stamped from Git tag (`vX.Y.Z`) or fallback to `0.0.0-<short-commit>`.
- Go binaries receive version metadata via linker flags (`main.appVersion`, `main.gitCommit`).
- `dist/map.json` includes `version` and `commit`.
- `dist/checksums.txt` is generated with SHA-256 hashes for release/download files.

## Run locally

```bash
go run ./cmd/luncher
```

Open `http://localhost:7331` (or fallback port written in `data/launcher-port`).

## Test

```bash
go test ./...
```
