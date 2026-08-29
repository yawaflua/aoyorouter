package push

import (
	"context"
	"strings"
	"testing"

	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
)

func window(name string, minutes int32, used float64, resetsAt string) *aoyorouter.ProviderQuotaWindow {
	return &aoyorouter.ProviderQuotaWindow{
		Name:          name,
		UsedPercent:   used,
		ResetsAt:      resetsAt,
		WindowMinutes: minutes,
	}
}

func quota(windows ...*aoyorouter.ProviderQuotaWindow) *aoyorouter.ProviderQuota {
	return &aoyorouter.ProviderQuota{Quotas: windows}
}

func TestDiffQuota(t *testing.T) {
	tests := []struct {
		name      string
		prev      *aoyorouter.ProviderQuota
		next      *aoyorouter.ProviderQuota
		wantKinds []string
		wantTags  []string
		bodyPart  string
	}{
		{
			name:      "first observation is silent",
			prev:      nil,
			next:      quota(window("weekly", 10080, 100, "2026-09-01T00:00:00Z")),
			wantKinds: nil,
		},
		{
			name:      "exhaustion transition",
			prev:      quota(window("weekly", 10080, 82.5, "")),
			next:      quota(window("weekly", 10080, 100, "2026-09-01T00:00:00Z")),
			wantKinds: []string{eventExhausted},
			wantTags:  []string{"p1:weekly:exhausted"},
			bodyPart:  "Resets at 2026-09-01T00:00:00Z.",
		},
		{
			name:      "reset transition",
			prev:      quota(window("", 300, 100, "")),
			next:      quota(window("", 300, 12.25, "")),
			wantKinds: []string{eventReset},
			wantTags:  []string{"p1:5h window:reset"},
			bodyPart:  "87.8% remaining",
		},
		{
			name:      "no change produces nothing",
			prev:      quota(window("weekly", 10080, 42, "")),
			next:      quota(window("weekly", 10080, 42.5, "")),
			wantKinds: nil,
		},
		{
			name:      "already exhausted does not repeat",
			prev:      quota(window("weekly", 10080, 100, "")),
			next:      quota(window("weekly", 10080, 100, "")),
			wantKinds: nil,
		},
		{
			name:      "unmatched window is ignored",
			prev:      quota(window("weekly", 10080, 10, "")),
			next:      quota(window("monthly", 43200, 100, "")),
			wantKinds: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := diffQuota(tt.prev, tt.next, "Anthropic", "p1")
			if len(events) != len(tt.wantKinds) {
				t.Fatalf("got %d events, want %d: %+v", len(events), len(tt.wantKinds), events)
			}
			for i, kind := range tt.wantKinds {
				if events[i].kind != kind {
					t.Fatalf("event %d kind = %q, want %q", i, events[i].kind, kind)
				}
				if events[i].payload.Tag != tt.wantTags[i] {
					t.Fatalf("event %d tag = %q, want %q", i, events[i].payload.Tag, tt.wantTags[i])
				}
				if events[i].payload.ProviderID != "p1" {
					t.Fatalf("event %d provider id = %q", i, events[i].payload.ProviderID)
				}
				if !strings.Contains(events[i].payload.Title, "Anthropic") {
					t.Fatalf("event %d title = %q", i, events[i].payload.Title)
				}
			}
			if tt.bodyPart != "" && !strings.Contains(events[0].payload.Body, tt.bodyPart) {
				t.Fatalf("body = %q, want to contain %q", events[0].payload.Body, tt.bodyPart)
			}
		})
	}
}

func TestObserveFirstPassIsSilent(t *testing.T) {
	w := NewQuotaWatcher(nil, nil, nil)
	w.Observe(context.Background(), "p1", "Anthropic", quota(window("weekly", 10080, 100, "")))

	if _, ok := w.previous.Load("p1"); !ok {
		t.Fatal("expected snapshot to be stored on first observation")
	}
}

func TestWindowLabel(t *testing.T) {
	tests := []struct {
		window *aoyorouter.ProviderQuotaWindow
		want   string
	}{
		{window("weekly", 10080, 0, ""), "weekly"},
		{window("", 300, 0, ""), "5h window"},
		{window("", 1440, 0, ""), "1d window"},
		{window("", 45, 0, ""), "45m window"},
		{window("", 0, 0, ""), "quota window"},
	}

	for _, tt := range tests {
		if got := windowLabel(tt.window); got != tt.want {
			t.Fatalf("windowLabel = %q, want %q", got, tt.want)
		}
	}
}
