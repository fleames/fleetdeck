package httpx

import (
	"encoding/json"
	"io"
	"net/http"
)

const defaultMaxBody = 1 << 20 // 1 MiB

type ErrorBody struct {
	Code        string         `json:"code"`
	Message     string         `json:"message"`
	Details     map[string]any `json:"details,omitempty"`
	Diagnostics map[string]any `json:"diagnostics,omitempty"`
}

type errorEnvelope struct {
	Error ErrorBody `json:"error"`
}

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, errorEnvelope{Error: ErrorBody{Code: code, Message: message}})
}

func ErrorDetails(w http.ResponseWriter, status int, code, message string, details, diagnostics map[string]any) {
	JSON(w, status, errorEnvelope{Error: ErrorBody{
		Code: code, Message: message, Details: details, Diagnostics: diagnostics,
	}})
}

func Decode(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, defaultMaxBody))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
