package main

import (
	"os"

	"gopkg.in/yaml.v3"

	"github.com/livekit/protocol/logger"
)

type Config struct {
	NumShards  int  `yaml:"num_shards"`
	NumRooms   int  `yaml:"num_rooms"`
	UsePooling bool `yaml:"use_pooling"`
	// number of messages per room / sec
	MessageRate            int           `yaml:"message_rate"`
	ProducersPerRoom       int           `yaml:"producers_per_room"`
	ConsumersPerRoom       int           `yaml:"consumers_per_room"`
	MessageRetentionSec    int           `yaml:"message_retention_sec"`
	ConsumersPerConnection int           `yaml:"consumers_per_connection"`
	NATS                   NatsConfig    `yaml:"nats"`
	Logging                logger.Config `yaml:"logging"`
}

func NewConfig() (*Config, error) {
	c := &Config{
		NumShards:              50,
		NumRooms:               1000,
		UsePooling:             true,
		MessageRate:            10,
		ConsumersPerRoom:       10,
		ProducersPerRoom:       1,
		MessageRetentionSec:    10,
		ConsumersPerConnection: 10,
		NATS: NatsConfig{
			URL:                    "nats://localhost:4000",
			JetStreamReplicas:      3,
			PublishAsyncMaxPending: 200000,
		},
		Logging: logger.Config{
			JSON:  false,
			Level: "info",
		},
	}
	if v := os.Getenv("NATS_TEST_CONFIG"); v != "" {
		if err := yaml.Unmarshal([]byte(v), c); err != nil {
			return nil, err
		}
	}
	return c, nil
}
