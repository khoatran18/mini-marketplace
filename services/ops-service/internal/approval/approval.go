// Package approval tạo và kiểm mã duyệt: bằng chứng rằng đúng người này đã XEM đúng nội dung (băm) này trong vòng vài phút.
// Mã được cấp bởi POST /proposals/{id}/review và bắt buộc khi quyết định; nội dung đề xuất bất biến nên mã cũng gắn với băm.
package approval

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// ErrInvalid cho mọi lỗi mã (chữ ký sai, hết hạn, không khớp đề xuất / băm / người): không nói rõ lý do cho bên ngoài.
var ErrInvalid = errors.New("invalid approval token")

// Signer ký bằng khoá dẫn xuất từ bí mật dùng chung (OPS_API_TOKEN).
type Signer struct{ key []byte }

// New dẫn xuất khoá riêng cho mục đích này (không dùng thẳng bí mật làm khoá HMAC).
func New(secret string) *Signer {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte("ops-proposal-approval-v1"))
	return &Signer{key: m.Sum(nil)}
}

type payload struct {
	P int64  `json:"p"`
	H string `json:"h"`
	A string `json:"a"`
	E int64  `json:"e"`
}

func (s *Signer) mac(b []byte) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write(b)
	return m.Sum(nil)
}

// Issue cấp mã cho (đề xuất, băm, người xem) hiệu lực ttl.
func (s *Signer) Issue(proposalID int64, hash, actor string, now time.Time, ttl time.Duration) string {
	b, _ := json.Marshal(payload{proposalID, hash, actor, now.Add(ttl).Unix()})
	return base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(s.mac(b))
}

// Verify kiểm mã với đề xuất, băm và người hiện tại.
func (s *Signer) Verify(token string, proposalID int64, hash, actor string, now time.Time) error {
	body, sig, ok := strings.Cut(token, ".")
	if !ok {
		return ErrInvalid
	}
	b, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return ErrInvalid
	}
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(got, s.mac(b)) {
		return ErrInvalid
	}
	var p payload
	if json.Unmarshal(b, &p) != nil || p.P != proposalID || p.H != hash || p.A != actor || now.Unix() > p.E {
		return ErrInvalid
	}
	return nil
}
