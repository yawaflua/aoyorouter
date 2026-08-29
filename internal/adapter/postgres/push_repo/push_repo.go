package push_repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres"
	"github.com/yawaflua/aoyorouter/internal/models"
)

type PushRepo struct {
	DB *postgres.DB
}

func NewPushRepo(db *postgres.DB) *PushRepo {
	return &PushRepo{DB: db}
}

const subscriptionColumns = "s.id, s.endpoint, s.p256dh, s.auth, s.expiration_time, s.user_agent, s.labels, s.created_at, s.updated_at"

func (r *PushRepo) Subscribe(ctx context.Context, subject string, sub *models.PushSubscription) (string, bool, error) {
	if err := sub.Validate(); err != nil {
		return "", false, fmt.Errorf("postgres.push_repo.Subscribe: %w", err)
	}

	labels := sub.Labels
	if labels == nil {
		labels = map[string]string{}
	}

	now := time.Now()
	conn := r.DB.GetConnection(ctx)

	sql, args, err := squirrel.Insert("push_subscriptions").
		Columns("id", "endpoint", "p256dh", "auth", "expiration_time", "user_agent", "labels", "created_at", "updated_at").
		Values(uuid.NewString(), sub.Endpoint, sub.Keys.P256dh, sub.Keys.Auth, sub.ExpirationTime, sub.UserAgent, labels, now, now).
		Suffix("ON CONFLICT (endpoint) DO UPDATE SET p256dh = EXCLUDED.p256dh, auth = EXCLUDED.auth, expiration_time = EXCLUDED.expiration_time, user_agent = EXCLUDED.user_agent, labels = EXCLUDED.labels, updated_at = EXCLUDED.updated_at").
		Suffix("RETURNING id").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return "", false, fmt.Errorf("postgres.push_repo.Subscribe: %w", err)
	}

	var id string
	if err := conn.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
		return "", false, fmt.Errorf("postgres.push_repo.Subscribe: %w", err)
	}

	sql, args, err = squirrel.Insert("push_subscription_topics").
		Columns("subscription_id", "subject").
		Values(id, subject).
		Suffix("ON CONFLICT DO NOTHING").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return "", false, fmt.Errorf("postgres.push_repo.Subscribe: %w", err)
	}

	tag, err := conn.Exec(ctx, sql, args...)
	if err != nil {
		return "", false, fmt.Errorf("postgres.push_repo.Subscribe: %w", err)
	}

	return id, tag.RowsAffected() > 0, nil
}

func (r *PushRepo) Unsubscribe(ctx context.Context, subject, endpoint string) (bool, error) {
	conn := r.DB.GetConnection(ctx)

	sql, args, err := squirrel.Delete("push_subscription_topics").
		Where(squirrel.Eq{"subject": subject}).
		Where(squirrel.Expr("subscription_id IN (SELECT id FROM push_subscriptions WHERE endpoint = ?)", endpoint)).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return false, fmt.Errorf("postgres.push_repo.Unsubscribe: %w", err)
	}

	tag, err := conn.Exec(ctx, sql, args...)
	if err != nil {
		return false, fmt.Errorf("postgres.push_repo.Unsubscribe: %w", err)
	}

	sql, args, err = squirrel.Delete("push_subscriptions").
		Where(squirrel.Eq{"endpoint": endpoint}).
		Where(squirrel.Expr("NOT EXISTS (SELECT 1 FROM push_subscription_topics t WHERE t.subscription_id = push_subscriptions.id)")).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return false, fmt.Errorf("postgres.push_repo.Unsubscribe: %w", err)
	}

	if _, err := conn.Exec(ctx, sql, args...); err != nil {
		return false, fmt.Errorf("postgres.push_repo.Unsubscribe: %w", err)
	}

	return tag.RowsAffected() > 0, nil
}

func (r *PushRepo) SubjectsByEndpoint(ctx context.Context, endpoint string) ([]string, error) {
	sql, args, err := squirrel.Select("t.subject").
		From("push_subscription_topics t").
		Join("push_subscriptions s ON s.id = t.subscription_id").
		Where(squirrel.Eq{"s.endpoint": endpoint}).
		OrderBy("t.subject").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("postgres.push_repo.SubjectsByEndpoint: %w", err)
	}

	return r.scanSubjects(ctx, "SubjectsByEndpoint", sql, args)
}

func (r *PushRepo) SubscribedSubjects(ctx context.Context) ([]string, error) {
	sql, args, err := squirrel.Select("DISTINCT subject").
		From("push_subscription_topics").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("postgres.push_repo.SubscribedSubjects: %w", err)
	}

	return r.scanSubjects(ctx, "SubscribedSubjects", sql, args)
}

func (r *PushRepo) scanSubjects(ctx context.Context, op, sql string, args []any) ([]string, error) {
	rows, err := r.DB.GetConnection(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres.push_repo.%s: %w", op, err)
	}
	defer rows.Close()

	subjects := make([]string, 0)
	for rows.Next() {
		var subject string
		if err := rows.Scan(&subject); err != nil {
			return nil, fmt.Errorf("postgres.push_repo.%s: %w", op, err)
		}
		subjects = append(subjects, subject)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.push_repo.%s: %w", op, err)
	}
	return subjects, nil
}

