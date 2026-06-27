package config

import (
	"os"
	"strings"
)

type Config struct {
	Port               string
	DatabaseURL        string
	KafkaBrokers       []string
	KafkaOrderTopic    string
	KafkaConsumerGroup string
}

func Load() Config {
	return Config{
		Port:               env("PORT", "8080"),
		DatabaseURL:        env("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/zus_home_assessment?sslmode=disable"),
		KafkaBrokers:       splitCSV(env("KAFKA_BROKERS", "localhost:9092")),
		KafkaOrderTopic:    env("KAFKA_ORDER_TOPIC", "orders.created"),
		KafkaConsumerGroup: env("KAFKA_CONSUMER_GROUP", "order-summary-worker"),
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
