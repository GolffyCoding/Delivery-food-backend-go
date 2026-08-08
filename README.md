# Delivery-food-backend-go

A backend service for a food delivery platform, written in Go. It uses [Fiber v3](https://github.com/gofiber/fiber) as the web framework, PostgreSQL (via [Bun ORM](https://github.com/uptrace/bun)) as the primary database, Redis for caching/locking/idempotency, and NATS for the event bus / outbox pattern.

## Features

The project is organized into business domain modules (`internal/`):

- **auth** — registration, login, JWT access/refresh tokens
- **order** — order creation and management
- **restaurant** — restaurant data
- **menu** — restaurant menu items
- **driver** — driver data and real-time location caching
- **payment** — payments across multiple providers
- **wallet** — user wallet
- **coupon** — coupons/discounts
- **review** — restaurant/driver reviews
- **support** — customer support tickets
- **notification** — user notifications (multi-channel senders)
- **tracking** — delivery status tracking and anomaly detection
- **websocket** — realtime hub for clients
- **admin** — admin-facing endpoints

And shared packages (`pkg/`):

- **eventbus** — NATS-based pub/sub event bus
- **outbox** — transactional outbox pattern
- **lock** — Redis-based distributed lock
- **middleware** — common middleware (e.g. idempotency)
- **response** — shared response format
- **realtime** — realtime messaging helpers

## Project Structure

```
cmd/
  server/     API server entry point
  worker/     background worker entry point (processes outbox/events)
configs/      environment-based configuration loading
database/     postgres/redis connections and schema
internal/     business domain modules
pkg/          shared packages
```

## Requirements

- Go 1.25+
- PostgreSQL 16
- Redis 7
- NATS (optional, enable with `NATS_ENABLED=true`)
- Docker & Docker Compose (for containerized setup)

## Environment Setup

Copy the example env file and adjust values for your environment:

```bash
cp .env.example .env
```

Key variables include `SERVER_PORT`, `DB_*`, `REDIS_*`, `JWT_*`, and `NATS_*` (see `.env.example` for the full list).

## Running with Docker Compose

Run all services (postgres, redis, nats, backend, worker) together:

```bash
docker compose up --build
```

The API server will be available at `http://localhost:8080`.

## Running Locally (without Docker)

1. Have PostgreSQL and Redis running, and configure `.env` accordingly.
2. Install dependencies:

   ```bash
   go mod download
   ```

3. Run the API server:

   ```bash
   go run ./cmd/server
   ```

4. Run the background worker (separate process):

   ```bash
   go run ./cmd/worker
   ```

## Build

```bash
go build -o bin/server ./cmd/server
go build -o bin/worker ./cmd/worker
```

## License

No license has been specified for this project yet.
