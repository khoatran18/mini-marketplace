package dto

type RegisterInput struct {
	Username        string `json:"username" binding:"required"`
	Password        string `json:"password" binding:"required"`
	Role            string `json:"role" binding:"required"`
	StoreID         uint64 `json:"store_id"`
	RoleNotRegister string `json:"role_not_register"`
}
type RegisterOutput struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

type LoginInput struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Role     string `json:"role" binding:"required"`
}
type LoginOutput struct {
	Message      string `json:"message"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Success      bool   `json:"success"`
}

type ChangePasswordInput struct {
	Username    string
	OldPassword string
	NewPassword string
	Role        string
}
type ChangePasswordOutput struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

type RefreshTokenInput struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}
type RefreshTokenOutput struct {
	Message      string `json:"message"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Success      bool   `json:"success"`
}

type TokenRequest struct {
	UserID     uint64
	Username   string
	Role       string
	PwdVersion int64
}

type RegisterSellerRolesInput struct {
	SellerAdminID uint64
	Username      string
	Password      string
	Role          string
}
type RegisterSellerRolesOutput struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

type GetStoreIDRoleByIdInput struct {
	ID uint64
}
type GetStoreIDRoleByIdOutput struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
	StoreID uint64 `json:"store_id"`
	Role    string `json:"role"`
}
