package repository

import (
	"context"
	"errors"
	"fmt"
	"product-service/pkg/model"
	"strings"
	"unicode"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SearchParams are the filters of a catalog search.
type SearchParams struct {
	Query              string
	CategoryID         uint64
	PriceMin, PriceMax float64
	Brand              string
	SellerID           uint64
	InStockOnly        bool
	Sort               string // relevance | price_asc | price_desc | newest | best_selling
	Page, PageSize     uint64
	IncludeAllStatuses bool
}

// Sort options.
const (
	SortRelevance   = "relevance"
	SortPriceAsc    = "price_asc"
	SortPriceDesc   = "price_desc"
	SortNewest      = "newest"
	SortBestSelling = "best_selling"
)

// tsQuery turns free text into a prefix tsquery ("áo khoác" -> "ao:* & khoac:*"); only letters and digits survive,
// so user input can never inject tsquery operators. Returns "" when nothing searchable is left.
func tsQuery(text string) string {
	words := strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	if len(words) > 8 {
		words = words[:8]
	}
	terms := make([]string, 0, len(words))
	for _, w := range words {
		terms = append(terms, w+":*")
	}
	return strings.Join(terms, " & ")
}

// SearchProducts runs a keyword search (accent-insensitive Postgres full text with prefix matching) with filters.
func (r *ProductRepository) SearchProducts(ctx context.Context, p SearchParams) ([]*model.Product, int64, error) {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 || p.PageSize > MaxPageSize {
		p.PageSize = 20
	}
	q := r.DB.WithContext(ctx).Model(&model.Product{})
	if !p.IncludeAllStatuses {
		q = q.Where("status = ?", model.StatusActive)
	}
	if p.SellerID != 0 {
		q = q.Where("seller_id = ?", p.SellerID)
	}
	if p.CategoryID != 0 {
		ids, err := r.categoryWithDescendants(ctx, p.CategoryID)
		if err != nil {
			return nil, 0, err
		}
		q = q.Where("category_id IN ?", ids)
	}
	if p.PriceMin > 0 {
		q = q.Where("price >= ?", p.PriceMin)
	}
	if p.PriceMax > 0 {
		q = q.Where("price <= ?", p.PriceMax)
	}
	if b := strings.TrimSpace(p.Brand); b != "" {
		q = q.Where("mm_unaccent(brand) = mm_unaccent(?)", b)
	}
	if p.InStockOnly {
		q = q.Where("inventory > 0")
	}
	ts := tsQuery(p.Query)
	if ts != "" {
		q = q.Where("to_tsvector('simple', "+searchDocument+") @@ to_tsquery('simple', mm_unaccent(?))", ts)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	switch {
	case p.Sort == SortPriceAsc:
		q = q.Order("price ASC, id ASC")
	case p.Sort == SortPriceDesc:
		q = q.Order("price DESC, id ASC")
	case p.Sort == SortNewest:
		q = q.Order("created_at DESC, id DESC")
	case p.Sort == SortBestSelling:
		q = q.Order("sold DESC, id ASC")
	case ts != "":
		// relevance: name matches first, then ts_rank, then best sellers
		q = q.Order(clause.Expr{SQL: "(mm_unaccent(name) LIKE '%' || mm_unaccent(?) || '%') DESC, ts_rank_cd(to_tsvector('simple', " + searchDocument + "), to_tsquery('simple', mm_unaccent(?))) DESC, sold DESC, id ASC", Vars: []interface{}{strings.TrimSpace(p.Query), ts}})
	default:
		q = q.Order("id ASC")
	}

	var products []*model.Product
	err := q.Limit(int(p.PageSize)).Offset(int((p.Page - 1) * p.PageSize)).Find(&products).Error
	return products, total, err
}

// ---- categories --------------------------------------------------------------------------

// ListCategories returns the category tree rows (flat, ordered by parent then sort) with the number of active
// products in each category including its descendants.
func (r *ProductRepository) ListCategories(ctx context.Context, includeInactive bool) ([]*model.Category, map[uint64]int64, error) {
	var cats []*model.Category
	q := r.DB.WithContext(ctx).Order("parent_id ASC, sort ASC, id ASC")
	if !includeInactive {
		q = q.Where("active = ?", true)
	}
	if err := q.Find(&cats).Error; err != nil {
		return nil, nil, err
	}
	var rows []struct {
		CategoryID uint64
		N          int64
	}
	if err := r.DB.WithContext(ctx).Model(&model.Product{}).Select("category_id, count(*) AS n").
		Where("status = ? AND category_id <> 0", model.StatusActive).Group("category_id").Scan(&rows).Error; err != nil {
		return nil, nil, err
	}
	direct := map[uint64]int64{}
	for _, row := range rows {
		direct[row.CategoryID] = row.N
	}
	parent := map[uint64]uint64{}
	for _, c := range cats {
		parent[c.ID] = c.ParentID
	}
	counts := map[uint64]int64{}
	for id, n := range direct {
		for cur, hops := id, 0; cur != 0 && hops < 10; cur, hops = parent[cur], hops+1 {
			counts[cur] += n
		}
	}
	return cats, counts, nil
}

func (r *ProductRepository) categoryWithDescendants(ctx context.Context, id uint64) ([]uint64, error) {
	var cats []*model.Category
	if err := r.DB.WithContext(ctx).Select("id, parent_id").Find(&cats).Error; err != nil {
		return nil, err
	}
	children := map[uint64][]uint64{}
	for _, c := range cats {
		children[c.ParentID] = append(children[c.ParentID], c.ID)
	}
	out := []uint64{id}
	for i := 0; i < len(out) && len(out) < 1000; i++ {
		out = append(out, children[out[i]]...)
	}
	return out, nil
}

// CategoryExists reports whether an active category with that id exists.
func (r *ProductRepository) CategoryExists(ctx context.Context, id uint64) (bool, error) {
	var n int64
	err := r.DB.WithContext(ctx).Model(&model.Category{}).Where("id = ? AND active = ?", id, true).Count(&n).Error
	return n > 0, err
}

// ErrCategory* are returned by UpsertCategory.
var (
	ErrCategoryNotFound = errors.New("category not found")
	ErrCategoryCycle    = errors.New("a category can not be its own ancestor")
	ErrSlugTaken        = errors.New("slug already in use")
)

// UpsertCategory creates (ID 0) or updates a category. The depth is limited to 3 levels and cycles are rejected.
func (r *ProductRepository) UpsertCategory(ctx context.Context, c *model.Category) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cats []*model.Category
		if err := tx.Find(&cats).Error; err != nil {
			return err
		}
		byID := map[uint64]*model.Category{}
		for _, x := range cats {
			byID[x.ID] = x
			if x.Slug == c.Slug && x.ID != c.ID {
				return ErrSlugTaken
			}
		}
		if c.ID != 0 && byID[c.ID] == nil {
			return ErrCategoryNotFound
		}
		if c.ParentID != 0 && byID[c.ParentID] == nil {
			return ErrCategoryNotFound
		}
		// walk up from the parent: must not meet c, and the chain (with c) may have at most 3 levels
		depth := 1
		for cur, hops := c.ParentID, 0; cur != 0; cur, hops = byID[cur].ParentID, hops+1 {
			if cur == c.ID || hops > 5 {
				return ErrCategoryCycle
			}
			depth++
		}
		if depth > 3 {
			return fmt.Errorf("categories can have at most 3 levels")
		}
		if c.ID == 0 {
			return tx.Create(c).Error
		}
		return tx.Model(&model.Category{}).Where("id = ?", c.ID).Updates(map[string]interface{}{
			"parent_id": c.ParentID, "name": c.Name, "slug": c.Slug, "sort": c.Sort, "active": c.Active,
		}).Error
	})
}

