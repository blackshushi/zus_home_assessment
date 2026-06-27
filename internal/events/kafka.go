package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/alex/zus_home_assessment/internal/models"
	"github.com/segmentio/kafka-go"
)

type OrderPublisher interface {
	PublishOrderCreated(context.Context, models.Order) error
	Close() error
}

type KafkaOrderPublisher struct {
	writer *kafka.Writer
	topic  string
}

func NewKafkaOrderPublisher(brokers []string, topic string) *KafkaOrderPublisher {
	return &KafkaOrderPublisher{
		topic: topic,
		writer: &kafka.Writer{
			Addr:         kafka.TCP(brokers...),
			Topic:        topic,
			RequiredAcks: kafka.RequireOne,
		},
	}
}

func (p *KafkaOrderPublisher) PublishOrderCreated(ctx context.Context, order models.Order) error {
	payload, err := json.Marshal(struct {
		EventType  string       `json:"eventType"`
		OccurredAt time.Time    `json:"occurredAt"`
		Order      models.Order `json:"order"`
	}{
		EventType:  "order.created",
		OccurredAt: time.Now().UTC(),
		Order:      order,
	})
	if err != nil {
		return err
	}

	return p.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(order.ID),
		Value: payload,
	})
}

func (p *KafkaOrderPublisher) Close() error {
	return p.writer.Close()
}
