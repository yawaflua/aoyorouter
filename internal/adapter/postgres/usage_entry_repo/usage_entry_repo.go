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
	// The `default` arm makes the send non-blocking, which made the ctx.Done
	// arm unreachable. Keep the non-blocking behaviour — a full queue must not
	// stall the request path — but make the drop loud instead of silent.
	select {
	case r.queue <- usageEntry:
		return nil
	default:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	r.log.Error("usage entry dropped: queue is full",
		slog.String("api_token", usageEntry.ApiTokenID.String()),
		slog.Int("queue_cap", cap(r.queue)))
	return errors.New("usage entry queue is full")
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

// Default and hard cap for paged reads, so a caller passing 0 (or a very large
// number) cannot turn into "return nothing" or "return the whole table".
const (
	defaultUsageLimit uint64 = 100
	maxUsageLimit     uint64 = 1000
)

// usageEntryColumns is listed explicitly and in the exact order the Scan calls
// below expect. The previous `SELECT *` relied on physical column order, which
// silently swapped created_at and requested_at in one of the two readers.
// COALESCE keeps the nullable `error` column scannable into a plain string.
var usageEntryColumns = []string{
	"id", "api_token", "provider", "latency",
	"input_tokens", "output_tokens", "total_tokens", "cached_tokens",
	"model", "reasoning", "failed", "COALESCE(error, '') AS error",
	"created_at", "requested_at",
}

func scanUsageEntry(rows interface{ Scan(...any) error }, entry *models.UsageEntry) error {
	return rows.Scan(
		&entry.ID, &entry.ApiTokenID, &entry.Provider, &entry.Latency,
		&entry.InputTokens, &entry.OutputTokens, &entry.TotalTokens, &entry.CachedTokens,
		&entry.Model, &entry.Reasoning, &entry.Failed, &entry.Error,
		&entry.CreatedAt, &entry.RequestedAt,
	)
}

func clampLimit(limit uint64) uint64 {
	if limit == 0 {
		return defaultUsageLimit
	}
	if limit > maxUsageLimit {
		return maxUsageLimit
	}
	return limit
}

func (r *UsageEntryRepo) GetAllUsageEntries(ctx context.Context, limit uint64, offset uint64) ([]*models.UsageEntry, error) {
	return r.queryUsageEntries(ctx, "GetAllUsageEntries", nil, limit, offset)
}

// GetUsageEntriesByApiKeyID returns one key's usage, newest first. Paging is
// applied in SQL alongside the ownership filter, so callers never have to
// post-filter a page and end up with fewer rows than they asked for.
func (r *UsageEntryRepo) GetUsageEntriesByApiKeyID(ctx context.Context, id uuid.UUID, limit, offset uint64) ([]*models.UsageEntry, error) {
	return r.queryUsageEntries(ctx, "GetUsageEntriesByApiKeyID", &id, limit, offset)
}

func (r *UsageEntryRepo) queryUsageEntries(ctx context.Context, op string, apiKeyID *uuid.UUID, limit, offset uint64) ([]*models.UsageEntry, error) {
	conn := r.DB.GetConnection(ctx)

	q := squirrel.Select(usageEntryColumns...).
		From("usage_entries").
		OrderBy("created_at DESC").
		Limit(clampLimit(limit)).
		Offset(offset).
		PlaceholderFormat(squirrel.Dollar)

	if apiKeyID != nil {
		q = q.Where(squirrel.Eq{"api_token": *apiKeyID})
	}

	sql, args, err := q.ToSql()
	if err != nil {
		return nil, fmt.Errorf("UsageEntryRepo.%s: %w", op, err)
	}

	rows, err := conn.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("UsageEntryRepo.%s: %w", op, err)
	}
	defer rows.Close()

	var entries []*models.UsageEntry
	for rows.Next() {
		var entry models.UsageEntry
		if err := scanUsageEntry(rows, &entry); err != nil {
			return nil, fmt.Errorf("UsageEntryRepo.%s: %w", op, err)
		}
		entries = append(entries, &entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("UsageEntryRepo.%s: %w", op, err)
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("UsageEntryRepo.SumUsageEntries: %w", err)
	}

	return &entry, nil
}
