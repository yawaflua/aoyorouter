package apikey_repo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres"
	"github.com/yawaflua/aoyorouter/internal/models"
)

// ErrApiKeyNotFound is returned when a lookup matches no row. Callers must be
// able to tell "no such key" apart from "here is a zero-valued key".
var ErrApiKeyNotFound = errors.New("api key not found")

// QuotaUsage is a delta, not a snapshot. Queueing the whole ApiKey meant the
// consumer wrote back a quota_tokens value read before the request ran, so
// concurrent requests overwrote each other's accounting.
type QuotaUsage struct {
	ApiKeyID string
	Tokens   int64
}

type ApiKeyRepo struct {
	DB    *postgres.DB
	queue chan QuotaUsage
	log   *slog.Logger
}

func NewApiKeyRepo(db *postgres.DB, log *slog.Logger) *ApiKeyRepo {
	return &ApiKeyRepo{DB: db, log: log, queue: make(chan QuotaUsage, 1000)}
}

func (r *ApiKeyRepo) AddToQueue(
	ctx context.Context,
	usage QuotaUsage,
) error {
	select {
	case r.queue <- usage:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *ApiKeyRepo) ProcessQueue(ctx context.Context) {
	r.log.Info("ApiKeyRepo processer registred")
	for {
		select {
		case usage := <-r.queue:
			r.log.Debug("Processing queue for apikeyrepo")
			if err := r.AddApiKeyQuotaUsage(ctx, usage.ApiKeyID, usage.Tokens); err != nil {
				r.log.Error("failed to record quota usage", slog.Any("err", err))
			}
		case <-ctx.Done():
			return
		}
	}
}

// AddApiKeyQuotaUsage increments the consumed quota in a single statement so
// concurrent requests accumulate instead of clobbering one another.
func (r *ApiKeyRepo) AddApiKeyQuotaUsage(ctx context.Context, id string, tokens int64) error {
	if tokens == 0 {
		return nil
	}
	conn := r.DB.GetConnection(ctx)
	sql, args, err := squirrel.Update("api_keys").
		Set("quota_tokens", squirrel.Expr("quota_tokens + ?", tokens)).
		Set("updated_at", time.Now().UTC()).
		Where(squirrel.Eq{"id": id}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("postgres.apikey_repo.AddApiKeyQuotaUsage: %w", err)
	}
	if _, err := conn.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("postgres.apikey_repo.AddApiKeyQuotaUsage: %w", err)
	}
	return nil
}

