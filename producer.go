package main

import (
	"fmt"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/livekit/protocol/logger"
)

type Producer struct {
	id            string
	js            nats.JetStreamContext
	subjectPrefix string
	// messages/room/sec
	messageRate  int
	closeChan    chan struct{}
	produceCount atomic.Int64
	produceBytes atomic.Int64
}

func NewProducer(id string, js nats.JetStreamContext, subjectPrefix string, messageRate int) *Producer {
	return &Producer{
		id:            id,
		js:            js,
		subjectPrefix: subjectPrefix,
		messageRate:   messageRate,
		closeChan:     make(chan struct{}),
	}
}

func (p *Producer) Start() {
	go p.worker()
}

func (p *Producer) Stop() {
	if !p.IsRunning() {
		return
	}
	close(p.closeChan)
}

func (p *Producer) IsRunning() bool {
	select {
	case <-p.closeChan:
		return false
	default:
		return true
	}
}

func (p *Producer) MessageCount() int {
	return int(p.produceCount.Load())
}

func (p *Producer) BytesCount() int {
	return int(p.produceBytes.Load())
}

// publish worker, sends up to messageRate messages per second
func (p *Producer) worker() {
	closeChan := p.closeChan
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	failed := false
	for {
		select {
		case <-closeChan:
			return
		case <-ticker.C:
			// send a bunch of messages
			for i := 0; i < p.messageRate; i++ {
				subject := fmt.Sprintf("%s.message%d", p.subjectPrefix, rand.Intn(10000))
				data := make([]byte, 1024)
				//logger.Infow("publishing message", "subject", subject, "room", p.id)
				_, err := p.js.PublishAsync(subject, data)
				if err != nil {
					logger.Warnw("failed to publish message", err,
						"id", p.id,
						"subject", subject,
						"pending", p.js.PublishAsyncPending())
					failed = true
					break
				}
				if failed {
					failed = false
					logger.Infow("recovered from publish failure", "id", p.id)
				}
				p.produceCount.Add(1)
				p.produceBytes.Add(int64(len(data)))
			}
		}
	}
}
