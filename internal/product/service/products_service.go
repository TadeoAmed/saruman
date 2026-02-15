package service

import (
	"context"

	"saruman/internal/domain"
)

type Repository interface {
	FindByIDsAndCompany(ctx context.Context, ids []int, companyID int) ([]domain.Product, error)
}

type ProductService struct {
	repo Repository
}

func NewService(repo Repository) *ProductService {
	return &ProductService{repo: repo}
}

func (s *ProductService) GetProductsByIDsAndCompany(ctx context.Context, ids []int, companyID int) ([]domain.Product, []int, error) {
	found, err := s.repo.FindByIDsAndCompany(ctx, ids, companyID)
	if err != nil {
		return nil, nil, err
	}

	foundSet := make(map[int]struct{}, len(found))
	for _, p := range found {
		foundSet[p.ID] = struct{}{}
	}

	var notFoundIDs []int
	for _, id := range ids {
		if _, ok := foundSet[id]; !ok {
			notFoundIDs = append(notFoundIDs, id)
		}
	}

	return found, notFoundIDs, nil
}
