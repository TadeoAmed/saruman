package product

import (
	"context"
)

type searchUseCase struct {
	service Service
}

func NewSearchUseCase(service Service) SearchUseCase {
	return &searchUseCase{service: service}
}

func (uc *searchUseCase) SearchProducts(ctx context.Context, req SearchProductsRequest) (*SearchProductsResponse, error) {
	found, notFoundIDs, err := uc.service.GetProductsByIDsAndCompany(ctx, req.ProductIDs, req.CompanyID)
	if err != nil {
		return nil, err
	}

	products := make([]ProductDTO, 0, len(found))
	for _, p := range found {
		products = append(products, ProductDTO{
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

	return &SearchProductsResponse{
		Products: products,
		NotFound: notFoundIDs,
	}, nil
}
