package port

import (
	"encoding/json"
	"time"
)

type ID int64

type Product struct {
	ID                    ID              `json:"id"`
	ProductCode           string          `json:"product_code"`
	Name                  string          `json:"name"`
	Description           string          `json:"description"`
	PriceMinor            int64           `json:"price_minor"`
	Currency              string          `json:"currency"`
	StockQuantity         int32           `json:"stock_quantity"`
	Images                []string        `json:"images"`
	CreatedBy             int64           `json:"created_by"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
	LegacyAdminProjection json.RawMessage `json:"legacy_admin_projection"`
}

type CreateCommand struct {
	ProductCode, Name, Description, Currency, IdempotencyKey string
	PriceMinor                                               int64
	StockQuantity                                            int32
	Images                                                   []string
	LegacyAdminProjection                                    json.RawMessage
	Actor                                                    int64
}

type Page struct {
	Items      []Product `json:"items"`
	NextCursor string    `json:"next_cursor"`
}

type LegacyPage struct {
	Items  []Product
	Total  int64
	Limit  int32
	Offset int32
}
