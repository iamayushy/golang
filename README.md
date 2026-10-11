# URL Shortener

A minimal URL shortener API in Go (standard library `net/http`) backed by MongoDB.

## Environment variables

| Variable    | Required | Default                   | Description                                         |
|-------------|----------|---------------------------|-----------------------------------------------------|
| `MONGO_URI` | yes      | —                         | MongoDB connection string, e.g. `mongodb://localhost:27017` or an Atlas `mongodb+srv://...` URI |
| `MONGO_DB`  | no       | `urlshortener`            | Database name (links live in the `links` collection) |
| `PORT`      | no       | `8080`                    | Port the HTTP server listens on                      |
| `BASE_URL`  | no       | `http://localhost:$PORT`  | Public base URL used to build `short_url`            |

See `.env.example`.

## API

| Method   | Path                | Description                              |
|----------|---------------------|------------------------------------------|
| `POST`   | `/api/links`        | Create a short link: `{"url": "https://..."}` |
| `GET`    | `/api/links/{code}` | Link details and click count             |
| `DELETE` | `/api/links/{code}` | Delete a link                            |
| `GET`    | `/{code}`           | Redirect (302) to the original URL, counts a click |
| `GET`    | `/healthz`          | `200` if MongoDB is reachable, else `503` |

```sh
curl -X POST localhost:8080/api/links -d '{"url":"https://go.dev/doc"}'
# {"code":"aZ3kP9q","url":"https://go.dev/doc","clicks":0,"created_at":"...","short_url":"http://localhost:8080/aZ3kP9q"}

curl -i localhost:8080/aZ3kP9q        # 302 Location: https://go.dev/doc
curl localhost:8080/api/links/aZ3kP9q # clicks: 1
```

## Running

```sh
# Everything in Docker (API + MongoDB)
docker compose up --build

# Or locally against your own MongoDB
MONGO_URI=mongodb://localhost:27017 make run

make test   # unit tests (in-memory repository, no MongoDB needed)
```

## Layout

```
cmd/server/          entrypoint: config, Mongo connection, graceful shutdown
internal/config/     env-based config
internal/shortener/  domain: Link, validation, code generation, Repository interface
internal/mongostore/ MongoDB Repository (unique index on code, atomic click $inc)
internal/server/     HTTP routes, JSON helpers, logging + panic-recovery middleware
```
