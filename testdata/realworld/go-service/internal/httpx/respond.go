package httpx

import (
	"encoding/json"
	"net/http"
)

type errorBody struct {
	Error string `json:"error"`
}

// JSON writes v as a JSON response with the given status code.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Error writes a JSON error body.
func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, errorBody{Error: msg})
}
