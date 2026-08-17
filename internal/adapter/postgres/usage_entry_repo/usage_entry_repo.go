package usage_entry_repo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/apikey_repo"
	"github.com/yawaflua/aoyorouter/internal/models"
)

type UsageEntryRepo struct {
	DB         *postgres.DB
	ApiKeyRepo *apikey_repo.ApiKeyRepo
	queue      chan *models.UsageEntry
	log        *slog.Logger
}

func NewUsageEntryRepo(db *postgres.DB, apiKeyRepo *apikey_repo.ApiKeyRepo, log *slog.Logger) *UsageEntryRepo {
	return &UsageEntryRepo{
		DB:         db,
		ApiKeyRepo: apiKeyRepo,
		queue:      make(chan *models.UsageEntry, 1000),
		log:        log,
	}
}

func (r *UsageEntryRepo) AddToQueue(ctx context.Context, usageEntry *models.UsageEntry) error {
	select {
	case r.queue <- usageEntry:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return errors.New("usage entry queue is full")
	}
}

func (r *UsageEntryRepo) ProcessQueue(ctx context.Context) {
	r.log.Info("ApiKeyRepo processer registred")
	for {
		select {
		case usageEntry := <-r.queue:
			r.log.Debug("Processing queue for usageentry")
			err := r.SaveUsageEntry(ctx, usageEntry)
			if err != nil {
				r.log.Error("failed to save usage entry", slog.Any("err", err))
			}
		case <-ctx.Done():
			return
		}
	}
}

func (r *UsageEntryRepo) SaveUsageEntry(ctx context.Context, usageEntry *models.UsageEntry) error {
	conn := r.DB.GetConnection(ctx)
	sql, args, err := squirrel.Insert("usage_entries").
		Columns("api_token", "provider", "latency", "input_tokens", "output_tokens", "total_tokens", "cached_tokens", "model", "reasoning", "failed", "error", "requested_at", "created_at").
		Values(usageEntry.ApiTokenID, usageEntry.Provider, usageEntry.Latency, usageEntry.InputTokens, usageEntry.OutputTokens, usageEntry.TotalTokens, usageEntry.CachedTokens, usageEntry.Model, usageEntry.Reasoning, usageEntry.Failed, usageEntry.Error, usageEntry.RequestedAt, usageEntry.CreatedAt).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("UsageEntryRepo.SaveUsageEntry: %w", err)
	}

	if _, err := conn.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("UsageEntryRepo.SaveUsageEntry: %w", err)
	}

	return nil
}

func (r *UsageEntryRepo) GetAllUsageEntries(ctx context.Context, limit uint64, offset uint64) ([]*models.UsageEntry, error) {
	conn := r.DB.GetConnection(ctx)

	sql, args, err := squirrel.Select("*").
		From("usage_entries").
		OrderBy("created_at DESC").
		Limit(limit).
		Offset(offset).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()

	if err != nil {
		return nil, fmt.Errorf("UsageEntryRepo.GetAllUsageEntries: %w", err)
	}

	rows, err := conn.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("UsageEntryRepo.GetAllUsageEntries: %w", err)
	}
	defer rows.Close()

	var entries []*models.UsageEntry
	for rows.Next() {
		var entry models.UsageEntry
		if err := rows.Scan(&entry.ID, &entry.ApiTokenID, &entry.Provider, &entry.Latency, &entry.InputTokens, &entry.OutputTokens, &entry.TotalTokens, &entry.CachedTokens, &entry.Model, &entry.Reasoning, &entry.Failed, &entry.Error, &entry.RequestedAt, &entry.CreatedAt); err != nil {
			return nil, fmt.Errorf("UsageEntryRepo.GetAllUsageEntries: %w", err)
		}
		entries = append(entries, &entry)
	}

	return entries, nil
}

func (r *UsageEntryRepo) GetUsageEntryByApiKeyID(ctx context.Context, id uuid.UUID) ([]*models.UsageEntry, error) {
	conn := r.DB.GetConnection(ctx)

	sql, args, err := squirrel.Select("*").
		From("usage_entries").
		Where(squirrel.Eq{"api_token": id}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("UsageEntryRepo.GetUsageEntryByApiKeyID: %w", err)
	}

	rows, err := conn.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("UsageEntryRepo.GetUsageEntryByApiKeyID: %w", err)
	}

	defer rows.Close()

	var entries []*models.UsageEntry
	for rows.Next() {
		var entry models.UsageEntry
		if err := rows.Scan(&entry.ID, &entry.ApiTokenID, &entry.Provider, &entry.Latency, &entry.InputTokens, &entry.OutputTokens, &entry.TotalTokens, &entry.CachedTokens, &entry.Model, &entry.Reasoning, &entry.Failed, &entry.Error, &entry.RequestedAt, &entry.CreatedAt); err != nil {
			return nil, fmt.Errorf("UsageEntryRepo.GetUsageEntryByApiKeyID: %w", err)
		}
		entries = append(entries, &entry)
	}

	return entries, nil
}

func (r *UsageEntryRepo) SumUsageEntries(ctx context.Context, apiKeyID uuid.UUID) (*models.UsageEntry, error) {
	conn := r.DB.GetConnection(ctx)
	sql, args, err := squirrel.Select("SUM(input_tokens) as input_tokens, SUM(output_tokens) as output_tokens, SUM(total_tokens) as total_tokens, SUM(cached_tokens) as cached_tokens").
		From("usage_entries").
		Where(squirrel.Eq{"api_token": apiKeyID}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("UsageEntryRepo.SumUsageEntries: %w", err)
	}

	rows, err := conn.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("UsageEntryRepo.SumUsageEntries: %w", err)
	}

	defer rows.Close()

	var entry models.UsageEntry
	if rows.Next() {
		if err := rows.Scan(&entry.InputTokens, &entry.OutputTokens, &entry.TotalTokens, &entry.CachedTokens); err != nil {
			return nil, fmt.Errorf("UsageEntryRepo.SumUsageEntries: %w", err)
		}
	}

	return &entry, nil
}
