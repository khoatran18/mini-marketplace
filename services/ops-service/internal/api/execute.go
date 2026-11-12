package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"ops-service/internal/actions"
	"ops-service/internal/executor"
	"ops-service/internal/store"
)

// Runner là phần của executor mà API cần (tách để test).
type Runner interface {
	Run(ctx context.Context, actionID string, params map[string]any) (map[string]any, error)
}

func (a *API) execLimits() store.ExecLimits {
	l := a.ExecLimits
	if l.MaxPerHour == 0 {
		l.MaxPerHour = 5
	}
	if l.Cooldown == 0 {
		l.Cooldown = 5 * time.Minute
	}
	if l.Window == 0 {
		l.Window = time.Hour
	}
	return l
}

func execErr(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrBadState):
		writeErr(w, http.StatusConflict, "đề xuất chưa được duyệt (ACCEPTED/ALTERNATIVE)")
	case errors.Is(err, store.ErrAlreadyExecuted):
		writeErr(w, http.StatusConflict, "đề xuất đã thực thi hoặc đang thực thi")
	case errors.Is(err, store.ErrSelfApproval):
		writeErr(w, http.StatusForbidden, "người tạo đề xuất không được tự thực thi")
	case errors.Is(err, store.ErrApprovalStale):
		writeErr(w, http.StatusConflict, "quyết định duyệt đã quá hạn thực thi: tạo đề xuất mới")
	case errors.Is(err, store.ErrRateLimited), errors.Is(err, store.ErrCooldown):
		writeErr(w, http.StatusTooManyRequests, err.Error())
	default:
		writeErr(w, http.StatusInternalServerError, "internal error")
	}
	return true
}

// executeProposal chạy hành động đã duyệt (chỉ mode=http). Công tắc OPS_ACTIONS_ENABLED tắt thì 403 và không chạm gì.
func (a *API) executeProposal(w http.ResponseWriter, r *http.Request) {
	n, ok := id(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "id không hợp lệ")
		return
	}
	if !a.ActionsEnabled {
		writeErr(w, http.StatusForbidden, "thực thi đang tắt (OPS_ACTIONS_ENABLED=false)")
		return
	}
	if a.Runner == nil {
		writeErr(w, http.StatusServiceUnavailable, "executor chưa cấu hình")
		return
	}
	p, err := a.Store.GetProposal(r.Context(), n)
	if execErr(w, err) {
		return
	}
	actionID, _, _ := p.Effective()
	if act, found := actions.Get(actionID); !found || act.Mode != actions.ModeHTTP {
		writeErr(w, http.StatusConflict, "hành động này không tự chạy: chạy lệnh trong đề xuất rồi gọi /executed")
		return
	}
	who, rid := actor(r), r.Header.Get("X-Request-ID")
	p, err = a.Store.ClaimExecution(r.Context(), n, who, rid, a.execLimits(), a.now())
	if execErr(w, err) {
		return
	}
	actionID, params, _ := p.Effective()
	// kiểm lại tham số theo sổ hành động ngay trước khi chạy (không tin dữ liệu đã lưu)
	runCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var res map[string]any
	if _, verr := actions.Validate(actionID, params); verr != nil {
		err = verr
	} else {
		res, err = a.Runner.Run(runCtx, actionID, params)
	}
	if res == nil {
		res = map[string]any{}
	}
	okRun := err == nil
	if err != nil {
		res["error"] = err.Error()
		res["precondition_failed"] = errors.Is(err, executor.ErrPrecondition)
	}
	if ferr := a.Store.FinishExecution(context.Background(), n, okRun, res, who, rid, a.now()); ferr != nil {
		writeErr(w, http.StatusInternalServerError, "đã chạy nhưng không ghi được kết quả: kiểm tra audit")
		return
	}
	code := http.StatusOK
	if !okRun {
		code = http.StatusUnprocessableEntity
	}
	writeJSON(w, code, map[string]any{"exec_state": map[bool]string{true: store.ExecDone, false: store.ExecFailed}[okRun], "result": res})
}

type executedReq struct {
	Outcome string `json:"outcome"` // success | failed
	Note    string `json:"note"`
}

// recordExecuted ghi kết quả khi người đã tự chạy lệnh (mode=command/manual).
func (a *API) recordExecuted(w http.ResponseWriter, r *http.Request) {
	n, ok := id(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "id không hợp lệ")
		return
	}
	var req executedReq
	if !decode(w, r, &req) {
		return
	}
	if (req.Outcome != "success" && req.Outcome != "failed") || len(req.Note) > 2000 {
		writeErr(w, http.StatusBadRequest, "outcome phải là success|failed, note tối đa 2000 ký tự")
		return
	}
	p, err := a.Store.GetProposal(r.Context(), n)
	if execErr(w, err) {
		return
	}
	actionID, _, _ := p.Effective()
	if act, found := actions.Get(actionID); !found || act.Mode == actions.ModeHTTP {
		writeErr(w, http.StatusConflict, "hành động này do ops-service tự chạy: dùng /execute")
		return
	}
	p, err = a.Store.RecordManual(r.Context(), n, actor(r), r.Header.Get("X-Request-ID"), req.Outcome == "success", req.Note, a.now())
	if execErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"proposal": p})
}
