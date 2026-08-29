package push

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/push_repo"
	"github.com/yawaflua/aoyorouter/internal/models"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
	"google.golang.org/protobuf/proto"
)

const (
	eventExhausted = "exhausted"
	eventReset     = "reset"

	exhaustedThreshold = 99.999
	resetDropThreshold = 1.0
)

type event struct {
	kind    string
	payload Payload
}

type QuotaWatcher struct {
	notifier *Notifier
	repo     *push_repo.PushRepo
	logger   *slog.Logger
	previous sync.Map
}

func NewQuotaWatcher(notifier *Notifier, repo *push_repo.PushRepo, logger *slog.Logger) *QuotaWatcher {
	return &QuotaWatcher{notifier: notifier, repo: repo, logger: logger}
}

func (w *QuotaWatcher) Observe(ctx context.Context, providerID, providerName string, quota *aoyorouter.ProviderQuota) {
	if quota == nil {
		return
	}

	snapshot, _ := proto.Clone(quota).(*aoyorouter.ProviderQuota)
	stored, hadPrevious := w.previous.Load(providerID)
	w.previous.Store(providerID, snapshot)
	if !hadPrevious {
		return
	}

	prev, ok := stored.(*aoyorouter.ProviderQuota)
	if !ok {
		return
	}

	subject := models.QuotaTopic(providerID)
	for _, ev := range diffQuota(prev, quota, providerName, providerID) {
		if err := w.notifier.Notify(ctx, subject, ev.payload); err != nil {
			w.logger.Error("failed to notify quota subscribers", "provider_id", providerID, "kind", ev.kind, "error", err)
		}
	}
}

func (w *QuotaWatcher) SubscribedProviderIDs(ctx context.Context) (map[string]struct{}, error) {
	subjects, err := w.repo.SubscribedSubjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("push.QuotaWatcher.SubscribedProviderIDs: %w", err)
	}

	ids := make(map[string]struct{}, len(subjects))
	for _, subject := range subjects {
		if id, ok := models.ProviderIDFromQuotaTopic(subject); ok {
			ids[id] = struct{}{}
		}
	}
	return ids, nil
}

func diffQuota(prev, next *aoyorouter.ProviderQuota, providerName, providerID string) []event {
	if prev == nil || next == nil {
		return nil
	}

	old := make(map[string]*aoyorouter.ProviderQuotaWindow, len(prev.GetQuotas()))
	for _, window := range prev.GetQuotas() {
		if window == nil {
			continue
		}
		old[windowKey(window)] = window
	}

	events := make([]event, 0)
	for _, window := range next.GetQuotas() {
		if window == nil {
			continue
		}
		previous, ok := old[windowKey(window)]
		if !ok {
			continue
		}

		label := windowLabel(window)

		switch {
		case previous.GetUsedPercent() < 100 && window.GetUsedPercent() >= exhaustedThreshold:
			body := fmt.Sprintf("%s is fully used.", label)
			if window.GetResetsAt() != "" {
				body = fmt.Sprintf("%s Resets at %s.", body, window.GetResetsAt())
			}
			events = append(events, event{
				kind: eventExhausted,
				payload: Payload{
					Title:      fmt.Sprintf("%s: quota exhausted", providerName),
					Body:       body,
					Tag:        fmt.Sprintf("%s:%s:%s", providerID, label, eventExhausted),
					ProviderID: providerID,
					URL:        "/providers",
					Urgent:     true,
				},
			})
		case previous.GetUsedPercent() > 0 && window.GetUsedPercent() <= previous.GetUsedPercent()-resetDropThreshold:
			events = append(events, event{
				kind: eventReset,
				payload: Payload{
					Title:      fmt.Sprintf("%s: quota refreshed", providerName),
					Body:       fmt.Sprintf("%s has %.1f%% remaining.", label, 100-window.GetUsedPercent()),
					Tag:        fmt.Sprintf("%s:%s:%s", providerID, label, eventReset),
					ProviderID: providerID,
					URL:        "/providers",
				},
			})
		}
	}

	return events
}

func windowKey(window *aoyorouter.ProviderQuotaWindow) string {
	return fmt.Sprintf("%s|%d", window.GetName(), window.GetWindowMinutes())
}

func windowLabel(window *aoyorouter.ProviderQuotaWindow) string {
	if window.GetName() != "" {
		return window.GetName()
	}

	minutes := int(window.GetWindowMinutes())
	switch {
	case minutes <= 0:
		return "quota window"
	case minutes%1440 == 0:
		return fmt.Sprintf("%dd window", minutes/1440)
	case minutes%60 == 0:
		return fmt.Sprintf("%dh window", minutes/60)
	default:
		return fmt.Sprintf("%dm window", minutes)
	}
}
