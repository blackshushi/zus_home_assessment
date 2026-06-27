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

---

> **Both will serve API endpoints at `http://localhost:8080`**

### Test with Postman

You can import `Zus Home Assessment.postman_collection.json`(under root directory) into Postman desktop or web browser (Refer to this [Documentation](https://learning.postman.com/docs/getting-started/importing-and-exporting/importing-and-exporting-overview)) and test the APIs.

### Test with Terminal

You can test the api with `curl`

> For example: `curl [http://localhost:8080/menu](http://localhost:8080/menu)'

## Data Model

I used raw SQL with `pgx` because the model is small, the queries are straightforward, and explicit transaction boundaries are useful for order creation and status updates.

### Tables

- Categories
Stores menu categories like Coffee, Food, etc. Controls display order and whether a category is active.
Key fields: id, name, sort_order, active
- Menu_items
Stores individual menu items under each category (e.g. Latte, Croissant). Includes pricing and availability status.
Key fields: id, category_id, name, price_cents, availability, active
- Orders
Stores customer orders with status tracking and total pricing.
Key fields: id, status, subtotal_cents, total_cents, currency
- Order_items
Stores items inside each order (line items with snapshot of product data).
Key fields: id, order_id, menu_item_id, name, unit_price_cents, quantity, line_total_cents

---

## Assessment Questions

### 1. API contract decisions

> One non-obvious API decision is that `PATCH /menu/items/{id}` only accepts an `availability` field instead of being a general menu-item update endpoint. Since the assessment required to be done between 3-4 hours, there's no more rooms for me to explore the complexity for this endpoint. The endpoint returns the full updated item so clients can refresh local state without issuing a follow-up `GET`.

### 2. Versioning

If a mobile client already consumed `GET /menu` and I needed to make a breaking response-shape change: 
> I would keep the existing endpoint stable and add a versioned route such as `GET /v2/menu`. For mobile clients, URL versioning is easy to observe, document, and support during a gradual rollout. I would keep both versions active until telemetry showed old client usage had dropped to an acceptable level.

### 3. What I would do differently with more time

With an additional two hours, I would focus on two key improvements:

>
>  - **Add integration tests**  
>    The most important coverage would include unavailable items, total calculation, order event publishing, and invalid status transitions. This would ensure the system behavior is correctly validated end-to-end, rather than relying only on handler-level happy path tests.

>  - **Improve Kafka event publishing flow**  
>    I would extend the Kafka publisher to handle order status updates as well, so that any change in order state (e.g. preparing → ready → completed) would emit corresponding events. This would make the event system more complete and better suited for downstream consumers.

### 4. Production gap

> The largest production gap is reliable event publishing around `POST /orders`. Today the order is committed first and then the Kafka publish is attempted, which can leave the system with an order but no async event if Kafka is unavailable. Before shipping to real users, I would add idempotency keys for order creation and a transactional outbox so events are stored with the order and retried safely.