package clicks

import (
	"context"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/prasadgujar22/shorty/pkg/events"
)

// JetStream publishes clicks to a stream and waits for the server ack.
type JetStream struct {
	JS     jetstream.JetStream
	Stream string
}

func (j JetStream) Publish(ctx context.Context, c events.Click) error {
	data, err := c.Encode()
	if err != nil {
		return err
	}
	_, err = j.JS.Publish(ctx, events.Subject, data,
		jetstream.WithMsgID(c.ID),            // server-side de-dup within the duplicate window
		jetstream.WithExpectStream(j.Stream), // fail loudly if the subject is not captured
	)
	return err
}
