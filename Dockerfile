FROM golang:1.26-alpine AS build

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN go build -o /out/server . && \
    go build -o /out/worker ./cmd/worker && \
    go build -o /out/migrate ./cmd/migrate && \
    go build -o /out/seed ./cmd/seed

FROM alpine:3.20

RUN adduser -D -H appuser
WORKDIR /app

COPY --from=build /out/server /usr/local/bin/server
COPY --from=build /out/worker /usr/local/bin/worker
COPY --from=build /out/migrate /usr/local/bin/migrate
COPY --from=build /out/seed /usr/local/bin/seed
COPY migrations ./migrations
COPY seeds ./seeds

USER appuser

CMD ["server"]
