package dto

type ChangePwdKafkaEvent struct {
	UserID     uint64 `json:"user_id"`
	PwdVersion int64  `json:"pwd_version"`
}
