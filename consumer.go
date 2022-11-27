package main

import (
	"fmt"
	"sync/atomic"

	"github.com/nats-io/nats.go"
)

type Consumer struct {
	room          string
	consumerId    string
	js            nats.JetStreamContext
	subjectFilter string
	subscription  *nats.Subscription
	consumeCount  atomic.Int64
	consumeBytes  atomic.Int64
}

func NewConsumer(room, consumer string, js nats.JetStreamContext, subjectPrefix string) *Consumer {
	return &Consumer{
		room:          room,
		consumerId:    consumer,
		js:            js,
		subjectFilter: fmt.Sprintf("%s.>", subjectPrefix),
	}
}

func (c *Consumer) ID() string {
	return fmt.Sprintf("c-%s-%s", c.room, c.consumerId)
}

func (c *Consumer) Start() error {
	if c.subscription != nil {
		return nil
	}
	//logger.Infow("subscribing", "room", c.room, "consumer", c.consumerId, "subject", c.subjectFilter)
	sub, err := c.js.Subscribe(c.subjectFilter, func(msg *nats.Msg) {
		//logger.Infow("received message", "room", c.room, "consumer", c.consumerId, "subject", msg.Subject)
		c.consumeCount.Add(1)
		c.consumeBytes.Add(int64(len(msg.Data)))
	})
	if err != nil {
		return err
	}
	c.subscription = sub
	return nil
}

func (c *Consumer) Stop() {
	if c.subscription == nil {
		return
	}
	_ = c.subscription.Unsubscribe()
	c.subscription = nil
}

func (c *Consumer) MessageCount() int {
	return int(c.consumeCount.Load())
}

func (c *Consumer) BytesCount() int {
	return int(c.consumeBytes.Load())
}
