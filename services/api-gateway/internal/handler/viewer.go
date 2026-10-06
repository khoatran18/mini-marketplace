package handler

import (
	"api-gateway/internal/client/authclient"
	"api-gateway/pkg/dto"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Viewer is who is calling: anonymous (UserID 0), a user, or a seller whose store was resolved from auth-service.
type Viewer struct {
	UserID  uint64
	Role    string
	StoreID uint64 // only for seller roles with a store
}

// IsSeller reports whether the viewer acts for a store.
func (v Viewer) IsSeller() bool { return v.Role == "seller_admin" || v.Role == "seller_employee" }

// IsAdmin reports whether the viewer is a platform administrator.
func (v Viewer) IsAdmin() bool { return v.Role == "admin" }

// Authenticated reports whether a user is signed in.
func (v Viewer) Authenticated() bool { return v.UserID != 0 }

// Owns reports whether the viewer manages the given store (sellers of that store) or is an admin.
func (v Viewer) Owns(storeID uint64) bool {
	return v.IsAdmin() || (v.IsSeller() && v.StoreID != 0 && v.StoreID == storeID)
}

// resolveViewer builds the Viewer from the identity set by (Optional)AuthMiddleware. The store is looked up
// through auth-service only for seller roles. Errors from that lookup are returned to the caller.
func resolveViewer(c *gin.Context, auth *authclient.AuthClient) (Viewer, error) {
	v := Viewer{}
	id, ok := currentUserID(c)
	if !ok {
		return v, nil
	}
	_, role, _ := currentUser(c)
	v.UserID, v.Role = id, role
	if v.IsSeller() && auth != nil {
		storeID, _, err := auth.GetStoreID(id)
		if err != nil {
			return v, err
		}
		v.StoreID = storeID
	}
	return v, nil
}

// requireSellerStore resolves the caller's store and writes the error response when there is none.
func requireSellerStore(c *gin.Context, auth *authclient.AuthClient) (Viewer, bool) {
	v, err := resolveViewer(c, auth)
	if err != nil {
		c.JSON(httpStatusFromError(err), dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return v, false
	}
	if !v.IsSeller() {
		c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "Only sellers can do this"})
		return v, false
	}
	if v.StoreID == 0 {
		c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "Create a store first"})
		return v, false
	}
	return v, true
}

// publicStockCap is the highest stock number shown to people who do not manage the product ("20+").
const publicStockCap = 20

// maskProduct hides stock internals from people who do not manage the product: the available stock is capped,
// reserved stock and the low-stock threshold are removed (the stock_level stays).
func maskProduct(p *dto.Product, v Viewer) {
	if p == nil || v.Owns(p.SellerID) {
		return
	}
	if p.Inventory > publicStockCap {
		p.Inventory = publicStockCap
	}
	p.Reserved, p.LowStockThreshold, p.Version = 0, 0, 0
}
