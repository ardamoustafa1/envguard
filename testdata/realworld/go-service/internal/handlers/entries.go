package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/example/ledger/internal/httpx"
	"github.com/example/ledger/internal/store"
)

func (h *Handlers) ListEntries(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			httpx.Error(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = n
	}

	entries, err := h.store.ListEntries(r.Context(), chi.URLParam(r, "account"), limit)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not list entries")
		return
	}
	httpx.JSON(w, http.StatusOK, entries)
}

func (h *Handlers) AddEntry(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AmountCents int64  `json:"amount_cents"`
		Memo        string `json:"memo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.AmountCents == 0 {
		httpx.Error(w, http.StatusUnprocessableEntity, "amount_cents must be non-zero")
		return
	}

	e, err := h.store.AddEntry(r.Context(), store.Entry{
		Account:     chi.URLParam(r, "account"),
		AmountCents: in.AmountCents,
		Memo:        in.Memo,
	})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not add entry")
		return
	}
	httpx.JSON(w, http.StatusCreated, e)
}
