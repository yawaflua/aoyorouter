package usage_entry_repo

import (
	"context"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/apikey_repo"
	"github.com/yawaflua/aoyorouter/internal/models"
)

type UsageEntryRepo struct {
	DB         *postgres.DB
	ApiKeyRepo *apikey_repo.ApiKeyRepo
}

func NewUsageEntryRepo(db *postgres.DB, apiKeyRepo *apikey_repo.ApiKeyRepo) *UsageEntryRepo {
	return &UsageEntryRepo{
		DB:         db,
		ApiKeyRepo: apiKeyRepo,
	}
}

func (r *UsageEntryRepo) SaveUsageEntry(ctx context.Context, usageEntry *models.UsageEntry) (*models.UsageEntry, error) {
	conn := r.DB.GetConnection(ctx)

	sql, args, err := squirrel.Insert("usage_entries").
		Columns("id", "api_token", "provider", "latency", "input_tokens", "output_tokens", "total_tokens", "cached_tokens", "model", "reasoning", "failed", "error", "requested_at", "created_at").
		Values(usageEntry.ID, usageEntry.ApiTokenID, usageEntry.Provider, usageEntry.Latency, usageEntry.InputTokens, usageEntry.OutputTokens, usageEntry.TotalTokens, usageEntry.CachedTokens, usageEntry.Model, usageEntry.Reasoning, usageEntry.Failed, usageEntry.Error, usageEntry.RequestedAt, usageEntry.CreatedAt).
		Suffix("RETURNING id, api_token, provider, latency, input_tokens, output_tokens, total_tokens, cached_tokens, model, reasoning, failed, error, requested_at, created_at").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("UsageEntryRepo.SaveUsageEntry: %w", err)
	}

	var result models.UsageEntry
	if err := conn.QueryRow(ctx, sql, args...).Scan(&result.ID, &result.ApiTokenID, &result.Provider, &result.Latency, &result.InputTokens, &result.OutputTokens, &result.TotalTokens, &result.CachedTokens, &result.Model, &result.Reasoning, &result.Failed, &result.Error, &result.RequestedAt, &result.CreatedAt); err != nil {
		return nil, fmt.Errorf("UsageEntryRepo.SaveUsageEntry: %w", err)
	}

	return &result, nil
}

func (r *UsageEntryRepo) GetAllUsageEntries(ctx context.Context, limit uint64, offset uint64) ([]*models.UsageEntry, error) {
	conn := r.DB.GetConnection(ctx)

	sql, args, err := squirrel.Select("*").
		From("usage_entries").
		OrderBy("created_at ASC").
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
