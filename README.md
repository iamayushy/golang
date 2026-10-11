# golang

A minimal Go HTTP server using only the standard library. No database — state lives in memory.

## Layout

```
cmd/server/        entrypoint (graceful shutdown, timeouts)
internal/config/   env-based config (PORT, default 8080)
internal/server/   routes, JSON helpers, logging + panic-recovery middleware
internal/store/    generic thread-safe in-memory store
```

## Usage

```sh
make run     # start on :8080
make test    # run tests with the race detector
curl localhost:8080/healthz
```
