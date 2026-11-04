package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type pg struct {
	pool *pgxpool.Pool
}

// Open connects to Postgres and returns a Store.
func Open(ctx context.Context, url string, maxConns int32) (Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConns = maxConns

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	return &pg{pool: pool}, nil
}

func (p *pg) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }

func (p *pg) Close() { p.pool.Close() }

func (p *pg) ListEntries(ctx context.Context, account string, limit int) ([]Entry, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, account, amount_cents, memo, created_at
		   FROM entries WHERE account = $1 ORDER BY id DESC LIMIT $2`, account, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.Account, &e.AmountCents, &e.Memo, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (p *pg) AddEntry(ctx context.Context, e Entry) (Entry, error) {
	err := p.pool.QueryRow(ctx,
		`INSERT INTO entries (account, amount_cents, memo) VALUES ($1, $2, $3)
		 RETURNING id, created_at`, e.Account, e.AmountCents, e.Memo).Scan(&e.ID, &e.CreatedAt)
	return e, err
}