// ---- status, manual stock, ledger, low stock ------------------------------------------------

// SetProductStatus changes the visibility of a product. A store may only touch its own products and can not
// set or clear "banned"; an admin can do both.
func (r *ProductRepository) SetProductStatus(ctx context.Context, productID, storeID uint64, isAdmin bool, status string) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var p model.Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", productID).First(&p).Error; err != nil {
			return err
		}
		if !isAdmin {
			if storeID == 0 || p.SellerID != storeID {
				return ErrNotOwner
			}
			if status == model.StatusBanned || p.Status == model.StatusBanned {
				return ErrBannedByAdmin
			}
		}
		if p.Status == status {
			return nil
		}
		if err := tx.Model(&p).Updates(map[string]interface{}{"status": status, "version": gorm.Expr("version + 1")}).Error; err != nil {
			return err
		}
		p.Status = status
		return emitProductChanged(tx, &p, "upsert")
	})
}

// ErrBannedByAdmin is returned when a seller tries to ban/unban.
var ErrBannedByAdmin = errors.New("only an administrator can ban or unban a product")

// AdjustInventory applies a manual stock correction by the owner store and returns the new available stock.
func (r *ProductRepository) AdjustInventory(ctx context.Context, productID, storeID uint64, delta int64, reason string) (int64, error) {
	var out int64
	err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var p model.Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", productID).First(&p).Error; err != nil {
			return err
		}
		if storeID == 0 || p.SellerID != storeID {
			return ErrNotOwner
		}
		kind := model.ReasonAdjust
		if delta > 0 {
			kind = model.ReasonRestock
		}
		row, err := applyStock(tx, productID, delta, 0, 0, kind, "manual", reason)
		if err != nil {
			return err
		}
		out = row.Inventory
		return nil
	})
	return out, err
}

// GetLedger returns the inventory history of a product owned by storeID, newest first.
func (r *ProductRepository) GetLedger(ctx context.Context, productID, storeID uint64, page, pageSize uint64) ([]*model.InventoryLedger, error) {
	var p model.Product
	if err := r.DB.WithContext(ctx).Select("id, seller_id").Where("id = ?", productID).First(&p).Error; err != nil {
		return nil, err
	}
	if storeID == 0 || p.SellerID != storeID {
		return nil, ErrNotOwner
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > MaxPageSize {
		pageSize = 50
	}
	var entries []*model.InventoryLedger
	err := r.DB.WithContext(ctx).Where("product_id = ?", productID).Order("id DESC").
		Limit(int(pageSize)).Offset(int((page - 1) * pageSize)).Find(&entries).Error
	return entries, err
}

// ListLowStock returns a store's products whose available stock is at or below their threshold.
func (r *ProductRepository) ListLowStock(ctx context.Context, storeID uint64, defaultThreshold int64) ([]*model.Product, error) {
	if defaultThreshold <= 0 {
		defaultThreshold = model.DefaultLowStockThreshold
	}
	var products []*model.Product
	err := r.DB.WithContext(ctx).
		Where("seller_id = ? AND status IN ? AND inventory <= COALESCE(NULLIF(low_stock_threshold, 0), ?)", storeID, []string{model.StatusActive, model.StatusHidden}, defaultThreshold).
		Order("inventory ASC, id ASC").Limit(200).Find(&products).Error
	return products, err
}
