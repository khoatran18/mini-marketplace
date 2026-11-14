// Package playbooks đọc docs/playbooks/*.yml (đáp án chuẩn của sự cố: cảnh báo, truy vấn chẩn đoán, nguyên nhân, hành động) và phục vụ
// cho agent operator/admin đọc qua GET /playbooks. Chỉ ĐỌC: agent không chạy được command trong playbook, chỉ nêu lại cho người.
// Trường ground_truth (mã kịch bản chấm điểm) không phục vụ ra ngoài, để mô hình không học thuộc đáp án của bộ đánh giá.
package playbooks

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Step struct {
	ID        string `yaml:"id" json:"id"`
	Tool      string `yaml:"tool" json:"tool"`
	Query     string `yaml:"query" json:"query"`
	ReadOnly  bool   `yaml:"read_only" json:"read_only"`
	Interpret string `yaml:"interpret" json:"interpret"`
}

type Cause struct {
	ID   string   `yaml:"id" json:"id"`
	When string   `yaml:"when" json:"when"`
	Fix  []string `yaml:"fix" json:"fix"`
}

type Action struct {
	ID           string `yaml:"id" json:"id"`
	Risk         string `yaml:"risk" json:"risk"`
	Approval     string `yaml:"approval" json:"approval"`
	Command      string `yaml:"command" json:"command"`
	Precondition string `yaml:"precondition,omitempty" json:"precondition,omitempty"`
	Verify       string `yaml:"verify" json:"verify"`
	Rollback     string `yaml:"rollback,omitempty" json:"rollback,omitempty"`
}

type Playbook struct {
	ID       string `yaml:"id" json:"id"`
	Title    string `yaml:"title" json:"title"`
	Triggers struct {
		Alerts []string `yaml:"alerts" json:"alerts"`
	} `yaml:"triggers" json:"triggers"`
	Runbook  string   `yaml:"runbook" json:"runbook"`
	Diagnose []Step   `yaml:"diagnose" json:"diagnose"`
	Causes   []Cause  `yaml:"causes" json:"causes"`
	Actions  []Action `yaml:"actions" json:"actions"`
	Escalate string   `yaml:"escalate" json:"escalate"`
	// Tồn tại trong YAML nhưng không phục vụ ra ngoài (json:"-").
	GroundTruth []string `yaml:"ground_truth" json:"-"`
}

// Summary là một dòng trong danh sách.
type Summary struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Alerts  []string `json:"alerts"`
	Runbook string   `json:"runbook"`
}

// Library giữ các playbook đã nạp, sắp theo id.
type Library struct{ list []Playbook }

// Load đọc mọi *.yml trong dir. Thư mục không tồn tại hoặc rỗng cho Library rỗng (service vẫn chạy, API trả danh sách rỗng);
// file sai lược đồ (khoá lạ, thiếu id) là lỗi, để docs/playbooks không lệch lặng lẽ với những gì agent đọc.
func Load(dir string) (*Library, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.yml"))
	if err != nil {
		return nil, err
	}
	lib := &Library{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		dec := yaml.NewDecoder(bytes.NewReader(b))
		dec.KnownFields(true)
		var p Playbook
		if err := dec.Decode(&p); err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(f), err)
		}
		if p.ID == "" || p.Title == "" {
			return nil, fmt.Errorf("%s: thiếu id hoặc title", filepath.Base(f))
		}
		if strings.TrimSuffix(filepath.Base(f), ".yml") != p.ID {
			return nil, fmt.Errorf("%s: id %q không khớp tên file", filepath.Base(f), p.ID)
		}
		lib.list = append(lib.list, p)
	}
	sort.Slice(lib.list, func(i, j int) bool { return lib.list[i].ID < lib.list[j].ID })
	return lib, nil
}

// List trả tóm tắt; alert != "" chỉ giữ playbook có cảnh báo đó (so khớp không phân biệt hoa thường).
func (l *Library) List(alert string) []Summary {
	out := []Summary{}
	if l == nil {
		return out
	}
	for _, p := range l.list {
		if alert != "" && !hasAlert(p, alert) {
			continue
		}
		out = append(out, Summary{ID: p.ID, Title: p.Title, Alerts: p.Triggers.Alerts, Runbook: p.Runbook})
	}
	return out
}

// Get trả một playbook đầy đủ theo id.
func (l *Library) Get(id string) (Playbook, bool) {
	if l == nil {
		return Playbook{}, false
	}
	for _, p := range l.list {
		if p.ID == id {
			return p, true
		}
	}
	return Playbook{}, false
}

func hasAlert(p Playbook, alert string) bool {
	for _, a := range p.Triggers.Alerts {
		if strings.EqualFold(a, alert) {
			return true
		}
	}
	return false
}
