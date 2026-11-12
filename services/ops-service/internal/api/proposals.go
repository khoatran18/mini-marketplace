package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ops-service/internal/actions"
	"ops-service/internal/store"
)

const maxBody = 64 << 10

func (a *API) proposalTTL() time.Duration {
	if a.ProposalTTL > 0 {
		return a.ProposalTTL
	}
	return 24 * time.Hour
}

func (a *API) approvalTTL() time.Duration {
	if a.ApprovalTTL > 0 {
		return a.ApprovalTTL
	}
	return 5 * time.Minute
}

func (a *API) maxPending() int {
	if a.MaxPending > 0 {
		return a.MaxPending
	}
	return 5
}

// decode đọc JSON chặt: giới hạn kích thước, từ chối trường lạ.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "body JSON không hợp lệ: "+err.Error())
		return false
	}
	return true
}

// numbersToFloat đổi json.Number (do UseNumber) về float64 để actions.Validate dùng chung một kiểu.
func numbersToFloat(v any) any {
	switch t := v.(type) {
	case json.Number:
		f, _ := t.Float64()
		return f
	case map[string]any:
		for k, x := range t {
			t[k] = numbersToFloat(x)
		}
	case []any:
		for i, x := range t {
			t[i] = numbersToFloat(x)
		}
	}
	return v
}

func (a *API) listActions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"actions": actions.All()})
}

type createReq struct {
	IncidentID   *int64         `json:"incident_id"`
	ActionID     string         `json:"action_id"`
	Params       map[string]any `json:"params"`
	Evidence     []string       `json:"evidence"`
	Rationale    string         `json:"rationale"`
	SupersedesID *int64         `json:"supersedes_id"`
}

func (a *API) createProposal(w http.ResponseWriter, r *http.Request) {
	var req createReq
	if !decode(w, r, &req) {
		return
	}
	for k, v := range req.Params {
		req.Params[k] = numbersToFloat(v)
	}
	if len(req.Evidence) > 20 {
		writeErr(w, http.StatusBadRequest, "evidence tối đa 20 mục")
		return
	}
	for _, e := range req.Evidence {
		if len(e) > 1000 {
			writeErr(w, http.StatusBadRequest, "mỗi mục evidence tối đa 1000 ký tự")
			return
		}
	}
	if len(req.Rationale) > 4000 {
		writeErr(w, http.StatusBadRequest, "rationale tối đa 4000 ký tự")
		return
	}
	// Lớp 2: hành động phải nằm trong sổ cho phép, tham số kiểm chặt. Rủi ro lấy từ sổ, bỏ qua mọi thứ người tạo khai.
	act, ok := actions.Get(req.ActionID)
	if !ok {
		writeErr(w, http.StatusBadRequest, "hành động "+strconv.Quote(req.ActionID)+" không có trong sổ hành động cho phép")
		return
	}
	params, err := actions.Validate(req.ActionID, req.Params)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := a.Store.CreateProposal(r.Context(), store.CreateInput{
		IncidentID: req.IncidentID, ActionID: req.ActionID, Params: params, Evidence: req.Evidence, Rationale: req.Rationale,
		Risk: act.Risk, Hash: actions.ContentHash(req.IncidentID, req.ActionID, params, act.Risk), CreatedBy: actor(r),
		RequestID: r.Header.Get("X-Request-ID"), SupersedesID: req.SupersedesID, TTL: a.proposalTTL(), MaxPending: a.maxPending(), Now: a.now(),
	})
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "incident hoặc đề xuất được thay thế không tồn tại")
	case errors.Is(err, store.ErrBadState):
		writeErr(w, http.StatusConflict, "chỉ thay thế được đề xuất đang NEEDS_REVIEW")
	case errors.Is(err, store.ErrTooMany):
		writeErr(w, http.StatusTooManyRequests, "sự cố này đã có quá nhiều đề xuất đang chờ")
	case errors.Is(err, store.ErrDuplicate):
		writeErr(w, http.StatusConflict, "đề xuất giống hệt đang chờ duyệt")
	case err != nil:
		writeErr(w, http.StatusInternalServerError, "internal error")
	default:
		writeJSON(w, http.StatusCreated, map[string]any{"proposal": p})
	}
}

