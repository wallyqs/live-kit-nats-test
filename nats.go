package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
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

type NatsConfig struct {
	URL                    string `yaml:"url"`
	ClientName             string `yaml:"-"`
	JetStreamReplicas      int    `yaml:"jetstream_replicas"`
	PublishAsyncMaxPending int    `yaml:"publish_async_max_pending"`
}

func CreateNatsConnection(conf *NatsConfig, name string) (*nats.Conn, error) {
	if name == "" {
		name = conf.ClientName
	}
	return nats.Connect(conf.URL,
		nats.Name(name),
		nats.PingInterval(30*time.Second),
		nats.MaxPingsOutstanding(4), // close connection after 2 minutes of missed pings
		nats.DiscoveredServersHandler(func(nc *nats.Conn) {
			logger.Infow("nats discovered servers",
				"discoveredServers", strings.Join(nc.DiscoveredServers(), ", "),
			)
		}),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			logger.Errorw("nats disconnected", err,
				"serverName", nc.ConnectedServerName(),
			)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			logger.Infow("nats reconnect",
				"serverName", nc.ConnectedServerName(),
			)
		}),
		nats.ErrorHandler(func(conn *nats.Conn, subscription *nats.Subscription, err error) {
			var values []interface{}
			if conn != nil {
				values = append(values,
					"reconnects", conn.Reconnects,
					"numSubscriptions", conn.NumSubscriptions(),
					"connectedAddr", conn.ConnectedAddr(),
					"connectedClusterName", conn.ConnectedClusterName(),
					"connectedServerId", conn.ConnectedServerId(),
					"connectedServerName", conn.ConnectedServerName(),
					"isClosed", conn.IsClosed(),
					"isConnected", conn.IsConnected(),
					"isDraining", conn.IsDraining(),
					"isReconnecting", conn.IsReconnecting(),
				)
			}
			if subscription != nil {
				values = append(values, "subject", subscription.Subject)
			}
			logger.Errorw("nats error", err, values...)
		}),
	)
}
