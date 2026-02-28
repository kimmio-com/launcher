# Kimmio Launcher

Kimmio Launcher is a local Go app for creating and managing Docker-based Kimmio profiles (instances).

## Run

```bash
go run ./cmd/launcher
```

Open `http://localhost:7331` (or the fallback port written to `data/launcher-port`).

## Terminal Commands

```bash
go run ./cmd/launcher profile list
go run ./cmd/launcher profile <name> info
go run ./cmd/launcher profile <name> update [version]
go run ./cmd/launcher profile <name> delete
```

## Build

```bash
./build.sh
```

This creates distributable artifacts in `dist/` (apps, archives, binaries, `map.json`, `checksums.txt`).
