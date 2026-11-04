package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/ledger/internal/store"
)

type fakeStore struct{ entries []store.Entry }

func (f *fakeStore) Ping(context.Context) error { return nil }
func (f *fakeStore) Close()                     {}
func (f *fakeStore) ListEntries(_ context.Context, _ string, _ int) ([]store.Entry, error) {
	return f.entries, nil
}
func (f *fakeStore) AddEntry(_ context.Context, e store.Entry) (store.Entry, error) {
	e.ID = int64(len(f.entries) + 1)
	f.entries = append(f.entries, e)
	return e, nil
}

func TestHealth(t *testing.T) {
	h := New(&fakeStore{}, "test-only-secret", "test").Routes()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestAddEntryRequiresAuth(t *testing.T) {
	h := New(&fakeStore{}, "test-only-secret", "test").Routes()
	req := httptest.NewRequest(http.MethodPost, "/v1/accounts/a/entries", strings.NewReader(`{"amount_cents":5}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAddEntry(t *testing.T) {
	fs := &fakeStore{}
	h := New(fs, "test-only-secret", "test").Routes()
	req := httptest.NewRequest(http.MethodPost, "/v1/accounts/a/entries", strings.NewReader(`{"amount_cents":500,"memo":"coffee"}`))
	req.Header.Set("Authorization", "Bearer test-only-secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated || len(fs.entries) != 1 {
		t.Fatalf("status = %d entries = %d", rec.Code, len(fs.entries))
	}
}
