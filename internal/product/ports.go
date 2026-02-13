package product

import (
	"context"

	"saruman/internal/domain"
)

type SearchUseCase interface {
	SearchProducts(ctx context.Context, req SearchProductsRequest) (*SearchProductsResponse, error)
}

type Service interface {
	GetProductsByIDsAndCompany(ctx context.Context, ids []int, companyID int) (found []domain.Product, notFoundIDs []int, err error)
}

type Repository interface {
	FindByIDsAndCompany(ctx context.Context, ids []int, companyID int) ([]domain.Product, error)
}
