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

type ApiKeyRepo struct {
	DB    *postgres.DB
	queue chan *models.ApiKey
	log   *slog.Logger
}

func NewApiKeyRepo(db *postgres.DB, log *slog.Logger) *ApiKeyRepo {
	return &ApiKeyRepo{DB: db, log: log, queue: make(chan *models.ApiKey, 1000)}
}

func (r *ApiKeyRepo) AddToQueue(
	ctx context.Context,
	apiKey *models.ApiKey,
) error {
	select {
	case r.queue <- apiKey:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return errors.New("api key queue is full")
	}
}

func (r *ApiKeyRepo) ProcessQueue(ctx context.Context) {
	for {
		select {
		case usageEntry, ok := <-r.queue:
			if !ok {
				return
			}
			err := r.UpdateApiKeyQuota(ctx, usageEntry)
			if err != nil {
				r.log.Error("failed to save usage entry", slog.Any("err", err))
			}
		case <-ctx.Done():
			return
		}
	}
}

func (r *ApiKeyRepo) CreateApiKey(ctx context.Context, name string) (*models.ApiKey, error) {
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
		Columns("ID", "name", "key", "created_at", "updated_at", "is_deleted", "is_active").
		Values(id, name, token, time.Now(), time.Now(), false, true).
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

func (r *ApiKeyRepo) GetApiKeyByID(ctx context.Context, id string) (*models.ApiKey, error) {
	conn := r.DB.GetConnection(ctx)

	sql, args, err := squirrel.Select("id", "name", "key", "quota_setted", "quota_tokens", "quota_period", "quota_reset_at", "reserved_tokens", "created_at", "updated_at", "is_deleted", "is_active").
		From("api_keys").
		Where(squirrel.Eq{"id": id}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("postgres.apikey_repo.GetApiKeyByID: %w", err)
	}
	var apiKey models.ApiKey
	if err := conn.QueryRow(ctx, sql, args...).Scan(&apiKey.ID, &apiKey.Name, &apiKey.Key, &apiKey.QuotaSetted, &apiKey.QuotaTokens, &apiKey.QuotaPeriod, &apiKey.QuotaResetAt, &apiKey.ReservedTokens, &apiKey.CreatedAt, &apiKey.UpdatedAt, &apiKey.IsDeleted, &apiKey.IsActive); err != nil {
		return nil, fmt.Errorf("postgres.apikey_repo.GetApiKeyByID: %w", err)
	}
	return &apiKey, nil
}

func (r *ApiKeyRepo) GetApiKeyByKey(ctx context.Context, key string) (*models.ApiKey, error) {
	conn := r.DB.GetConnection(ctx)
	sql, args, err := squirrel.Select("id", "name", "key", "quota_setted", "quota_tokens", "quota_period", "quota_reset_at", "reserved_tokens", "created_at", "updated_at", "is_deleted", "is_active").
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
	if rows.Next() {
		if err := rows.Scan(&apiKey.ID, &apiKey.Name, &apiKey.Key, &apiKey.QuotaSetted, &apiKey.QuotaTokens, &apiKey.QuotaPeriod, &apiKey.QuotaResetAt, &apiKey.ReservedTokens, &apiKey.CreatedAt, &apiKey.UpdatedAt, &apiKey.IsDeleted, &apiKey.IsActive); err != nil {
			return nil, fmt.Errorf("postgres.apikey_repo.GetApiKeyByKey: %w", err)
		}
	}
	return &apiKey, nil
}

func (r *ApiKeyRepo) UpdateApiKeyQuota(ctx context.Context, apiKey *models.ApiKey) error {
	conn := r.DB.GetConnection(ctx)
	if apiKey.QuotaSetted && apiKey.QuotaResetAt.Before(time.Now().UTC()) {

		var newQuotaResetAt time.Time

		switch apiKey.QuotaPeriod {
		case models.QuotaPeriodMonth:
			newQuotaResetAt = apiKey.QuotaResetAt.Add(30 * 24 * time.Hour)
		case models.QuotaPeriodWeek:
			newQuotaResetAt = apiKey.QuotaResetAt.Add(7 * 24 * time.Hour)
		case models.QuotaPeriodDay:
			newQuotaResetAt = apiKey.QuotaResetAt.Add(24 * time.Hour)
		case models.QuotaPeriodHour:
			newQuotaResetAt = apiKey.QuotaResetAt.Add(1 * time.Hour)
		case models.QuotaPeriodMinute:
			newQuotaResetAt = apiKey.QuotaResetAt.Add(1 * time.Minute)
		default:
			fmt.Println("Forever")
		}
		apiKey.QuotaResetAt = newQuotaResetAt

		apiKey.QuotaTokens = 0
	}

	sql, args, err := squirrel.Update("api_keys").
		Set("quota_tokens", apiKey.QuotaTokens).
		Set("quota_period", apiKey.QuotaPeriod).
		Set("quota_reset_at", apiKey.QuotaResetAt).
		Set("reserved_tokens", apiKey.ReservedTokens).
		Set("updated_at", time.Now()).
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
		Set("quota_setted", apiKey.QuotaSetted).
		Set("quota_period", apiKey.QuotaPeriod).
		Set("quota_reset_at", apiKey.QuotaResetAt).
		Set("reserved_tokens", apiKey.ReservedTokens).
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
	rows, err := conn.Query(ctx, "SELECT id, name, key, quota_setted, quota_tokens, quota_period, quota_reset_at, reserved_tokens, created_at, updated_at, is_deleted, is_active FROM api_keys WHERE is_deleted = false")
	if err != nil {
		return nil, fmt.Errorf("postgres.apikey_repo.GetApiKeys: %w", err)
	}
	defer rows.Close()

	var apiKeys []*models.ApiKey
	for rows.Next() {
		var apiKey models.ApiKey
		if err := rows.Scan(&apiKey.ID, &apiKey.Name, &apiKey.Key, &apiKey.QuotaSetted, &apiKey.QuotaTokens, &apiKey.QuotaPeriod, &apiKey.QuotaResetAt, &apiKey.ReservedTokens, &apiKey.CreatedAt, &apiKey.UpdatedAt, &apiKey.IsDeleted, &apiKey.IsActive); err != nil {
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
	sql, args, err := squirrel.Delete("api_keys").
		Where(squirrel.Eq{"id": id}).
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
