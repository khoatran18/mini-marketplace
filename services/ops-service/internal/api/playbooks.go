package api

import (
	"net/http"
	"regexp"
)

var playbookID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// listPlaybooks: GET /playbooks[?alert=TênCảnhBáo]. Không tham số = tất cả.
func (a *API) listPlaybooks(w http.ResponseWriter, r *http.Request) {
	alert := r.URL.Query().Get("alert")
	if len(alert) > 64 {
		writeErr(w, http.StatusBadRequest, "alert quá dài")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"playbooks": a.Playbooks.List(alert)})
}

// getPlaybook: GET /playbooks/{id}: chẩn đoán, nguyên nhân, hành động (chỉ để đọc; agent không chạy được command).
func (a *API) getPlaybook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !playbookID.MatchString(id) {
		writeErr(w, http.StatusBadRequest, "id playbook không hợp lệ")
		return
	}
	p, ok := a.Playbooks.Get(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "không có playbook này")
		return
	}
	writeJSON(w, http.StatusOK, p)
}
