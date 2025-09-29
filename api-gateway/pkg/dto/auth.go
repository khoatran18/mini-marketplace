package dto

type LoginInput struct {
	Username string `json:"username" binding:"required" example:"user1"`
	Password string `json:"password" binding:"required" example:"password1"`
	Role     string `json:"role" binding:"required" example:"admin"`
}
type LoginOutput struct {
	Message      string `json:"message"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Success      bool   `json:"success"`
}

type RegisterInput struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Role     string `json:"role" binding:"required"`
}
type RegisterOutput struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

type ChangePasswordInput struct {
	Username    string `json:"username"`
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
	Role        string `json:"role"`
}
type ChangePasswordOutput struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

type RefreshTokenInput struct {
	RefreshToken string `json:"refresh_token"`
}
type RefreshTokenOutput struct {
	Message      string `json:"message"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Success      bool   `json:"success"`
}

type RegisterSellerRolesInput struct {
	SellerAdminID uint64 `json:"seller_admin_id"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	Role          string `json:"role"`
}
type RegisterSellerRolesOutput struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}
