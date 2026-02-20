# Kimmio Launcher

Kimmio Launcher is a local Go app for creating and managing Docker-based Kimmio profiles (instances).

## Run

```bash
go run ./cmd/luncher
```

Open `http://localhost:7331` (or the fallback port written to `data/launcher-port`).

## Build

```bash
./build.sh
```

This creates distributable artifacts in `dist/` (apps, archives, binaries, `map.json`, `checksums.txt`).
