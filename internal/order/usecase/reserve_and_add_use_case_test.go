package usecase

import (
	"context"
	"testing"

	"github.com/go-sql-driver/mysql"
	"saruman/internal/domain"
	dtoerrors "saruman/internal/errors"
	"saruman/internal/dto"
	"go.uber.org/zap"
)

// Helper to create a MySQL deadlock error for testing
func createDeadlockError() error {
	return &mysql.MySQLError{Number: 1213}
}

// Helper to create ReserveAndAddUseCase with test defaults
func newTestReserveAndAddUseCase(
	orderRepo OrderRepository,
	companyConfigRepo CompanyConfigRepository,
	reservationSvc StockReservationService,
) *ReserveAndAddUseCase {
	return NewReserveAndAddUseCase(
		orderRepo,
		companyConfigRepo,
		reservationSvc,
		zap.NewNop(),
		3, // Default max retry attempts
	)
}

// Mock implementations
type mockOrderRepository struct {
	FindByIDFunc func(ctx context.Context, id uint) (*domain.Order, error)
}

func (m *mockOrderRepository) FindByID(ctx context.Context, id uint) (*domain.Order, error) {
	return m.FindByIDFunc(ctx, id)
}

type mockCompanyConfigRepository struct {
	FindByCompanyIDFunc func(ctx context.Context, companyID int) (*domain.CompanyConfig, error)
}

func (m *mockCompanyConfigRepository) FindByCompanyID(ctx context.Context, companyID int) (*domain.CompanyConfig, error) {
	return m.FindByCompanyIDFunc(ctx, companyID)
}

type mockStockReservationService struct {
	ReserveItemsFunc func(ctx context.Context, orderID uint, companyID int, items []dto.ReservationItem, hasStockControl bool) (*dto.ReservationResult, error)
}

func (m *mockStockReservationService) ReserveItems(ctx context.Context, orderID uint, companyID int, items []dto.ReservationItem, hasStockControl bool) (*dto.ReservationResult, error) {
	return m.ReserveItemsFunc(ctx, orderID, companyID, items, hasStockControl)
}

// Tests

func TestReserveItems_OrderNotFound(t *testing.T) {
	ctx := context.Background()

	orderRepo := &mockOrderRepository{
		FindByIDFunc: func(ctx context.Context, id uint) (*domain.Order, error) {
			return nil, dtoerrors.NewNotFoundError("order not found")
		},
	}

	companyConfigRepo := &mockCompanyConfigRepository{}
	reservationSvc := &mockStockReservationService{}

	uc := newTestReserveAndAddUseCase(orderRepo, companyConfigRepo, reservationSvc)

	_, err := uc.ReserveItems(ctx, 1, 1, []dto.ReservationItem{})

	if err == nil {
		t.Errorf("expected error, got nil")
	}

	if _, ok := dtoerrors.IsNotFoundError(err); !ok {
		t.Errorf("expected NotFoundError, got %T", err)
	}
}

func TestReserveItems_OrderNotPending(t *testing.T) {
	ctx := context.Background()

	orderRepo := &mockOrderRepository{
		FindByIDFunc: func(ctx context.Context, id uint) (*domain.Order, error) {
			return &domain.Order{
				ID:        id,
				CompanyID: 1,
				Status:    domain.OrderStatusCreated, // Not pending
			}, nil
		},
	}

	companyConfigRepo := &mockCompanyConfigRepository{}
	reservationSvc := &mockStockReservationService{}

	uc := newTestReserveAndAddUseCase(orderRepo, companyConfigRepo, reservationSvc)

	_, err := uc.ReserveItems(ctx, 1, 1, []dto.ReservationItem{})

	if err == nil {
		t.Errorf("expected error, got nil")
	}

	if _, ok := dtoerrors.IsConflictError(err); !ok {
		t.Errorf("expected ConflictError, got %T", err)
	}
}

func TestReserveItems_CompanyMismatch(t *testing.T) {
	ctx := context.Background()

	orderRepo := &mockOrderRepository{
		FindByIDFunc: func(ctx context.Context, id uint) (*domain.Order, error) {
			return &domain.Order{
				ID:        id,
				CompanyID: 2, // Different company
				Status:    domain.OrderStatusPending,
			}, nil
		},
	}

	companyConfigRepo := &mockCompanyConfigRepository{}
	reservationSvc := &mockStockReservationService{}

	uc := newTestReserveAndAddUseCase(orderRepo, companyConfigRepo, reservationSvc)

	_, err := uc.ReserveItems(ctx, 1, 1, []dto.ReservationItem{})

	if err == nil {
		t.Errorf("expected error, got nil")
	}

	if _, ok := dtoerrors.IsForbiddenError(err); !ok {
		t.Errorf("expected ForbiddenError, got %T", err)
	}
}