func (r *PushRepo) SubscribersOf(ctx context.Context, subject string) ([]*models.PushSubscription, error) {
	sql, args, err := squirrel.Select(subscriptionColumns).
		From("push_subscriptions s").
		Join("push_subscription_topics t ON t.subscription_id = s.id").
		Where(squirrel.Eq{"t.subject": subject}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("postgres.push_repo.SubscribersOf: %w", err)
	}

	rows, err := r.DB.GetConnection(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres.push_repo.SubscribersOf: %w", err)
	}
	defer rows.Close()

	subs := make([]*models.PushSubscription, 0)
	for rows.Next() {
		var sub models.PushSubscription
		if err := rows.Scan(&sub.ID, &sub.Endpoint, &sub.Keys.P256dh, &sub.Keys.Auth, &sub.ExpirationTime, &sub.UserAgent, &sub.Labels, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, fmt.Errorf("postgres.push_repo.SubscribersOf: %w", err)
		}
		subs = append(subs, &sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.push_repo.SubscribersOf: %w", err)
	}
	return subs, nil
}

func (r *PushRepo) DeleteByEndpoint(ctx context.Context, endpoint string) error {
	sql, args, err := squirrel.Delete("push_subscriptions").
		Where(squirrel.Eq{"endpoint": endpoint}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("postgres.push_repo.DeleteByEndpoint: %w", err)
	}

	if _, err := r.DB.GetConnection(ctx).Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("postgres.push_repo.DeleteByEndpoint: %w", err)
	}
	return nil
}

func (r *PushRepo) AppendEvent(ctx context.Context, ev *models.NotificationEvent) (int64, error) {
	createdAt := ev.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	sql, args, err := squirrel.Insert("push_events").
		Columns("subject", "title", "body", "tag", "provider_id", "url", "created_at").
		Values(ev.Subject, ev.Title, ev.Body, ev.Tag, ev.ProviderID, ev.URL, createdAt).
		Suffix("RETURNING id").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("postgres.push_repo.AppendEvent: %w", err)
	}

	var id int64
	if err := r.DB.GetConnection(ctx).QueryRow(ctx, sql, args...).Scan(&id); err != nil {
		return 0, fmt.Errorf("postgres.push_repo.AppendEvent: %w", err)
	}
	return id, nil
}

func (r *PushRepo) EventsForEndpoint(ctx context.Context, endpoint string, afterID int64, limit int) ([]*models.NotificationEvent, error) {
	if limit <= 0 {
		limit = 50
	}

	sql, args, err := squirrel.Select("id", "subject", "title", "body", "tag", "provider_id", "url", "created_at").
		From("push_events").
		Where(squirrel.Expr("subject IN (SELECT t.subject FROM push_subscription_topics t JOIN push_subscriptions s ON s.id = t.subscription_id WHERE s.endpoint = ?)", endpoint)).
		Where(squirrel.Gt{"id": afterID}).
		OrderBy("id ASC").
		Limit(uint64(limit)).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("postgres.push_repo.EventsForEndpoint: %w", err)
	}

	rows, err := r.DB.GetConnection(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres.push_repo.EventsForEndpoint: %w", err)
	}
	defer rows.Close()

	events := make([]*models.NotificationEvent, 0)
	for rows.Next() {
		var ev models.NotificationEvent
		if err := rows.Scan(&ev.ID, &ev.Subject, &ev.Title, &ev.Body, &ev.Tag, &ev.ProviderID, &ev.URL, &ev.CreatedAt); err != nil {
			return nil, fmt.Errorf("postgres.push_repo.EventsForEndpoint: %w", err)
		}
		events = append(events, &ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.push_repo.EventsForEndpoint: %w", err)
	}
	return events, nil
}

func (r *PushRepo) PruneEvents(ctx context.Context, olderThan time.Time) error {
	sql, args, err := squirrel.Delete("push_events").
		Where(squirrel.Lt{"created_at": olderThan}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("postgres.push_repo.PruneEvents: %w", err)
	}

	if _, err := r.DB.GetConnection(ctx).Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("postgres.push_repo.PruneEvents: %w", err)
	}
	return nil
}

func (r *PushRepo) GetVapidKeys(ctx context.Context) (*models.VapidKeys, error) {
	sql, args, err := squirrel.Select("public_key", "private_key").
		From("push_vapid_keys").
		Where(squirrel.Eq{"id": true}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("postgres.push_repo.GetVapidKeys: %w", err)
	}

	var keys models.VapidKeys
	err = r.DB.GetConnection(ctx).QueryRow(ctx, sql, args...).Scan(&keys.PublicKey, &keys.PrivateKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("postgres.push_repo.GetVapidKeys: %w", err)
	}
	return &keys, nil
}

func (r *PushRepo) SaveVapidKeys(ctx context.Context, keys *models.VapidKeys) (*models.VapidKeys, error) {
	sql, args, err := squirrel.Insert("push_vapid_keys").
		Columns("id", "public_key", "private_key").
		Values(true, keys.PublicKey, keys.PrivateKey).
		Suffix("ON CONFLICT (id) DO NOTHING").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("postgres.push_repo.SaveVapidKeys: %w", err)
	}

	if _, err := r.DB.GetConnection(ctx).Exec(ctx, sql, args...); err != nil {
		return nil, fmt.Errorf("postgres.push_repo.SaveVapidKeys: %w", err)
	}

	stored, err := r.GetVapidKeys(ctx)
	if err != nil {
		return nil, err
	}
	if stored == nil {
		return nil, fmt.Errorf("postgres.push_repo.SaveVapidKeys: keys missing after insert")
	}
	return stored, nil
}
