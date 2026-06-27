package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alex/zus_home_assessment/internal/config"
	"github.com/alex/zus_home_assessment/internal/models"
	"github.com/segmentio/kafka-go"
)

type orderCreatedEvent struct {
	EventType  string       `json:"eventType"`
	OccurredAt time.Time    `json:"occurredAt"`
	Order      models.Order `json:"order"`
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: cfg.KafkaBrokers,
		Topic:   cfg.KafkaOrderTopic,
		GroupID: cfg.KafkaConsumerGroup,
	})
	defer reader.Close()

	logger.Info("worker listening", "topic", cfg.KafkaOrderTopic, "group", cfg.KafkaConsumerGroup)

	for {
		message, err := reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Error("failed to read kafka message", "error", err)
			continue
		}

		var event orderCreatedEvent
		if err := json.Unmarshal(message.Value, &event); err != nil {
			logger.Error("failed to decode order event", "error", err, "key", string(message.Key))
			continue
		}

		logger.Info("Order had been created! Summary:",
			"eventType", event.EventType,
			"orderId", event.Order.ID,
			"status", event.Order.Status,
			"itemCount", len(event.Order.Items),
			"totalCents", event.Order.TotalCents,
			"currency", event.Order.Currency,
			"occurredAt", event.OccurredAt,
		)
	}
}
