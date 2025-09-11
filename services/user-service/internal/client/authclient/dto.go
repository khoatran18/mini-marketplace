package authclient

type GetStoreIDRoleByIDInput struct {
	ID uint64
}
type GetStoreIDRoleByIDOutput struct {
	Message string
	StoreID uint64
	Role    string
	Success bool
}
