package main

import (
	"math"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/gammazero/workerpool"
	"github.com/nats-io/nats.go"

	"github.com/livekit/protocol/logger"
)

func main() {
	conf, err := NewConfig()
	if err != nil {
		panic("failed to parse config: " + err.Error())
	}
	logger.InitFromConfig(conf.Logging, "nats-test")

	// create nats connection for setup
	nc, err := CreateNatsConnection(&conf.NATS, "")
	if err != nil {
		logger.Errorw("could not create nats connection", err)
		return
	}

	// create jetstream context
	js, err := nc.JetStream(nats.PublishAsyncMaxPending(conf.NATS.PublishAsyncMaxPending))
	if err != nil {
		logger.Errorw("could not create jetstream context", err)
		return
	}

	// create streams
	if err := SetupStreams(nc, js, conf); err != nil {
		logger.Errorw("could not create streams", err)
		return
	}

	// setup publishers and consumers
	producers := make([]*Producer, 0, conf.NumRooms)
	consumers := make([]*Consumer, 0)
	var producerConnections, consumerConnections []*nats.Conn
	if conf.UsePooling {
		producerConnections, err = createConnections(&conf.NATS, conf.NumRooms)
		if err != nil {
			logger.Errorw("could not create nats connections", err)
			return
		}
		logger.Infow("Created producer connections", "count", len(producerConnections))
	}

	if conf.UsePooling {
		numConsumerConnections := int(math.Ceil(float64(conf.ConsumersPerRoom*conf.NumRooms) / float64(conf.ConsumersPerConnection)))
		consumerConnections, err = createConnections(&conf.NATS, numConsumerConnections)
		if err != nil {
			logger.Errorw("could not create nats connections", err)
			return
		}
		logger.Infow("Created consumer connections", "count", len(consumerConnections))
	}

	idx := 0
	// keep track of consumers on a connection
	consumerCount := 0
	for i := 0; i < conf.NumRooms; i++ {
		roomSubjectPrefix := SubjectPrefixForRoom(i, conf.NumShards)
		roomStr := strconv.Itoa(i)

		if conf.ConsumersPerRoom > 0 {
			if conf.UsePooling {
				nc = consumerConnections[idx]
				js, err = nc.JetStream()
			}

			if err != nil {
				logger.Errorw("could not create jetstream context", err)
				return
			}
			// start consumers first
			for j := 0; j < conf.ConsumersPerRoom; j++ {
				c := NewConsumer(
					roomStr,
					strconv.Itoa(j),
					js,
					roomSubjectPrefix,
				)
				consumerCount++
				if consumerCount == conf.ConsumersPerConnection {
					idx++
				}

				consumers = append(consumers, c)
			}
		}

		if conf.UsePooling {
			nc = producerConnections[i]
			js, err = nc.JetStream()
			if err != nil {
				logger.Errorw("could not create jetstream context", err)
				return
			}
		}
		for j := 0; j < conf.ProducersPerRoom; j++ {
			p := NewProducer(
				roomStr,
				js,
				roomSubjectPrefix,
				conf.MessageRate,
			)
			producers = append(producers, p)
		}
	}

	logger.Infow("starting consumers")
	pool := workerpool.New(30)
	wg := sync.WaitGroup{}
	for _, c := range consumers {
		c := c
		wg.Add(1)
		pool.Submit(func() {
			defer wg.Done()
			err := c.Start()
			if err != nil {
				logger.Errorw("could not start consumer", err)
			}
		})
	}
	wg.Wait()

	logger.Infow("starting producers")
	for _, p := range producers {
		p.Start()
	}

	numConsumers := len(consumers)
	logger.Infow("started tests", "numProducers", len(producers), "numConsumers", numConsumers)

	startedAt := time.Now()
	doneChan := make(chan struct{})
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGABRT)

	go func() {
		<-sigChan
		close(doneChan)
	}()

	<-doneChan

	logger.Infow("stopping...")
	wg = sync.WaitGroup{}
	for _, p := range producers {
		p := p
		wg.Add(1)
		pool.Submit(func() {
			defer wg.Done()
			p.Stop()
		})
	}
	for _, c := range consumers {
		c := c
		wg.Add(1)
		pool.Submit(func() {
			defer wg.Done()
			c.Stop()
		})
	}
	wg.Wait()

	duration := time.Since(startedAt)

	stats := make(map[string]*roomStat)
	for _, p := range producers {
		stat := &roomStat{
			room:             p.id,
			messagesProduced: p.MessageCount(),
			bytesProduced:    p.BytesCount(),
		}
		stats[p.id] = stat
	}
	for _, c := range consumers {
		stat := stats[c.room]
		if stat == nil {
			stat = &roomStat{
				room: c.room,
			}
			stats[c.room] = stat
		}
		stat.numConsumers++
		stat.messagesConsumed += c.MessageCount()
		stat.bytesConsumed += c.BytesCount()
	}

	// determine lowest, highest, and avg
	var lowest, highest *roomStat
	avg := &roomStat{}
	for _, stat := range stats {
		if lowest == nil || stat.messagesConsumed < lowest.messagesConsumed {
			lowest = stat
		}
		if highest == nil || stat.messagesConsumed > highest.messagesConsumed {
			highest = stat
		}
		avg.messagesConsumed += stat.messagesConsumed
		avg.messagesProduced += stat.messagesProduced
		avg.bytesConsumed += stat.bytesConsumed
		avg.numConsumers += stat.numConsumers
	}

	avg.messagesConsumed /= len(stats)
	avg.messagesProduced /= len(stats)
	avg.bytesConsumed /= len(stats)
	avg.numConsumers /= len(stats)

	logger.Infow("finished NATS test")
	if lowest != nil {
		logger.Infow("lowest stats", lowest.Fields(duration)...)
	}
	if highest != nil {
		logger.Infow("highest stats", highest.Fields(duration)...)
	}
	logger.Infow("avg stats", avg.Fields(duration)...)
}

func createConnections(natsConfig *NatsConfig, numConnections int) ([]*nats.Conn, error) {
	// numConnections := c.ConsumersPerRoom * c.NumRooms / c.ConsumersPerConnection
	conns := make([]*nats.Conn, 0, numConnections)
	for i := 0; i < numConnections; i++ {
		nc, err := CreateNatsConnection(natsConfig, "")
		if err != nil {
			return nil, err
		}
		conns = append(conns, nc)
	}
	return conns, nil
}

type roomStat struct {
	room             string
	numConsumers     int
	messagesProduced int
	bytesProduced    int
	messagesConsumed int
	bytesConsumed    int
}

func (r *roomStat) Fields(duration time.Duration) []interface{} {
	fields := make([]interface{}, 0, 20)
	fields = append(fields, "room", r.room)
	fields = append(fields, "numConsumers", r.numConsumers)
	fields = append(fields, "messagesProduced", r.messagesProduced)
	fields = append(fields, "messagesConsumed", r.messagesConsumed)
	fields = append(fields, "producerMessageRate", math.Ceil(float64(r.messagesProduced)/duration.Seconds()))
	fields = append(fields, "producerMessageRateBytes", math.Ceil(float64(r.bytesProduced)/duration.Seconds()))
	fields = append(fields, "consumerMessageRate", math.Ceil(float64(r.messagesConsumed)/duration.Seconds()))
	fields = append(fields, "consumerMessageRateBytes", math.Ceil(float64(r.bytesConsumed)/duration.Seconds()))
	return fields
}
