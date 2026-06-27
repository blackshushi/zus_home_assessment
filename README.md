# zus_home_assessment

Small Go service for a single-merchant F&B ordering flow. It exposes menu and order REST APIs, stores state in PostgreSQL, publishes `order.created` events to Kafka, and includes a worker that consumes those events and logs a structured order summary.

## Stack

- Go 1.22 standard `net/http`
- PostgreSQL with raw SQL through `pgx`
- Kafka through `segmentio/kafka-go`
- Docker Compose for PostgreSQL, Kafka, the server, and the worker

## API

- `GET /menu`
- `GET /menu/items/{id}`
- `PATCH /menu/items/{id}`
    - payload: {"availability": ['in_stock', 'out_of_stock']}
- `POST /orders`
- `GET /orders/{id}`
- `PATCH /orders/{id}/status`
    - payload: {"status": ["received", "preparing", "ready", "completed"]}

Order statuses must move in sequence: `received -> preparing -> ready -> completed`.

## Setup
---
### Run With Docker Compose

Compose starts PostgreSQL, Kafka, runs migration and seed jobs, then starts the server and Kafka worker.

```sh
docker compose up --build
```

The server is available at `http://localhost:8080`.

```sh
curl -s http://localhost:8080/menu
```

The worker runs in the same Compose stack and logs a structured `order summary` record whenever a new order is placed.

---
### Run Without Docker

```sh
cp .env.example .env
docker compose up -d postgres kafka
go mod download
go run ./cmd/migrate
go run ./cmd/seed
go run .
```

Run the Kafka worker in a second terminal:

```sh
go run ./cmd/worker
```


> Both will serve at http://localhost:8080 

### Test with Postman
You can import `Zus Home Assessment.postman_collection.json`(under root directory) into Postman desktop or web browser and test the APIs.

### Test with Terminal
You can test the api with `curl`
> For example: `curl http://localhost:8080/menu'

## Data Model

The schema has `categories`, `menu_items`, `orders`, and `order_items`. Order items snapshot the item name and unit price at purchase time so historical order totals stay stable even if the menu changes later. I used raw SQL with `pgx` because the model is small, the queries are straightforward, and explicit transaction boundaries are useful for order creation and status updates.

## Assessment Questions

### 1. API contract decisions

One non-obvious API decision is that `PATCH /menu/items/{id}` only accepts an `availability` field instead of being a general menu-item update endpoint. The requirement only asks operators to mark items in or out of stock, so a narrow contract reduces accidental changes to price, name, or category. The endpoint returns the full updated item so clients can refresh local state without issuing a follow-up `GET`.

### 2. Versioning

If a mobile client already consumed `GET /menu` and I needed to make a breaking response-shape change: 
I would keep the existing endpoint stable and add a versioned route such as `GET /v2/menu`. For mobile clients, URL versioning is easy to observe, document, and support during a gradual rollout. I would keep both versions active until telemetry showed old client usage had dropped to an acceptable level.

### 3. What you'd do differently with more time

With another two hours, I would add integration tests that run the API against PostgreSQL and Kafka using containers. The most important coverage would be unavailable items, total calculation, order event publishing, and invalid status transitions. That would test the behavior that matters most rather than only checking handler-level happy paths.

### 4. Production gap

The largest production gap is reliable event publishing around `POST /orders`. Today the order is committed first and then the Kafka publish is attempted, which can leave the system with an order but no async event if Kafka is unavailable. Before shipping to real users, I would add idempotency keys for order creation and a transactional outbox so events are stored with the order and retried safely.
