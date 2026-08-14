package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool *pgxpool.Pool
}

type txKey struct{}

type connector interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, optionsAndArgs ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, optionsAndArgs ...any) pgx.Row
}

type Config struct {
	Host     string `env:"POSTGRES_HOST" env-default:"127.0.0.1"`
	Port     string `env:"POSTGRES_PORT" env-default:"5432"`
	User     string `env:"POSTGRES_USER" env-default:"postgres"`
	Password string `env:"POSTGRES_PASSWORD" env-default:"postgres"`
	DB       string `env:"POSTGRES_DB" env-default:"postgres"`
	SSLMode  string `env:"POSTGRES_SSL" env-default:"disable"`
}

func New(ctx context.Context, cfg *Config) (*DB, error) {
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.DB,
		cfg.SSLMode,
	)

	connPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	connection, err := connPool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("can't acquire connection: %w", err)
	}
	defer connection.Release()

	err = connection.Conn().Ping(ctx)
	if err != nil {
		return nil, fmt.Errorf("can't ping database: %w", err)
	}

	return &DB{
		Pool: connPool,
	}, nil
}

func (db *DB) Close() {
	db.Pool.Close()
}

func (db *DB) WithinTransaction(
	ctx context.Context,
	tFunc func(ctx context.Context) error,
) error {
	incomingTx := extractTx(ctx)
	if incomingTx != nil {
		err := tFunc(ctx)
		if err == nil {
			return nil
		}

		tx, ok := incomingTx.(pgx.Tx)
		if ok {
			_ = tx.Rollback(ctx)
		}

		return fmt.Errorf("transaction failed: %w", err)
	}

	conn, err := db.Pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("can't acquire connection: %w", err)
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("can't begin transaction: %w", err)
	}

	err = tFunc(injectTx(ctx, tx))
	if err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("transaction failed: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("can't commit transaction: %w", err)
	}

	return nil
}

func (db *DB) GetConnection(ctx context.Context) connector {
	conn := extractTx(ctx)
	if conn == nil {
		conn = db.Pool
	}

	return conn
}

func injectTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

func extractTx(ctx context.Context) connector {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}

	return nil
}