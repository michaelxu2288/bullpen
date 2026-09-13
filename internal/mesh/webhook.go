package mesh

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Webhook wire headers. Signature covers "<timestamp>.<body>" so a captured
// body cannot be replayed under a fresh timestamp.
const (
	HeaderDelivery  = "X-Bullpen-Delivery"
	HeaderTimestamp = "X-Bullpen-Timestamp"
	HeaderSignature = "X-Bullpen-Signature"
	HeaderFrom      = "X-Bullpen-From"
	HeaderAttempt   = "X-Bullpen-Attempt"
)

// replayWindow bounds how old a delivery timestamp may be. Anything outside it
// is rejected even with a valid signature.
const replayWindow = 5 * time.Minute

// Delivery is the signed envelope one agent POSTs to another's inbox.
type Delivery struct {
	ID      string    `json:"id"`
	Message Message   `json:"message"`
	Attempt int       `json:"attempt"`
	SentAt  time.Time `json:"sent_at"`
}

// Sign returns the hex HMAC-SHA256 of "<timestamp>.<body>".
func Sign(secret string, timestamp int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.", timestamp)
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature checks an inbound delivery's signature and freshness.
// Returns a descriptive error so operators can tell a bad secret from a replay.
func VerifySignature(secret, signature, timestamp string, body []byte, now time.Time) error {
	if secret == "" {
		return fmt.Errorf("no shared secret configured for inbound deliveries")
	}
	if signature == "" || timestamp == "" {
		return fmt.Errorf("missing signature or timestamp header")
	}

	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("malformed timestamp header")
	}
	age := now.Sub(time.Unix(ts, 0))
	if age < -replayWindow || age > replayWindow {
		return fmt.Errorf("delivery timestamp outside the %s replay window", replayWindow)
	}

	expected := Sign(secret, ts, body)
	// constant-time: a timing oracle here leaks the secret one byte at a time
	if subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) != 1 {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}

// NewSecret generates a shared secret for a newly registered crew.
func NewSecret() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("failed to generate a peer secret: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

// DeliveryLog remembers recently seen delivery ids so a retried webhook is
// applied once. Webhook transports retry on timeout, so the same delivery
// legitimately arrives twice and the receiver must be idempotent.
type DeliveryLog struct {
	mu   sync.Mutex
	seen map[string]time.Time
	ttl  time.Duration
	max  int
}

func NewDeliveryLog(ttl time.Duration) *DeliveryLog {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &DeliveryLog{seen: make(map[string]time.Time), ttl: ttl, max: 4096}
}

// Observe records a delivery id and reports whether it is a duplicate.
func (l *DeliveryLog) Observe(id string) bool {
	if id == "" {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if at, ok := l.seen[id]; ok && now.Sub(at) < l.ttl {
		return true
	}
	if len(l.seen) >= l.max {
		for key, at := range l.seen {
			if now.Sub(at) >= l.ttl {
				delete(l.seen, key)
			}
		}
		// still full of fresh entries: drop the whole window rather than grow
		if len(l.seen) >= l.max {
			l.seen = make(map[string]time.Time, l.max)
		}
	}
	l.seen[id] = now
	return false
}

// RetryPolicy controls webhook redelivery.
type RetryPolicy struct {
	Attempts int
	Backoff  time.Duration
	// MaxBackoff caps exponential growth.
	MaxBackoff time.Duration
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{Attempts: 3, Backoff: 250 * time.Millisecond, MaxBackoff: 2 * time.Second}
}

func (p RetryPolicy) delay(attempt int) time.Duration {
	d := p.Backoff << (attempt - 1)
	if p.MaxBackoff > 0 && d > p.MaxBackoff {
		return p.MaxBackoff
	}
	return d
}

// WebhookTransport delivers bus messages to remote agents over signed HTTP.
type WebhookTransport struct {
	Peers  *PeerRegistry
	HTTP   *http.Client
	Retry  RetryPolicy
	SelfID string
	// Sleep is injected so tests do not wait out the backoff.
	Sleep func(time.Duration)
}

func NewWebhookTransport(peers *PeerRegistry, selfID string) *WebhookTransport {
	return &WebhookTransport{
		Peers:  peers,
		HTTP:   &http.Client{Timeout: 10 * time.Second},
		Retry:  DefaultRetryPolicy(),
		SelfID: selfID,
		Sleep:  time.Sleep,
	}
}

// Known reports whether a node id is a registered webhook peer.
func (t *WebhookTransport) Known(nodeID string) bool {
	return t.Peers != nil && t.Peers.Known(nodeID)
}

// Deliver POSTs one message to one peer, retrying per the policy. A 4xx other
// than 408/429 is treated as permanent: redelivering a rejected payload just
// burns the peer's error budget.
func (t *WebhookTransport) Deliver(ctx context.Context, m Message) error {
	peer, ok := t.Peers.Get(m.To)
	if !ok {
		return fmt.Errorf("no registered peer %q", m.To)
	}

	deliveryID, err := NewSecret()
	if err != nil {
		return err
	}

	attempts := t.Retry.Attempts
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		body, err := json.Marshal(Delivery{
			ID:      deliveryID,
			Message: m,
			Attempt: attempt,
			SentAt:  time.Now(),
		})
		if err != nil {
			return fmt.Errorf("failed to encode delivery: %w", err)
		}

		status, err := t.post(ctx, peer, deliveryID, attempt, body)
		if err == nil {
			t.Peers.MarkDelivered(peer.ID)
			return nil
		}
		lastErr = err

		if status >= 400 && status < 500 && status != http.StatusRequestTimeout && status != http.StatusTooManyRequests {
			t.Peers.MarkFailed(peer.ID)
			return fmt.Errorf("permanent delivery failure to %s: %w", peer.ID, err)
		}
		if attempt < attempts {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			t.Sleep(t.Retry.delay(attempt))
		}
	}

	quarantined := t.Peers.MarkFailed(peer.ID)
	if quarantined {
		return fmt.Errorf("delivery to %s failed %d times, peer quarantined: %w", peer.ID, attempts, lastErr)
	}
	return fmt.Errorf("delivery to %s failed after %d attempts: %w", peer.ID, attempts, lastErr)
}

func (t *WebhookTransport) post(ctx context.Context, peer Peer, deliveryID string, attempt int, body []byte) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, peer.URL, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("failed to build request: %w", err)
	}

	ts := time.Now().Unix()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderDelivery, deliveryID)
	req.Header.Set(HeaderTimestamp, strconv.FormatInt(ts, 10))
	req.Header.Set(HeaderSignature, Sign(peer.Secret, ts, body))
	req.Header.Set(HeaderFrom, t.SelfID)
	req.Header.Set(HeaderAttempt, strconv.Itoa(attempt))

	client := t.HTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("post to %s failed: %w", peer.URL, err)
	}
	defer resp.Body.Close()

	detail, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("peer answered %d: %s", resp.StatusCode, bytes.TrimSpace(detail))
	}
	return resp.StatusCode, nil
}

// Broadcast fans a message out to every registered peer except the sender.
// Failures are collected rather than aborting the fan-out: one dead agent must
// not stop the rest of the crew from hearing the message.
func (t *WebhookTransport) Broadcast(ctx context.Context, m Message) []error {
	var errs []error
	for _, peer := range t.Peers.List() {
		if peer.ID == m.From || peer.ID == t.SelfID || peer.Quarantined {
			continue
		}
		directed := m
		directed.To = peer.ID
		if err := t.Deliver(ctx, directed); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}
