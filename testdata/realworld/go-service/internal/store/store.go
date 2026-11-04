// Package store persists ledger entries.
package store

import (
	"context"
	"time"
)

type Entry struct {
	ID          int64     `json:"id"`
	Account     string    `json:"account"`
	AmountCents int64     `json:"amount_cents"`
	Memo        string    `json:"memo"`
	CreatedAt   time.Time `json:"created_at"`
}

// Store is the persistence interface used by the handlers.
type Store interface {
	Ping(ctx context.Context) error
	ListEntries(ctx context.Context, account string, limit int) ([]Entry, error)
	AddEntry(ctx context.Context, e Entry) (Entry, error)
	Close()
}