func TestReserveItems_CompanyConfigNotFound(t *testing.T) {
	ctx := context.Background()

	orderRepo := &mockOrderRepository{
		FindByIDFunc: func(ctx context.Context, id uint) (*domain.Order, error) {
			return &domain.Order{
				ID:        id,
				CompanyID: 1,
				Status:    domain.OrderStatusPending,
			}, nil
		},
	}

	companyConfigRepo := &mockCompanyConfigRepository{
		FindByCompanyIDFunc: func(ctx context.Context, companyID int) (*domain.CompanyConfig, error) {
			return nil, dtoerrors.NewNotFoundError("company config not found")
		},
	}

	reservationSvc := &mockStockReservationService{}

	uc := newTestReserveAndAddUseCase(orderRepo, companyConfigRepo, reservationSvc)

	_, err := uc.ReserveItems(ctx, 1, 1, []dto.ReservationItem{})

	if err == nil {
		t.Errorf("expected error, got nil")
	}

	if _, ok := dtoerrors.IsNotFoundError(err); !ok {
		t.Errorf("expected NotFoundError, got %T", err)
	}
}

func TestReserveItems_AllSuccess(t *testing.T) {
	ctx := context.Background()

	orderRepo := &mockOrderRepository{
		FindByIDFunc: func(ctx context.Context, id uint) (*domain.Order, error) {
			return &domain.Order{
				ID:        id,
				CompanyID: 1,
				Status:    domain.OrderStatusPending,
			}, nil
		},
	}

	companyConfigRepo := &mockCompanyConfigRepository{
		FindByCompanyIDFunc: func(ctx context.Context, companyID int) (*domain.CompanyConfig, error) {
			return &domain.CompanyConfig{
				CompanyID: companyID,
				HasStock:  true,
			}, nil
		},
	}

	reservationSvc := &mockStockReservationService{
		ReserveItemsFunc: func(ctx context.Context, orderID uint, companyID int, items []dto.ReservationItem, hasStockControl bool) (*dto.ReservationResult, error) {
			return &dto.ReservationResult{
				Status:  dto.ReservationAllSuccess,
				OrderID: orderID,
				Successes: []dto.ItemSuccess{
					{ProductID: 1, Quantity: 10},
					{ProductID: 2, Quantity: 20},
				},
				TotalPrice: 1500.0,
			}, nil
		},
	}

	uc := newTestReserveAndAddUseCase(orderRepo, companyConfigRepo, reservationSvc)

	items := []dto.ReservationItem{
		{ProductID: 1, Quantity: 10, Price: 100.0},
		{ProductID: 2, Quantity: 20, Price: 50.0},
	}

	result, err := uc.ReserveItems(ctx, 1, 1, items)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result.Status != dto.ReservationAllSuccess {
		t.Errorf("expected ALL_SUCCESS, got %s", result.Status)
	}

	if len(result.Successes) != 2 {
		t.Errorf("expected 2 successes, got %d", len(result.Successes))
	}

	if result.TotalPrice != 1500.0 {
		t.Errorf("expected total price 1500.0, got %f", result.TotalPrice)
	}
}

func TestReserveItems_AllFailed(t *testing.T) {
	ctx := context.Background()

	orderRepo := &mockOrderRepository{
		FindByIDFunc: func(ctx context.Context, id uint) (*domain.Order, error) {
			return &domain.Order{
				ID:        id,
				CompanyID: 1,
				Status:    domain.OrderStatusPending,
			}, nil
		},
	}

	companyConfigRepo := &mockCompanyConfigRepository{
		FindByCompanyIDFunc: func(ctx context.Context, companyID int) (*domain.CompanyConfig, error) {
			return &domain.CompanyConfig{
				CompanyID: companyID,
				HasStock:  true,
			}, nil
		},
	}

	reservationSvc := &mockStockReservationService{
		ReserveItemsFunc: func(ctx context.Context, orderID uint, companyID int, items []dto.ReservationItem, hasStockControl bool) (*dto.ReservationResult, error) {
			return &dto.ReservationResult{
				Status:  dto.ReservationAllFailed,
				OrderID: orderID,
				Failures: []dto.ItemFailure{
					{ProductID: 1, Quantity: 10, Reason: dto.ReasonNotFound},
					{ProductID: 2, Quantity: 20, Reason: dto.ReasonOutOfStock},
				},
			}, nil
		},
	}

	uc := newTestReserveAndAddUseCase(orderRepo, companyConfigRepo, reservationSvc)

	items := []dto.ReservationItem{
		{ProductID: 1, Quantity: 10, Price: 100.0},
		{ProductID: 2, Quantity: 20, Price: 50.0},
	}

	result, err := uc.ReserveItems(ctx, 1, 1, items)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result.Status != dto.ReservationAllFailed {
		t.Errorf("expected ALL_FAILED, got %s", result.Status)
	}

	if len(result.Failures) != 2 {
		t.Errorf("expected 2 failures, got %d", len(result.Failures))
	}
}

