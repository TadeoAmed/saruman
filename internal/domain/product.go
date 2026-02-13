package domain

import "time"

type Product struct {
	ID            int
	ExternalID    int
	Name          string
	Description   string
	Price         float64
	Stock         *int
	ReservedStock *int
	CompanyID     int
	TypeID        int
	Category      string
	IsActive      bool
	IsDeleted     bool
	HasStock      bool
	Stockeable    bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (p Product) AvailableStock() int {
	if p.Stock == nil || p.ReservedStock == nil {
		return 0
	}
	available := *p.Stock - *p.ReservedStock
	if available < 0 {
		return 0
	}
	return available
}
