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

func (r *ProviderRepo) UpdateProxy(ctx context.Context, id, proxy string, use_proxy bool, is_cloudflare bool) error {
	conn := r.DB.GetConnection(ctx)
	sql, args, err := squirrel.Update("providers").
		Set("proxy", proxy).
		Set("use_proxy", use_proxy).
		Set("is_cloudflare", is_cloudflare).
		Where(squirrel.Eq{"id": id}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("postgres.provider_repo.UpdateProxy: %w", err)
	}
	if _, err = conn.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("postgres.provider_repo.UpdateProxy: %w", err)
	}
	return nil
}

func (r *ProviderRepo) CreateProvider(ctx context.Context, name string, providerType int32, clientID, clientSecret string, useProxy bool, proxy string, isCloudflare bool) (*models.Provider, error) {
	id := uuid.NewString()
	now := time.Now()
	sql, args, err := squirrel.Insert("providers").
		Columns("id", "name", "type", "client_id", "client_secret", "use_proxy", "proxy", "is_cloudflare", "created_at", "updated_at").
		Values(id, name, providerType, clientID, clientSecret, useProxy, proxy, isCloudflare, now, now).
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

	sql, args, err := squirrel.Select("id", "name", "type", "client_id", "client_secret", "credentials", "use_proxy", "proxy", "is_cloudflare", "created_at", "updated_at").
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
	sql, args, err := squirrel.Select("id", "name", "type", "client_id", "client_secret", "credentials", "use_proxy", "proxy", "is_cloudflare", "created_at", "updated_at").
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
		if err := scanProvider(rows, &provider); err != nil {
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
	sql, args, err := squirrel.Select("id", "name", "type", "client_id", "client_secret", "credentials", "use_proxy", "proxy", "is_cloudflare", "created_at", "updated_at").
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

func (r *ProviderRepo) UpdateProvider(ctx context.Context, id, name string, providerType int32, clientID, clientSecret string, useProxy bool, proxy string, isCloudflare bool) (*models.Provider, error) {
	conn := r.DB.GetConnection(ctx)
	sql, args, err := squirrel.Update("providers").
		Set("name", name).
		Set("type", providerType).
		Set("client_id", clientID).
		Set("client_secret", clientSecret).
		Set("use_proxy", useProxy).
		Set("proxy", proxy).
		Set("is_cloudflare", isCloudflare).
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

func (r *ProviderRepo) UpdateProviderCredentials(ctx context.Context, id, clientSecret string, credentials map[string]any) (*models.Provider, error) {
	if credentials == nil {
		credentials = map[string]any{}
	}
	sql, args, err := squirrel.Update("providers").
		Set("client_secret", clientSecret).
		Set("credentials", credentials).
		Set("updated_at", time.Now()).
		Where(squirrel.Eq{"id": id}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("postgres.provider_repo.UpdateProviderCredentials: %w", err)
	}
	if _, err = r.DB.GetConnection(ctx).Exec(ctx, sql, args...); err != nil {
		return nil, fmt.Errorf("postgres.provider_repo.UpdateProviderCredentials: %w", err)
	}
	return r.GetProvider(ctx, id)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanProvider(row rowScanner, provider *models.Provider) error {
	return row.Scan(&provider.ID, &provider.Name, &provider.Type, &provider.BaseUrl, &provider.ClientSecret, &provider.Credentials, &provider.UseProxy, &provider.Proxy, &provider.IsCloudflare, &provider.CreatedAt, &provider.UpdatedAt)
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
