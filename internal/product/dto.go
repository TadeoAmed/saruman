package product

type SearchProductsRequest struct {
	CompanyID  int   `json:"companyId"`
	ProductIDs []int `json:"productIds"`
}

type SearchProductsResponse struct {
	Products []ProductDTO `json:"products"`
	NotFound []int        `json:"notFound"`
}

type ProductDTO struct {
	ID             int     `json:"id"`
	ExternalID     int     `json:"externalId"`
	Name           string  `json:"name"`
	Description    string  `json:"description"`
	Price          float64 `json:"price"`
	Stock          *int    `json:"stock"`
	ReservedStock  *int    `json:"reservedStock"`
	AvailableStock int     `json:"availableStock"`
	Category       string  `json:"category"`
	IsActive       bool    `json:"isActive"`
	HasStock       bool    `json:"hasStock"`
	Stockeable     bool    `json:"stockeable"`
}