func (r *ApiKeyRepo) CreateApiKey(ctx context.Context, name string, is_admin bool) (*models.ApiKey, error) {
	conn := r.DB.GetConnection(ctx)

	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("postgres.apikey_repo.CreateApiKey: %w", err)
	}
	id, err := uuid.NewUUID()
	if err != nil {
		return nil, fmt.Errorf("postgres.apikey_repo.CreateApiKey: %w", err)
	}

	sql, args, err := squirrel.Insert("api_keys").
		Columns("ID", "name", "key", "created_at", "updated_at", "is_deleted", "is_active", "is_admin").
		Values(id, name, token, time.Now(), time.Now(), false, true, is_admin).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()

	if err != nil {
		return nil, fmt.Errorf("postgres.apikey_repo.CreateApiKey: %w", err)
	}

	_, err = conn.Exec(ctx, sql, args...)

	if err != nil {
		return nil, fmt.Errorf("postgres.apikey_repo.CreateApiKey: %w", err)
	}
	return &models.ApiKey{
		ID:        id.String(),
		Name:      name,
		Key:       token,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

func (r *ApiKeyRepo) RecreateApiKey(ctx context.Context, id string) (string, error) {
	conn := r.DB.GetConnection(ctx)
	newKey, err := generateToken()
	if err != nil {
		return "", fmt.Errorf("postgres.apikey_repo.RecreateApiKey: %w", err)
	}
	sql, args, err := squirrel.Update("api_keys").
		Set("updated_at", time.Now()).
		Set("key", newKey).
		Where(squirrel.Eq{"id": id}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return "", fmt.Errorf("postgres.apikey_repo.RecreateApiKey: %w", err)
	}
	_, err = conn.Exec(ctx, sql, args...)
	if err != nil {
		return "", fmt.Errorf("postgres.apikey_repo.RecreateApiKey: %w", err)
	}
	return newKey, nil
}

func (r *ApiKeyRepo) GetApiKeyByID(ctx context.Context, id string) (*models.ApiKey, error) {
	conn := r.DB.GetConnection(ctx)

	sql, args, err := squirrel.Select("id", "name", "key", "quota_setted", "quota_tokens", "quota_period", "quota_reset_at", "reserved_tokens", "excluded_providers", "excluded_models", "created_at", "updated_at", "is_deleted", "is_active", "is_admin").
		From("api_keys").
		Where(squirrel.Eq{"id": id}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("postgres.apikey_repo.GetApiKeyByID: %w", err)
	}
	var apiKey models.ApiKey
	if err := conn.QueryRow(ctx, sql, args...).Scan(&apiKey.ID, &apiKey.Name, &apiKey.Key, &apiKey.QuotaSetted, &apiKey.QuotaTokens, &apiKey.QuotaPeriod, &apiKey.QuotaResetAt, &apiKey.ReservedTokens, &apiKey.RestrictedProviders, &apiKey.RestrictedModels, &apiKey.CreatedAt, &apiKey.UpdatedAt, &apiKey.IsDeleted, &apiKey.IsActive, &apiKey.IsAdmin); err != nil {
		return nil, fmt.Errorf("postgres.apikey_repo.GetApiKeyByID: %w", err)
	}
	return &apiKey, nil
}

func (r *ApiKeyRepo) GetApiKeyByKey(ctx context.Context, key string) (*models.ApiKey, error) {
	conn := r.DB.GetConnection(ctx)
	sql, args, err := squirrel.Select("id", "name", "key", "quota_setted", "quota_tokens", "quota_period", "quota_reset_at", "reserved_tokens", "excluded_providers", "excluded_models", "created_at", "updated_at", "is_deleted", "is_active", "is_admin").
		From("api_keys").
		Where(squirrel.Eq{"key": key}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("postgres.apikey_repo.GetApiKeyByKey: %w", err)
	}
	rows, err := conn.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres.apikey_repo.GetApiKeyByKey: %w", err)
	}
	defer rows.Close()

	var apiKey models.ApiKey
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("postgres.apikey_repo.GetApiKeyByKey: %w", err)
		}
		return nil, ErrApiKeyNotFound
	}
	if err := rows.Scan(&apiKey.ID, &apiKey.Name, &apiKey.Key, &apiKey.QuotaSetted, &apiKey.QuotaTokens, &apiKey.QuotaPeriod, &apiKey.QuotaResetAt, &apiKey.ReservedTokens, &apiKey.RestrictedProviders, &apiKey.RestrictedModels, &apiKey.CreatedAt, &apiKey.UpdatedAt, &apiKey.IsDeleted, &apiKey.IsActive, &apiKey.IsAdmin); err != nil {
		return nil, fmt.Errorf("postgres.apikey_repo.GetApiKeyByKey: %w", err)
	}
	return &apiKey, nil
}

func (r *ApiKeyRepo) UpdateApiKeyQuota(ctx context.Context, apiKey *models.ApiKey) error {
	conn := r.DB.GetConnection(ctx)
	if apiKey.QuotaSetted && apiKey.QuotaResetAt.Before(time.Now().UTC()) {

		var newQuotaResetAt time.Time

		newQuotaResetAt = apiKey.QuotaResetAt.Add(apiKey.QuotaPeriod.ToDuration())
		apiKey.QuotaResetAt = newQuotaResetAt

		apiKey.QuotaTokens = 0
	}

	sql, args, err := squirrel.Update("api_keys").
		Set("quota_tokens", apiKey.QuotaTokens).
		Set("quota_period", apiKey.QuotaPeriod).
		Set("quota_reset_at", apiKey.QuotaResetAt).
		Set("reserved_tokens", apiKey.ReservedTokens).
		Set("updated_at", time.Now().UTC()).
		Where(squirrel.Eq{"id": apiKey.ID}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("postgres.apikey_repo.UpdateApiKeyQuota: %w", err)
	}
	_, err = conn.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("postgres.apikey_repo.UpdateApiKeyQuota: %w", err)
	}
	return nil
}

