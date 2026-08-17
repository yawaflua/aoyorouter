package cliproxyapi

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/apikey_repo"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/usage_entry_repo"
	"github.com/yawaflua/aoyorouter/internal/models"
)

type UsagePlugin struct {
	APIKeysRepo *apikey_repo.ApiKeyRepo
	UsageRepo   *usage_entry_repo.UsageEntryRepo
	Logger      *slog.Logger
}

func NewUsagePlugin(apikey_repo *apikey_repo.ApiKeyRepo, usage_repo *usage_entry_repo.UsageEntryRepo, logger *slog.Logger) *UsagePlugin {
	return &UsagePlugin{
		APIKeysRepo: apikey_repo,
		UsageRepo:   usage_repo,
		Logger:      logger,
	}
}

// HandleUsage implements [usage.Plugin].
func (u *UsagePlugin) HandleUsage(ctx context.Context, record usage.Record) {
	key, err := u.APIKeysRepo.GetApiKeyByID(ctx, record.APIKey)
	if err != nil {
		u.Logger.Error("HandleUsage caused error:", slog.Any("err", err))
		return
	}

	usageEntry := &models.UsageEntry{
		ID:           uuid.New(),
		ApiTokenID:   uuid.MustParse(key.ID),
		Provider:     record.Provider,
		Model:        record.Model,
		InputTokens:  record.Detail.InputTokens,
		OutputTokens: record.Detail.OutputTokens,
		TotalTokens:  record.Detail.TotalTokens,
		CachedTokens: record.Detail.CachedTokens,
		RequestedAt:  record.RequestedAt.UTC(),
		CreatedAt:    time.Now().UTC(),
		Latency:      int(record.Latency),
		Reasoning:    record.ReasoningEffort,
		Failed:       record.Failed,
	}
	if !record.Failed {
		usageEntry.Error = fmt.Sprintf("%d %s", record.Fail.StatusCode, record.Fail.Body)
	}
	key.QuotaTokens += record.Detail.TotalTokens
	go func() {
		err = u.APIKeysRepo.UpdateApiKeyQuota(ctx, key)
		if err != nil {
			u.Logger.Error("HandleUsage caused error:", slog.Any("err", err))
			return
		}
	}()
	go func() {
		model, err := u.UsageRepo.SaveUsageEntry(ctx, usageEntry)
		if err != nil {
			u.Logger.Error("HandleUsage caused error:", slog.Any("err", err))
			return
		}
		u.Logger.Debug("Saved usage entry", slog.Any("model", model))
	}()
}
