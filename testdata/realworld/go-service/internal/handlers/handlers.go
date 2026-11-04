// Package handlers wires HTTP routes to the store.
package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/example/ledger/internal/httpx"
	"github.com/example/ledger/internal/store"
)

type Handlers struct {
	store   store.Store
	secret  string
	version string
}

func New(s store.Store, secret, version string) *Handlers {
	return &Handlers{store: s, secret: secret, version: version}
}

func (h *Handlers) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(httpx.Logging)

	r.Get("/healthz", h.Health)
	r.Group(func(r chi.Router) {
		r.Use(httpx.BearerAuth(h.secret))
		r.Get("/v1/accounts/{account}/entries", h.ListEntries)
		r.Post("/v1/accounts/{account}/entries", h.AddEntry)
	})
	return r
}