func (a *API) listProposals(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	state := strings.ToUpper(q.Get("state"))
	switch state {
	case "", store.PPending, store.PAccepted, store.PRejected, store.PNeedsReview, store.PAlternative, store.PExpired:
	default:
		writeErr(w, http.StatusBadRequest, "state không hợp lệ")
		return
	}
	inc, _ := strconv.ParseInt(q.Get("incident_id"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	list, err := a.Store.ListProposals(r.Context(), state, inc, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"proposals": list})
}

func (a *API) getProposal(w http.ResponseWriter, r *http.Request) {
	n, ok := id(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "id không hợp lệ")
		return
	}
	p, err := a.Store.GetProposal(r.Context(), n)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	trail, err := a.Store.AuditList(r.Context(), "proposal", strconv.FormatInt(n, 10), 100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := map[string]any{"proposal": p, "audit": trail}
	if p.State == store.PAccepted || p.State == store.PAlternative {
		actionID, params, _ := p.Effective()
		if cmd := actions.Command(actionID, params); cmd != "" {
			out["command"] = cmd // lệnh để người chạy (hành động mode=command)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) reviewProposal(w http.ResponseWriter, r *http.Request) {
	n, ok := id(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "id không hợp lệ")
		return
	}
	now := a.now()
	p, err := a.Store.Review(r.Context(), n, actor(r), r.Header.Get("X-Request-ID"), now)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrBadState):
		writeErr(w, http.StatusConflict, "đề xuất không còn PENDING")
	case errors.Is(err, store.ErrExpired):
		writeErr(w, http.StatusConflict, "đề xuất đã hết hạn")
	case err != nil:
		writeErr(w, http.StatusInternalServerError, "internal error")
	default:
		ttl := a.approvalTTL()
		writeJSON(w, http.StatusOK, map[string]any{
			"proposal":       p,
			"approval_token": a.signer.Issue(p.ID, p.ContentHash, actor(r), now, ttl),
			"expires_in_sec": int(ttl.Seconds()),
		})
	}
}

type decisionReq struct {
	Decision      string `json:"decision"`
	Reason        string `json:"reason"`
	ApprovalToken string `json:"approval_token"`
	Alternative   *struct {
		ActionID string         `json:"action_id"`
		Params   map[string]any `json:"params"`
	} `json:"alternative"`
}

func (a *API) decideProposal(w http.ResponseWriter, r *http.Request) {
	n, ok := id(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "id không hợp lệ")
		return
	}
	var req decisionReq
	if !decode(w, r, &req) {
		return
	}
	switch req.Decision {
	case store.DecAccept, store.DecReject, store.DecNeedsReview, store.DecAlternative:
	default:
		writeErr(w, http.StatusBadRequest, "decision phải là accept|reject|needs_review|alternative")
		return
	}
	if (req.Decision == store.DecAlternative) != (req.Alternative != nil) {
		writeErr(w, http.StatusBadRequest, "`alternative` đi cùng và chỉ đi cùng decision=alternative")
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if len(reason) > 1000 {
		writeErr(w, http.StatusBadRequest, "reason tối đa 1000 ký tự")
		return
	}
	p, err := a.Store.GetProposal(r.Context(), n)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	// hành động rủi ro cao hoặc bất kỳ quyết định khác chấp nhận: bắt buộc có lý do
	if reason == "" && (req.Decision != store.DecAccept || p.Risk == actions.RiskHigh) {
		writeErr(w, http.StatusBadRequest, "cần `reason` cho quyết định này")
		return
	}
	// Mã duyệt: đúng người này đã xem đúng nội dung (băm) này trong vài phút gần đây.
	if err := a.signer.Verify(req.ApprovalToken, p.ID, p.ContentHash, actor(r), a.now()); err != nil {
		writeErr(w, http.StatusForbidden, "approval_token không hợp lệ hoặc đã hết hạn: gọi /review để xem đề xuất và nhận mã mới")
		return
	}
	var alt *store.Alternative
	if req.Alternative != nil {
		for k, v := range req.Alternative.Params {
			req.Alternative.Params[k] = numbersToFloat(v)
		}
		act, ok := actions.Get(req.Alternative.ActionID)
		if !ok {
			writeErr(w, http.StatusBadRequest, "hành động thay thế không có trong sổ hành động cho phép")
			return
		}
		params, err := actions.Validate(req.Alternative.ActionID, req.Alternative.Params)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "alternative: "+err.Error())
			return
		}
		hash := actions.ContentHash(p.IncidentID, act.ID, params, act.Risk)
		if hash == p.ContentHash {
			writeErr(w, http.StatusBadRequest, "alternative giống hệt đề xuất gốc: dùng decision=accept")
			return
		}
		alt = &store.Alternative{ActionID: act.ID, Params: params, Risk: act.Risk, Hash: hash}
	}
	out, err := a.Store.Decide(r.Context(), n, actor(r), req.Decision, reason, r.Header.Get("X-Request-ID"), alt, a.now())
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrBadState):
		writeErr(w, http.StatusConflict, "đề xuất không còn PENDING")
	case errors.Is(err, store.ErrExpired):
		writeErr(w, http.StatusConflict, "đề xuất đã hết hạn")
	case errors.Is(err, store.ErrSelfApproval):
		writeErr(w, http.StatusForbidden, "người tạo đề xuất không được tự quyết định")
	case err != nil:
		writeErr(w, http.StatusInternalServerError, "internal error")
	default:
		writeJSON(w, http.StatusOK, map[string]any{"proposal": out})
	}
}
