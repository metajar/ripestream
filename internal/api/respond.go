package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
)

// envelope is the standard JSON wrapper for every API response.
type envelope struct {
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
	Meta  any    `json:"meta,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		slog.Error("encode api response", "err", err)
		status = http.StatusInternalServerError
		body.Reset()
		body.WriteString("{\"error\":\"internal error\"}\n")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(body.Bytes()); err != nil {
		slog.Debug("write api response", "err", err)
	}
}

func respondOK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, envelope{Data: data})
}

func respondError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, envelope{Error: msg})
}

// ---- query param helpers ----------------------------------------------------

func qInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func qFloat(r *http.Request, key string, def float64) float64 {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

// qInt64 parses a path or query integer-or-zero; returns 0 on miss/parse error.
func qInt64(r *http.Request, key string) int64 {
	v := r.URL.Query().Get(key)
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

type pageMeta struct {
	Limit   int  `json:"limit"`
	Offset  int  `json:"offset"`
	HasMore bool `json:"has_more"`
}

// pageParams returns a bounded page size and a non-negative offset. List
// handlers request one additional row from their data store so has_more can be
// reported without running a separate full count query.
func pageParams(r *http.Request, defaultLimit int) (limit, offset int) {
	limit = qInt(r, "limit", defaultLimit)
	if limit < 1 {
		limit = defaultLimit
	}
	if limit > 100 {
		limit = 100
	}
	offset = qInt(r, "offset", 0)
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func respondPage[T any](w http.ResponseWriter, rows []T, limit, offset int) {
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	writeJSON(w, http.StatusOK, envelope{
		Data: rows,
		Meta: pageMeta{Limit: limit, Offset: offset, HasMore: hasMore},
	})
}

// ---- middleware -------------------------------------------------------------

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("api panic", "path", r.URL.Path, "panic", rec)
				respondError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// Log non-GET-lightweight requests; skip /api/health noise.
		if r.URL.Path != "/api/health" {
			srw := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(srw, r)
			slog.Info("api request", "method", r.Method, "path", r.URL.Path, "status", srw.status)
			return
		}
		next.ServeHTTP(w, r)
	})
}
