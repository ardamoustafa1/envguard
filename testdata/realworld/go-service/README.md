# ledger

A small HTTP service that records ledger entries per account. Go 1.23,
Postgres, [caarlos0/env](https://github.com/caarlos0/env) for configuration.

## Run it

```bash
cp .env.example .env
docker compose up -d db
make migrate
make run
```

## Configuration

Everything is configured through environment variables, parsed in
`internal/config`.

| Variable | Default | Description |
| --- | --- | --- |
| `APP_ENV` | `development` | `production` switches logs to JSON |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `JWT_SECRET` | - | **required**, bearer token for the API |
| `HTTP_ADDR` | `:8080` | listen address (`PORT` overrides the port) |
| `HTTP_READ_TIMEOUT` | `5s` | |
| `HTTP_WRITE_TIMEOUT` | `10s` | |
| `HTTP_SHUTDOWN_TIMEOUT` | `15s` | graceful shutdown deadline |
| `DB_URL` | - | **required**, Postgres connection string |
| `DB_MAX_CONNS` | `10` | pool size |
| `REDIS_ADDR` | `localhost:6379` | |
| `REDIS_TTL` | `5m` | |
| `APP_VERSION` | build version | optional override shown by `/healthz` |
| `TEST_DATABASE_URL` | - | enables the Postgres integration tests |

## Tests

```bash
make test
```
