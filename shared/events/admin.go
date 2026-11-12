package events

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// Mount adds the dead-letter tools to a service's admin mux (ops.Options.Mount). Routes (internal port only):
//
//	GET  /internal/outbox/dead?limit=50               list DEAD rows (no payload) newest first
//	POST /internal/outbox/requeue?id=1&id=2&limit=100 requeue the given DEAD ids, or every DEAD row up to limit
//
// When METRICS_TOKEN is set both routes need "Authorization: Bearer <token>" (the requeue changes data).
// The table is the one chosen with SetTable. Dead rows are the runbook's "needs a human" signal (mm_outbox_dead > 0).
func Mount(db *gorm.DB) func(mux *http.ServeMux) {
	return func(mux *http.ServeMux) {
		mux.HandleFunc("GET /internal/outbox/dead", guarded(func(w http.ResponseWriter, r *http.Request) {
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			rows, err := ListDead(db, limit)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list failed"})
				return
			}
			n, _ := DeadCount(db)
			writeJSON(w, http.StatusOK, map[string]any{"table": Table(), "dead_total": n, "rows": rows})
		}))
		mux.HandleFunc("POST /internal/outbox/requeue", guarded(func(w http.ResponseWriter, r *http.Request) {
			var ids []uint64
			for _, v := range r.URL.Query()["id"] {
				id, err := strconv.ParseUint(v, 10, 64)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id must be a number"})
					return
				}
				ids = append(ids, id)
			}
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			n, err := RequeueDead(db, ids, limit)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "requeue failed"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"table": Table(), "requeued": n})
		}))
	}
}

func guarded(next http.HandlerFunc) http.HandlerFunc {
	tok := strings.TrimSpace(os.Getenv("METRICS_TOKEN"))
	if tok == "" {
		return next
	}
	want := sha256.Sum256([]byte(tok))
	return func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		got := sha256.Sum256([]byte(strings.TrimPrefix(h, "Bearer ")))
		if !strings.HasPrefix(h, "Bearer ") || subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
