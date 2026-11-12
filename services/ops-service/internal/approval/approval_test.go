package approval

import (
	"testing"
	"time"
)

func TestIssueVerify(t *testing.T) {
	s := New("secret")
	now := time.Now()
	tok := s.Issue(5, "hashA", "admin:2", now, 5*time.Minute)
	if err := s.Verify(tok, 5, "hashA", "admin:2", now.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	for name, err := range map[string]error{
		"hết hạn":         s.Verify(tok, 5, "hashA", "admin:2", now.Add(6*time.Minute)),
		"đề xuất khác":    s.Verify(tok, 6, "hashA", "admin:2", now),
		"nội dung đã đổi": s.Verify(tok, 5, "hashB", "admin:2", now),
		"người khác":      s.Verify(tok, 5, "hashA", "admin:3", now),
		"rỗng":            s.Verify("", 5, "hashA", "admin:2", now),
		"không có chữ ký": s.Verify("abc", 5, "hashA", "admin:2", now),
		"chữ ký bị sửa":   s.Verify(tok[:len(tok)-2]+"xx", 5, "hashA", "admin:2", now),
		"khoá khác":       New("other").Verify(tok, 5, "hashA", "admin:2", now),
	} {
		if err != ErrInvalid {
			t.Errorf("%s: phải ErrInvalid, được %v", name, err)
		}
	}
}