func TestReserveItems_ItemsSortedByProductID(t *testing.T) {
	ctx := context.Background()

	var receivedItems []dto.ReservationItem

	orderRepo := &mockOrderRepository{
		FindByIDFunc: func(ctx context.Context, id uint) (*domain.Order, error) {
			return &domain.Order{
				ID:        id,
				CompanyID: 1,
				Status:    domain.OrderStatusPending,
			}, nil
		},
	}

	companyConfigRepo := &mockCompanyConfigRepository{
		FindByCompanyIDFunc: func(ctx context.Context, companyID int) (*domain.CompanyConfig, error) {
			return &domain.CompanyConfig{
				CompanyID: companyID,
				HasStock:  true,
			}, nil
		},
	}

	reservationSvc := &mockStockReservationService{
		ReserveItemsFunc: func(ctx context.Context, orderID uint, companyID int, items []dto.ReservationItem, hasStockControl bool) (*dto.ReservationResult, error) {
			receivedItems = items
			return &dto.ReservationResult{
				Status:  dto.ReservationAllSuccess,
				OrderID: orderID,
			}, nil
		},
	}

	uc := newTestReserveAndAddUseCase(orderRepo, companyConfigRepo, reservationSvc)

	items := []dto.ReservationItem{
		{ProductID: 3, Quantity: 10, Price: 100.0},
		{ProductID: 1, Quantity: 20, Price: 50.0},
		{ProductID: 2, Quantity: 5, Price: 75.0},
	}

	_, err := uc.ReserveItems(ctx, 1, 1, items)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Verify items are sorted by ProductID ASC
	if len(receivedItems) != 3 {
		t.Errorf("expected 3 items, got %d", len(receivedItems))
	}

	if receivedItems[0].ProductID != 1 {
		t.Errorf("expected first product ID to be 1, got %d", receivedItems[0].ProductID)
	}

	if receivedItems[1].ProductID != 2 {
		t.Errorf("expected second product ID to be 2, got %d", receivedItems[1].ProductID)
	}

	if receivedItems[2].ProductID != 3 {
		t.Errorf("expected third product ID to be 3, got %d", receivedItems[2].ProductID)
	}
}

func TestReserveItems_DeadlockRetry(t *testing.T) {
	ctx := context.Background()

	attemptCount := 0

	orderRepo := &mockOrderRepository{
		FindByIDFunc: func(ctx context.Context, id uint) (*domain.Order, error) {
			return &domain.Order{
				ID:        id,
				CompanyID: 1,
				Status:    domain.OrderStatusPending,
			}, nil
		},
	}

	companyConfigRepo := &mockCompanyConfigRepository{
		FindByCompanyIDFunc: func(ctx context.Context, companyID int) (*domain.CompanyConfig, error) {
			return &domain.CompanyConfig{
				CompanyID: companyID,
				HasStock:  true,
			}, nil
		},
	}

	reservationSvc := &mockStockReservationService{
		ReserveItemsFunc: func(ctx context.Context, orderID uint, companyID int, items []dto.ReservationItem, hasStockControl bool) (*dto.ReservationResult, error) {
			attemptCount++
			if attemptCount == 1 {
				// First attempt: deadlock
				return nil, createDeadlockError()
			}
			// Second attempt: success
			return &dto.ReservationResult{
				Status:  dto.ReservationAllSuccess,
				OrderID: orderID,
			}, nil
		},
	}

	uc := newTestReserveAndAddUseCase(orderRepo, companyConfigRepo, reservationSvc)

	items := []dto.ReservationItem{
		{ProductID: 1, Quantity: 10, Price: 100.0},
	}

	result, err := uc.ReserveItems(ctx, 1, 1, items)

	if err != nil {
		t.Errorf("expected no error on retry success, got %v", err)
	}

	if result == nil {
		t.Errorf("expected non-nil result")
	}

	if attemptCount != 2 {
		t.Errorf("expected 2 attempts, got %d", attemptCount)
	}
}

func TestReserveItems_DeadlockMaxRetries(t *testing.T) {
	ctx := context.Background()

	attemptCount := 0

	orderRepo := &mockOrderRepository{
		FindByIDFunc: func(ctx context.Context, id uint) (*domain.Order, error) {
			return &domain.Order{
				ID:        id,
				CompanyID: 1,
				Status:    domain.OrderStatusPending,
			}, nil
		},
	}

	companyConfigRepo := &mockCompanyConfigRepository{
		FindByCompanyIDFunc: func(ctx context.Context, companyID int) (*domain.CompanyConfig, error) {
			return &domain.CompanyConfig{
				CompanyID: companyID,
				HasStock:  true,
			}, nil
		},
	}

	reservationSvc := &mockStockReservationService{
		ReserveItemsFunc: func(ctx context.Context, orderID uint, companyID int, items []dto.ReservationItem, hasStockControl bool) (*dto.ReservationResult, error) {
			attemptCount++
			// Always deadlock
			return nil, createDeadlockError()
		},
	}

	uc := newTestReserveAndAddUseCase(orderRepo, companyConfigRepo, reservationSvc)

	items := []dto.ReservationItem{
		{ProductID: 1, Quantity: 10, Price: 100.0},
	}

	_, err := uc.ReserveItems(ctx, 1, 1, items)

	if err == nil {
		t.Errorf("expected error after max retries, got nil")
	}

	if _, ok := dtoerrors.IsDeadlockError(err); !ok {
		t.Errorf("expected DeadlockError, got %T", err)
	}

	if attemptCount != 3 {
		t.Errorf("expected 3 attempts, got %d", attemptCount)
	}
}
