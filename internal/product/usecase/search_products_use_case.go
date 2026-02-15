package usecase

import (
	"context"

	"saruman/internal/domain"
	"saruman/internal/dto"
)

type Service interface {
	GetProductsByIDsAndCompany(ctx context.Context, ids []int, companyID int) (found []domain.Product, notFoundIDs []int, err error)
}

type SearchUseCase struct {
	service Service
}

func NewSearchUseCase(service Service) *SearchUseCase {
	return &SearchUseCase{service: service}
}

func (uc *SearchUseCase) SearchProducts(ctx context.Context, req dto.SearchProductsRequest) (*dto.SearchProductsResponse, error) {
	found, notFoundIDs, err := uc.service.GetProductsByIDsAndCompany(ctx, req.ProductIDs, req.CompanyID)
	if err != nil {
		return nil, err
	}

	products := make([]dto.ProductDTO, 0, len(found))
	for _, p := range found {
		products = append(products, dto.ProductDTO{
			ID:             p.ID,
			ExternalID:     p.ExternalID,
			Name:           p.Name,
			Description:    p.Description,
			Price:          p.Price,
			Stock:          p.Stock,
			ReservedStock:  p.ReservedStock,
			AvailableStock: p.AvailableStock(),
			Category:       p.Category,
			IsActive:       p.IsActive,
			HasStock:       p.HasStock,
			Stockeable:     p.Stockeable,
		})
	}

	if notFoundIDs == nil {
		notFoundIDs = []int{}
	}

	return &dto.SearchProductsResponse{
		Products: products,
		NotFound: notFoundIDs,
	}, nil
}
