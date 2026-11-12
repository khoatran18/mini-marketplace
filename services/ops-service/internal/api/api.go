// Package api là HTTP API của ops-service (cổng :8080, chỉ gateway gọi). Mọi route cần `Authorization: Bearer <OPS_API_TOKEN>`;
// gateway đã kiểm JWT + role admin và gửi `X-Actor` (vd admin:7). Không có token cấu hình thì API từ chối mọi yêu cầu.
package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ops-service/internal/approval"
	"ops-service/internal/queries"
	"ops-service/internal/store"
)

// API giữ store và token. Signer dẫn xuất từ Token (nil nếu không có token: khi đó mọi route đã bị chặn).
type API struct {
	Store *store.Store
	Token string
	// Now cho phép test đặt giờ; mặc định time.Now.
	Now func() time.Time
	// Cấu hình đề xuất (0 = mặc định): hạn đề xuất, hạn mã duyệt, số đề xuất PENDING tối đa cho một sự cố.
	ProposalTTL time.Duration
	ApprovalTTL time.Duration
	MaxPending  int
	// Queries chạy truy vấn đọc có allowlist; nil = tính năng tắt (503).
	Queries *queries.Runner
	// Thực thi (đợt 3): công tắc, hạn mức và bộ chạy. ActionsEnabled=false (mặc định) thì /execute luôn 403.
	ActionsEnabled bool
	ExecLimits     store.ExecLimits
	Runner         Runner

	signer *approval.Signer
}

func (a *API) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

// Handler dựng mux.
func (a *API) Handler() http.Handler {
	if a.Token != "" {
		a.signer = approval.New(a.Token)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /actions", a.listActions)
	mux.HandleFunc("POST /proposals", a.createProposal)
	mux.HandleFunc("GET /proposals", a.listProposals)
	mux.HandleFunc("GET /proposals/{id}", a.getProposal)
	mux.HandleFunc("POST /proposals/{id}/review", a.reviewProposal)
	mux.HandleFunc("POST /proposals/{id}/decision", a.decideProposal)
	mux.HandleFunc("POST /proposals/{id}/execute", a.executeProposal)
	mux.HandleFunc("POST /proposals/{id}/executed", a.recordExecuted)
	mux.HandleFunc("GET /incidents", a.listIncidents)
	mux.HandleFunc("GET /incidents/{id}", a.getIncident)
	mux.HandleFunc("POST /incidents/{id}/investigate", a.investigate)
	mux.HandleFunc("GET /audit", a.listAudit)
	mux.HandleFunc("GET /queries", a.listQueries)
	mux.HandleFunc("POST /queries/{name}", a.runQuery)
	return a.auth(mux)
}

func (a *API) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.Token == "" {
			writeErr(w, http.StatusServiceUnavailable, "OPS_API_TOKEN chưa cấu hình: API đang tắt")
			return
		}
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(a.Token)) != 1 {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func actor(r *http.Request) string {
	if v := r.Header.Get("X-Actor"); v != "" {
		return v
	}
	return "unknown"
}

func (a *API) listIncidents(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state != "" && state != store.StateOpen && state != store.StateInvestigating && state != store.StateResolved {
		writeErr(w, http.StatusBadRequest, "state phải là open|investigating|resolved")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := a.Store.List(r.Context(), state, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"incidents": list})
}

func id(r *http.Request) (int64, bool) {
	n, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return n, err == nil && n > 0
}

func (a *API) getIncident(w http.ResponseWriter, r *http.Request) {
	n, ok := id(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "id không hợp lệ")
		return
	}
	inc, err := a.Store.Get(r.Context(), n)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	trail, err := a.Store.AuditList(r.Context(), "incident", strconv.FormatInt(n, 10), 100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"incident": inc, "audit": trail})
}

func (a *API) investigate(w http.ResponseWriter, r *http.Request) {
	n, ok := id(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "id không hợp lệ")
		return
	}
	switch err := a.Store.Investigate(r.Context(), n, actor(r), r.Header.Get("X-Request-ID")); {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrBadState):
		writeErr(w, http.StatusConflict, "chỉ chuyển được từ open sang investigating")
	case err != nil:
		writeErr(w, http.StatusInternalServerError, "internal error")
	default:
		writeJSON(w, http.StatusOK, map[string]string{"state": store.StateInvestigating})
	}
}

func (a *API) listAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if (q.Get("subject_type") == "") != (q.Get("subject_id") == "") {
		writeErr(w, http.StatusBadRequest, "subject_type và subject_id đi cùng nhau")
		return
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	list, err := a.Store.AuditList(r.Context(), q.Get("subject_type"), q.Get("subject_id"), limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"audit": list})
}

func (a *API) listQueries(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"queries": queries.Catalog()})
}

// runQuery chạy một truy vấn có tên (body: {"params":{...}}). Mỗi lần chạy ghi audit (người gọi, tên, tham số).
func (a *API) runQuery(w http.ResponseWriter, r *http.Request) {
	if a.Queries == nil {
		writeErr(w, http.StatusServiceUnavailable, "truy vấn chưa cấu hình")
		return
	}
	var body struct {
		Params map[string]string `json:"params"`
	}
	if r.ContentLength != 0 {
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "body không hợp lệ")
			return
		}
	}
	name := r.PathValue("name")
	res, err := a.Queries.Run(r.Context(), name, body.Params)
	if errors.Is(err, queries.ErrInvalid) {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = a.Store.Audit(r.Context(), actor(r), "query.run", "query", name, r.Header.Get("X-Request-ID"), map[string]any{"params": body.Params, "ok": err == nil})
	if err != nil {
		writeErr(w, http.StatusBadGateway, "backend lỗi: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}
