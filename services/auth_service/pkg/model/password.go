package model

type ChangePasswordRequest struct {
	Username    string
	OldPassword string
	NewPassword string
	Role        string
}
