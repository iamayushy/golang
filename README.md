# sortly

Short links with a short life. Every link self-destructs 10 to 30 minutes after it is made.

- **API** (repo root): Go, standard library `net/http`, MongoDB storage. A TTL index on
  `expires_at` makes MongoDB delete expired links on its own.
- **Web** (`web/`): a static frontend (plain HTML/CSS/JS, no build step) served by nginx.

## Environment variables

### API (root `Dockerfile`)

| Variable             | Required | Default                   | Description |
|----------------------|----------|---------------------------|-------------|
| `MONGO_URI`          | yes      | —                         | MongoDB connection string, e.g. `mongodb://localhost:27017` or an Atlas `mongodb+srv://...` URI |
| `MONGO_DB`           | no       | `urlshortener`            | Database name (links live in the `links` collection) |
| `PORT`               | no       | `8080`                    | Port the HTTP server listens on |
| `BASE_URL`           | no       | `http://localhost:$PORT`  | Public URL of the API, used to build `short_url` |
| `FRONTEND_URL`       | no       | —                         | Public URL of the web app. When set, `/` and expired short links redirect there |
| `ALLOWED_ORIGINS`    | no       | `FRONTEND_URL`, else `*`  | Comma-separated origins allowed to call the API from a browser (CORS), e.g. `https://sort.atlair.app`. Must match the browser's origin exactly: scheme, host and port, no path |
| `RATE_LIMIT_PER_MIN` | no       | `20`                      | Links one IP can create per minute; `0` turns the limit off |

### Web (`web/Dockerfile`)

| Variable  | Required | Default                 | Description |
|-----------|----------|-------------------------|-------------|
| `API_URL` | yes      | `http://localhost:8080` | Public URL of the API the browser calls |
| `PORT`    | no       | `8080`                  | Port nginx listens on |

See `.env.example`.

## API

| Method   | Path                | Description                              |
|----------|---------------------|------------------------------------------|
| `POST`   | `/api/links`        | Create a short link: `{"url": "https://...", "ttl_minutes": 20}` (10–30, default 10). The response includes a one-time `delete_token` |
| `GET`    | `/api/links/{code}` | Link details and click count             |
| `GET`    | `/api/stats`        | Site-wide counts: `{"total_created": 4312, "live": 27}` |
| `DELETE` | `/api/links/{code}` | Delete a link early; needs header `X-Delete-Token` |
| `GET`    | `/{code}`           | Redirect (302) to the original URL, counts a click |
| `GET`    | `/`                 | Service info (200), or a redirect to `FRONTEND_URL` when set |
| `GET`    | `/healthz`          | `200` if MongoDB is reachable, else `503` |

```sh
curl -X POST localhost:8080/api/links -d '{"url":"https://go.dev/doc","ttl_minutes":15}'
# {"code":"aZ3kP9","url":"https://go.dev/doc","clicks":0,"created_at":"...","expires_at":"...",
#  "short_url":"http://localhost:8080/aZ3kP9","expires_in_seconds":900,"delete_token":"..."}

curl -i localhost:8080/aZ3kP9        # 302 Location: https://go.dev/doc
curl localhost:8080/api/links/aZ3kP9 # clicks: 1
```

## Running

```sh
# Everything in Docker: web on :3000, API on :8080, MongoDB
docker compose up --build

# Or locally against your own MongoDB
MONGO_URI=mongodb://localhost:27017 make run

make test   # unit tests (in-memory repository, no MongoDB needed)
```

## Deploying as two services

1. **API**: build from the repo root. Set `MONGO_URI`, `BASE_URL` (e.g. `https://sortly.atlair.app`)
   and `FRONTEND_URL` (the web app's URL).
2. **Web**: build from `web/`. Set `API_URL` to the API's public URL.

## Layout

```
cmd/server/          entrypoint: config, Mongo connection, graceful shutdown
internal/config/     env-based config
internal/shortener/  domain: Link, validation, code generation, Repository interface
internal/mongostore/ MongoDB Repository (unique index on code, atomic click $inc)
internal/server/     HTTP routes; logging, panic recovery, CORS and rate-limit middleware
web/                 frontend (index.html, style.css, app.js) + nginx Dockerfile
```