func (r *ApiKeyRepo) UpdateApiKey(ctx context.Context, apiKey *models.ApiKey) error {
	conn := r.DB.GetConnection(ctx)
	sql, args, err := squirrel.Update("api_keys").
		Set("name", apiKey.Name).
		Set("is_active", apiKey.IsActive).
		Set("is_admin", apiKey.IsAdmin).
		Set("quota_setted", apiKey.QuotaSetted).
		Set("quota_period", apiKey.QuotaPeriod).
		Set("quota_reset_at", apiKey.QuotaResetAt).
		Set("reserved_tokens", apiKey.ReservedTokens).
		Set("excluded_providers", apiKey.RestrictedProviders).
		Set("excluded_models", apiKey.RestrictedModels).
		Set("updated_at", time.Now().UTC()).
		Where(squirrel.Eq{"id": apiKey.ID, "is_deleted": false}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("postgres.apikey_repo.UpdateApiKey: %w", err)
	}
	result, err := conn.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("postgres.apikey_repo.UpdateApiKey: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("postgres.apikey_repo.UpdateApiKey: api key not found")
	}
	return nil
}

func (r *ApiKeyRepo) GetApiKeys(ctx context.Context) ([]*models.ApiKey, error) {
	conn := r.DB.GetConnection(ctx)
	rows, err := conn.Query(ctx, "SELECT id, name, key, quota_setted, quota_tokens, quota_period, quota_reset_at, reserved_tokens, excluded_providers, excluded_models, created_at, updated_at, is_deleted, is_active, is_admin FROM api_keys WHERE is_deleted = false")
	if err != nil {
		return nil, fmt.Errorf("postgres.apikey_repo.GetApiKeys: %w", err)
	}
	defer rows.Close()

	var apiKeys []*models.ApiKey
	for rows.Next() {
		var apiKey models.ApiKey
		if err := rows.Scan(&apiKey.ID, &apiKey.Name, &apiKey.Key, &apiKey.QuotaSetted, &apiKey.QuotaTokens, &apiKey.QuotaPeriod, &apiKey.QuotaResetAt, &apiKey.ReservedTokens, &apiKey.RestrictedProviders, &apiKey.RestrictedModels, &apiKey.CreatedAt, &apiKey.UpdatedAt, &apiKey.IsDeleted, &apiKey.IsActive, &apiKey.IsAdmin); err != nil {
			return nil, fmt.Errorf("postgres.apikey_repo.GetApiKeys: %w", err)
		}
		apiKeys = append(apiKeys, &apiKey)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.apikey_repo.GetApiKeys: %w", err)
	}
	return apiKeys, nil
}

func (r *ApiKeyRepo) DeleteApiKey(ctx context.Context, id string) error {
	conn := r.DB.GetConnection(ctx)
	sql, args, err := squirrel.Update("api_keys").
		Where(squirrel.Eq{"id": id}).
		Set("is_deleted", true).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()

	if err != nil {
		return fmt.Errorf("postgres.apikey_repo.DeleteApiKey: %w", err)
	}
	_, err = conn.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("postgres.apikey_repo.DeleteApiKey: %w", err)
	}
	return nil
}

func generateToken() (string, error) {
	b := make([]byte, 15)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("postgres.apikey_repo.generateToken: %w", err)
	}
	s := hex.EncodeToString(b)
	return fmt.Sprintf(
		"sk-%s-%s-%s",
		s[:16],
		s[16:22],
		s[22:30],
	), nil
}
