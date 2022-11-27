package main

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/livekit/protocol/logger"
)

const streamPrefix = "testStream:"

func SetupStreams(nc *nats.Conn, js nats.JetStreamContext, c *Config) error {
	for i := 0; i < c.NumShards; i++ {
		if err := setupStream(nc, js, c, StreamNameForShard(i)); err != nil {
			return err
		}
	}
	return nil
}

func StreamNameForShard(shard int) string {
	return streamPrefix + strconv.Itoa(shard)
}

func SubjectPrefixForRoom(roomNum int, numShards int) string {
	shard := roomNum % numShards
	return fmt.Sprintf("%s.room%d", StreamNameForShard(shard), roomNum)
}

func setupStream(nc *nats.Conn, js nats.JetStreamContext, c *Config, name string) error {
	maxAge := time.Duration(c.MessageRetentionSec) * time.Second
	streamConfig := &nats.StreamConfig{
		Name:      name,
		Subjects:  []string{fmt.Sprintf("%s.>", name)},
		Retention: nats.LimitsPolicy,
		Discard:   nats.DiscardOld,
		MaxAge:    maxAge,
		Storage:   nats.MemoryStorage,
		Replicas:  c.NATS.JetStreamReplicas,
		Placement: &nats.Placement{
			Cluster: nc.ConnectedClusterName(),
		},
	}

	info, err := js.StreamInfo(name)
	if err == nil && info != nil {
		if info.Config.MaxAge != maxAge {
			_, err = js.UpdateStream(streamConfig)
		}
		return err
	}
	_, err = js.AddStream(streamConfig)
	if err != nil {
		if errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
			// stream may have been added by another process, ignore
			return nil
		}
		return err
	}
	logger.Infow("created or updated JetStream", "name", name)
	return err
}
