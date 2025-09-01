package redisx

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

type EventsPubSub struct {
	rdb     *redis.Client
	channel string
}

func NewEventsPubSub(rdb *redis.Client) *EventsPubSub {
	return &EventsPubSub{
		rdb:     rdb,
		channel: ChannelEventsChanged(),
	}
}

type eventChangedMsg struct {
	Type    string `json:"type"`
	EventID int64  `json:"event_id"`
	TsUnix  int64  `json:"ts_unix"`
}

func (p *EventsPubSub) PublishEventChanged(ctx context.Context, eventID int64) error {
	msg := eventChangedMsg{
		Type:    "event_changed",
		EventID: eventID,
		TsUnix:  time.Now().Unix(),
	}

	b, _ := json.Marshal(msg)

	return p.rdb.Publish(ctx, p.channel, b).Err()
}

func (p *EventsPubSub) Subscribe(ctx context.Context, handler func(ctx context.Context, eventID int64)) error {
	sub := p.rdb.Subscribe(ctx, p.channel)
	defer sub.Close()

	ch := sub.Channel(redis.WithChannelSize(256))
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case m, ok := <-ch:
			if !ok {
				return nil
			}
			var ev eventChangedMsg
			if err := json.Unmarshal([]byte(m.Payload), &ev); err == nil &&
				ev.EventID != 0 {
				handler(ctx, ev.EventID)
			}
		}
	}
}
