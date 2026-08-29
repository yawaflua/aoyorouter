package push

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/push_repo"
	"github.com/yawaflua/aoyorouter/internal/config"
	"github.com/yawaflua/aoyorouter/internal/models"
)

const (
	notificationTTL = 86400
)

type Payload struct {
	Title      string `json:"title"`
	Body       string `json:"body"`
	Tag        string `json:"tag"`
	ProviderID string `json:"providerId"`
	Subject    string `json:"subject"`
	URL        string `json:"url"`
	Urgent     bool   `json:"-"`
}

type Notifier struct {
	repo   *push_repo.PushRepo
	cfg    *config.C
	logger *slog.Logger

	keysMu sync.Mutex
	keys   *models.VapidKeys
}

func NewNotifier(repo *push_repo.PushRepo, cfg *config.C, logger *slog.Logger) *Notifier {
	return &Notifier{repo: repo, cfg: cfg, logger: logger}
}

func (n *Notifier) resolveKeys(ctx context.Context) (*models.VapidKeys, error) {
	n.keysMu.Lock()
	defer n.keysMu.Unlock()

	if n.keys != nil {
		return n.keys, nil
	}

	if n.cfg != nil && n.cfg.VapidPublicKey != "" && n.cfg.VapidPrivateKey != "" {
		n.keys = &models.VapidKeys{PublicKey: n.cfg.VapidPublicKey, PrivateKey: n.cfg.VapidPrivateKey}
		return n.keys, nil
	}

	stored, err := n.repo.GetVapidKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("push.Notifier.resolveKeys: %w", err)
	}
	if stored != nil {
		n.keys = stored
		return n.keys, nil
	}

	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return nil, fmt.Errorf("push.Notifier.resolveKeys: %w", err)
	}

	saved, err := n.repo.SaveVapidKeys(ctx, &models.VapidKeys{PublicKey: publicKey, PrivateKey: privateKey})
	if err != nil {
		return nil, fmt.Errorf("push.Notifier.resolveKeys: %w", err)
	}

	n.logger.Info("generated vapid keys")
	n.keys = saved
	return n.keys, nil
}

func (n *Notifier) PublicKey(ctx context.Context) (string, error) {
	keys, err := n.resolveKeys(ctx)
	if err != nil {
		return "", fmt.Errorf("push.Notifier.PublicKey: %w", err)
	}
	return keys.PublicKey, nil
}

func (n *Notifier) Enabled(ctx context.Context) bool {
	keys, err := n.resolveKeys(ctx)
	if err != nil {
		n.logger.Error("failed to resolve vapid keys", "error", err)
		return false
	}
	return keys.PublicKey != "" && keys.PrivateKey != ""
}

func (n *Notifier) Notify(ctx context.Context, subject string, payload Payload) error {
	keys, err := n.resolveKeys(ctx)
	if err != nil {
		return fmt.Errorf("push.Notifier.Notify: %w", err)
	}

	subs, err := n.repo.SubscribersOf(ctx, subject)
	if err != nil {
		return fmt.Errorf("push.Notifier.Notify: %w", err)
	}
	if len(subs) == 0 {
		return nil
	}

	payload.Subject = subject
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("push.Notifier.Notify: %w", err)
	}

	if _, err := n.repo.AppendEvent(ctx, &models.NotificationEvent{
		Subject:    subject,
		Title:      payload.Title,
		Body:       payload.Body,
		Tag:        payload.Tag,
		ProviderID: payload.ProviderID,
		URL:        payload.URL,
	}); err != nil {
		n.logger.Error("failed to persist notification event", "subject", subject, "error", err)
	}

	urgency := webpush.UrgencyNormal
	if payload.Urgent {
		urgency = webpush.UrgencyHigh
	}

	subscriber := ""
	if n.cfg != nil {
		subscriber = n.cfg.VapidSubject
	}

	var lastErr error
	failed := 0
	attempted := 0
	for _, sub := range subs {
		if models.IsInAppEndpoint(sub.Endpoint) {
			continue
		}

		attempted++

		options := &webpush.Options{
			Subscriber:      subscriber,
			VAPIDPublicKey:  keys.PublicKey,
			VAPIDPrivateKey: keys.PrivateKey,
			TTL:             notificationTTL,
			Urgency:         urgency,
		}

		target := &webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys: webpush.Keys{
				Auth:   sub.Keys.Auth,
				P256dh: sub.Keys.P256dh,
			},
		}

		resp, err := webpush.SendNotificationWithContext(ctx, body, target, options)
		if err != nil {
			failed++
			lastErr = err
			n.logger.Error("failed to send push notification", "endpoint", sub.Endpoint, "subject", subject, "error", err)
			continue
		}

		statusCode := resp.StatusCode
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		if statusCode == http.StatusNotFound || statusCode == http.StatusGone {
			n.logger.Info("dropping expired push subscription", "endpoint", sub.Endpoint, "status", statusCode)
			if err := n.repo.DeleteByEndpoint(ctx, sub.Endpoint); err != nil {
				n.logger.Error("failed to delete expired push subscription", "endpoint", sub.Endpoint, "error", err)
			}
			failed++
			lastErr = fmt.Errorf("push subscription gone: %d", statusCode)
			continue
		}

		if statusCode < 200 || statusCode > 299 {
			failed++
			lastErr = fmt.Errorf("push endpoint returned %d", statusCode)
			n.logger.Error("push endpoint rejected notification", "endpoint", sub.Endpoint, "subject", subject, "status", statusCode)
			continue
		}
	}

	if attempted > 0 && failed == attempted && lastErr != nil {
		return fmt.Errorf("push.Notifier.Notify: %w", lastErr)
	}
	return nil
}
