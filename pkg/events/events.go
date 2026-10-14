// Package events defines the click event contract between redirect-svc
// (producer) and analytics-svc (consumer), and the JetStream stream that
// carries it. Both services call EnsureStream with the same config, so
// startup order does not matter.
package events

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/prasadgujar22/shorty/pkg/config"
)

// DefaultSubject is where click events are published (versioned so the
// payload can evolve alongside a new subject).
const DefaultSubject = "clicks.v1"

// Click is a single redirect. ID is globally unique and is used for
// de-duplication twice: by JetStream (Nats-Msg-Id, within the duplicate
// window) and by analytics-svc (processed_events table, forever-ish).
type Click struct {
	ID       string    `json:"id"`
	Slug     string    `json:"slug"`
	At       time.Time `json:"at"`
	Referrer string    `json:"referrer"` // host only, or "direct"
}

// NewID returns a 128-bit random hex ID.
func NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Encode marshals c.
func (c Click) Encode() ([]byte, error) { return json.Marshal(c) }

// Decode unmarshals and validates a click.
func Decode(b []byte) (Click, error) {
	var c Click
	if err := json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	if c.ID == "" || c.Slug == "" || c.At.IsZero() {
		return c, errors.New("click event missing id, slug or at")
	}
	return c, nil
}

// ReferrerHost reduces a Referer header to its host (we never store full
// referring URLs: they can contain personal data).
func ReferrerHost(ref string) string {
	if ref == "" {
		return "direct"
	}
	u, err := url.Parse(ref)
	if err != nil || u.Host == "" {
		return "unknown"
	}
	h := strings.ToLower(u.Hostname())
	h = strings.TrimPrefix(h, "www.")
	if len(h) > 253 {
		h = h[:253]
	}
	return h
}

// StreamConfig is the CLICKS stream definition.
type StreamConfig struct {
	Name            string
	Subject         string
	Replicas        int
	MaxAge          time.Duration
	MaxBytes        int64
	DuplicateWindow time.Duration
}

// StreamConfigFromEnv reads CLICKS_STREAM_* variables.
func StreamConfigFromEnv(env *config.Env) StreamConfig {
	return StreamConfig{
		Name:            env.String("CLICKS_STREAM", "CLICKS"),
		Subject:         env.String("CLICKS_SUBJECT", DefaultSubject),
		Replicas:        env.Int("CLICKS_STREAM_REPLICAS", 1),
		MaxAge:          env.Duration("CLICKS_STREAM_MAX_AGE", 7*24*time.Hour),
		MaxBytes:        int64(env.Int("CLICKS_STREAM_MAX_BYTES", 1<<30)),
		DuplicateWindow: env.Duration("CLICKS_STREAM_DUPLICATE_WINDOW", 2*time.Minute),
	}
}

// EnsureStream creates or updates the stream. File storage + limits
// retention: messages survive NATS restarts and stay until MaxAge so a
// lagging consumer can catch up; consumers track their own position.
func EnsureStream(ctx context.Context, js jetstream.JetStream, c StreamConfig) (jetstream.Stream, error) {
	return js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:        c.Name,
		Description: "Shorty click events (redirect-svc -> analytics-svc)",
		Subjects:    []string{c.Subject},
		Storage:     jetstream.FileStorage,
		Retention:   jetstream.LimitsPolicy,
		Replicas:    c.Replicas,
		MaxAge:      c.MaxAge,
		MaxBytes:    c.MaxBytes,
		Discard:     jetstream.DiscardOld,
		Duplicates:  c.DuplicateWindow,
	})
}

// Connect opens a NATS connection that retries forever in the background,
// so a NATS outage degrades the service instead of crashing it.
func Connect(url, name string, log *slog.Logger) (*nats.Conn, error) {
	nc, err := nats.Connect(url,
		nats.Name(name),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(250*time.Millisecond),
		nats.ReconnectJitter(250*time.Millisecond, time.Second),
		nats.PingInterval(5*time.Second),
		nats.MaxPingsOutstanding(2),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) { log.Warn("nats disconnected", "err", err) }),
		nats.ReconnectHandler(func(c *nats.Conn) { log.Info("nats reconnected", "server", c.ConnectedUrlRedacted()) }),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) { log.Error("nats async error", "err", err) }),
	)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}
	return nc, nil
}

// Ping is a readiness check for a NATS connection.
func Ping(nc *nats.Conn) func(context.Context) error {
	return func(context.Context) error {
		if s := nc.Status(); s != nats.CONNECTED {
			return fmt.Errorf("nats %s", s)
		}
		return nil
	}
}
