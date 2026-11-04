package handlers

import (
	"net/http"

	"github.com/example/ledger/internal/httpx"
)

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Ping(r.Context()); err != nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok", "version": h.version})
}
