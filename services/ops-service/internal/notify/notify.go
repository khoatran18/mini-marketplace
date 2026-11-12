// Package notify gửi thông báo ra webhook (Slack/Discord/…) khi có sự cố mới hoặc đề xuất chờ duyệt.
// Tắt mặc định (URL rỗng). Nguồn là bảng nhật ký audit: đọc các dòng mới theo id, nên không cần bảng riêng và không gửi khi transaction rollback.
// Gửi "ít nhất một lần" trong lúc chạy; tiến trình khởi động lại thì bắt đầu từ id hiện tại (sự kiện lúc ngừng không được gửi bù).
// Nội dung chỉ gồm tên cảnh báo, id và người tạo, không có chi tiết hành động hay dữ liệu từ log.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"ops-service/internal/store"
)

// Events là các sự kiện audit được thông báo.
var Events = []string{"incident_opened", "proposal_created"}

// Source là phần của store mà notifier cần.
type Source interface {
	MaxAuditID(ctx context.Context) (int64, error)
	AuditSince(ctx context.Context, after int64, events []string, limit int) ([]store.AuditEntry, error)
}

type Notifier struct {
	Source   Source
	URL      string
	Interval time.Duration
	Client   *http.Client
	Logf     func(string, ...any)

	last int64
}

func (n *Notifier) logf(f string, a ...any) {
	if n.Logf != nil {
		n.Logf(f, a...)
	}
}

// Message dựng nội dung từ một dòng audit.
func Message(e store.AuditEntry) string {
	switch e.Event {
	case "incident_opened":
		var d struct {
			Alert string `json:"alert"`
			Key   string `json:"dedupe_key"`
		}
		_ = json.Unmarshal(e.Details, &d)
		return fmt.Sprintf("[mm-ops] Sự cố #%s mở: %s (%s)", e.SubjectID, d.Alert, d.Key)
	case "proposal_created":
		return fmt.Sprintf("[mm-ops] Đề xuất #%s chờ duyệt (tạo bởi %s)", e.SubjectID, e.Actor)
	}
	return fmt.Sprintf("[mm-ops] %s %s #%s", e.Event, e.SubjectType, e.SubjectID)
}

func (n *Notifier) client() *http.Client {
	if n.Client != nil {
		return n.Client
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (n *Notifier) post(ctx context.Context, text string) error {
	// "text" cho Slack/Mattermost, "content" cho Discord.
	b, _ := json.Marshal(map[string]string{"text": text, "content": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.URL, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client().Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("webhook trả %d", resp.StatusCode)
	}
	return nil
}

// Init đặt con trỏ tại id hiện tại.
func (n *Notifier) Init(ctx context.Context) error {
	id, err := n.Source.MaxAuditID(ctx)
	n.last = id
	return err
}

// Tick gửi các sự kiện mới. Lỗi gửi thì dừng và giữ con trỏ để thử lại ở lượt sau (không bỏ sót, có thể gửi trùng nếu lỗi sau khi nhận).
func (n *Notifier) Tick(ctx context.Context) (sent int, err error) {
	list, err := n.Source.AuditSince(ctx, n.last, Events, 50)
	if err != nil {
		return 0, err
	}
	for _, e := range list {
		if err := n.post(ctx, Message(e)); err != nil {
			return sent, err
		}
		n.last = e.ID
		sent++
	}
	return sent, nil
}

// Run chạy vòng lặp tới khi ctx hủy. URL rỗng thì không làm gì.
func (n *Notifier) Run(ctx context.Context) {
	if n.URL == "" {
		return
	}
	for ctx.Err() == nil {
		if err := n.Init(ctx); err == nil {
			break
		}
		time.Sleep(5 * time.Second)
	}
	t := time.NewTicker(n.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := n.Tick(ctx); err != nil {
				n.logf("notify: %v", err)
			}
		}
	}
}
