package provider_repo

import (
	"context"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres"
	"github.com/yawaflua/aoyorouter/internal/models"
)

type ProviderRepo struct {
	DB *postgres.DB
}

func NewProviderRepo(db *postgres.DB) *ProviderRepo {
	return &ProviderRepo{DB: db}
}

func (r *ProviderRepo) CreateProvider(ctx context.Context, name string, providerType int32, clientID, clientSecret string) (*models.Provider, error) {
	id := uuid.NewString()
	now := time.Now()
	sql, args, err := squirrel.Insert("providers").
		Columns("id", "name", "type", "client_id", "client_secret", "created_at", "updated_at").
		Values(id, name, providerType, clientID, clientSecret, now, now).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("postgres.provider_repo.CreateProvider: %w", err)
	}
	if _, err = r.DB.GetConnection(ctx).Exec(ctx, sql, args...); err != nil {
		return nil, fmt.Errorf("postgres.provider_repo.CreateProvider: %w", err)
	}
	return r.GetProvider(ctx, id)
}

func (r *ProviderRepo) GetProvider(ctx context.Context, id string) (*models.Provider, error) {
	conn := r.DB.GetConnection(ctx)

	sql, args, err := squirrel.Select("id", "name", "type", "client_id", "client_secret", "created_at", "updated_at").
		From("providers").
		Where(squirrel.Eq{"id": id}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()

	if err != nil {
		return nil, fmt.Errorf("postgres.provider_repo.GetProvider: %w", err)
	}

	var provider models.Provider
	if err := scanProvider(conn.QueryRow(ctx, sql, args...), &provider); err != nil {
		return nil, fmt.Errorf("postgres.provider_repo.GetProvider: %w", err)
	}

	return &provider, nil
}

func (r *ProviderRepo) GetProviders(ctx context.Context) ([]*models.Provider, error) {
	conn := r.DB.GetConnection(ctx)
	sql, args, err := squirrel.Select("id", "name", "type", "client_id", "client_secret", "created_at", "updated_at").
		From("providers").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("postgres.provider_repo.GetProviders: %w", err)
	}

	rows, err := conn.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres.provider_repo.GetProviders: %w", err)
	}
	defer rows.Close()

	var providers []*models.Provider
	for rows.Next() {
		var provider models.Provider
		if err := rows.Scan(&provider.ID, &provider.Name, &provider.Type, &provider.ClientID, &provider.ClientSecret, &provider.CreatedAt, &provider.UpdatedAt); err != nil {
			return nil, fmt.Errorf("postgres.provider_repo.GetProviders: %w", err)
		}
		providers = append(providers, &provider)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.provider_repo.GetProviders: %w", err)
	}
	return providers, nil
}

func (r *ProviderRepo) GetProviderById(ctx context.Context, id string) (*models.Provider, error) {
	conn := r.DB.GetConnection(ctx)
	sql, args, err := squirrel.Select("id", "name", "type", "client_id", "client_secret", "created_at", "updated_at").
		From("providers").
		Where(squirrel.Eq{"id": id}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("postgres.provider_repo.GetProvider: %w", err)
	}

	var provider models.Provider
	if err := scanProvider(conn.QueryRow(ctx, sql, args...), &provider); err != nil {
		return nil, fmt.Errorf("postgres.provider_repo.GetProvider: %w", err)
	}
	return &provider, nil
}

func (r *ProviderRepo) UpdateProvider(ctx context.Context, id, name string, providerType int32, clientID, clientSecret string) (*models.Provider, error) {
	conn := r.DB.GetConnection(ctx)
	sql, args, err := squirrel.Update("providers").
		Set("name", name).
		Set("type", providerType).
		Set("client_id", clientID).
		Set("client_secret", clientSecret).
		Set("updated_at", time.Now()).
		Where(squirrel.Eq{"id": id}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("postgres.provider_repo.UpdateProvider: %w", err)
	}
	_, err = conn.Exec(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres.provider_repo.UpdateProvider: %w", err)
	}
	return r.GetProviderById(ctx, id)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanProvider(row rowScanner, provider *models.Provider) error {
	return row.Scan(&provider.ID, &provider.Name, &provider.Type, &provider.ClientID, &provider.ClientSecret, &provider.CreatedAt, &provider.UpdatedAt)
}

func (r *ProviderRepo) DeleteProvider(ctx context.Context, id string) error {
	conn := r.DB.GetConnection(ctx)
	sql, args, err := squirrel.Delete("providers").
		Where(squirrel.Eq{"id": id}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("postgres.provider_repo.DeleteProvider: %w", err)
	}
	_, err = conn.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("postgres.provider_repo.DeleteProvider: %w", err)
	}
	return nil
}
