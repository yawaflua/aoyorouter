package apikey_repo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres"
	"github.com/yawaflua/aoyorouter/internal/models"
)

type ApiKeyRepo struct {
	DB *postgres.DB
}

func NewApiKeyRepo(db *postgres.DB) *ApiKeyRepo {
	return &ApiKeyRepo{DB: db}
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

	sql, args, err := squirrel.Select("id", "name", "key", "created_at", "updated_at", "is_deleted", "is_active").
		From("api_keys").
		Where(squirrel.Eq{"id": id}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("postgres.apikey_repo.GetApiKeyByID: %w", err)
	}
	var apiKey models.ApiKey
	if err := conn.QueryRow(ctx, sql, args...).Scan(&apiKey.ID, &apiKey.Name, &apiKey.Key, &apiKey.CreatedAt, &apiKey.UpdatedAt, &apiKey.IsDeleted, &apiKey.IsActive); err != nil {
		return nil, fmt.Errorf("postgres.apikey_repo.GetApiKeyByID: %w", err)
	}
	return &apiKey, nil
}

func (r *ApiKeyRepo) GetApiKeys(ctx context.Context) ([]*models.ApiKey, error) {
	conn := r.DB.GetConnection(ctx)
	rows, err := conn.Query(ctx, "SELECT id, name, key, created_at, updated_at, is_deleted, is_active FROM api_keys WHERE is_deleted = false")
	if err != nil {
		return nil, fmt.Errorf("postgres.apikey_repo.GetApiKeys: %w", err)
	}
	defer rows.Close()

	var apiKeys []*models.ApiKey
	for rows.Next() {
		var apiKey models.ApiKey
		if err := rows.Scan(&apiKey.ID, &apiKey.Name, &apiKey.Key, &apiKey.CreatedAt, &apiKey.UpdatedAt, &apiKey.IsDeleted, &apiKey.IsActive); err != nil {
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
