package service

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"saruman/internal/domain"
	dtoerrors "saruman/internal/errors"
	"saruman/internal/dto"
	"go.uber.org/zap"
)

// Helper function to convert int to *int
func intPtr(i int) *int {
	return &i
}

// Mock implementations
type mockTransactionManager struct {
	BeginTxFunc func(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

func (m *mockTransactionManager) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return m.BeginTxFunc(ctx, opts)
}

type mockProductRepository struct {
	FindByIDForUpdateFunc      func(ctx context.Context, tx *sql.Tx, productID int, companyID int) (*domain.Product, error)
	IncrementReservedStockFunc func(ctx context.Context, tx *sql.Tx, productID int, quantity int) error
}

func (m *mockProductRepository) FindByIDForUpdate(ctx context.Context, tx *sql.Tx, productID int, companyID int) (*domain.Product, error) {
	return m.FindByIDForUpdateFunc(ctx, tx, productID, companyID)
}

func (m *mockProductRepository) IncrementReservedStock(ctx context.Context, tx *sql.Tx, productID int, quantity int) error {
	return m.IncrementReservedStockFunc(ctx, tx, productID, quantity)
}

type mockOrderItemRepository struct {
	InsertFunc func(ctx context.Context, tx *sql.Tx, item domain.OrderItem) (uint, error)
}

func (m *mockOrderItemRepository) Insert(ctx context.Context, tx *sql.Tx, item domain.OrderItem) (uint, error) {
	return m.InsertFunc(ctx, tx, item)
}

type mockOrderRepository struct {
	UpdateStatusFunc     func(ctx context.Context, tx *sql.Tx, id uint, status string) error
	UpdateTotalPriceFunc func(ctx context.Context, tx *sql.Tx, id uint, totalPrice float64) error
}

func (m *mockOrderRepository) UpdateStatus(ctx context.Context, tx *sql.Tx, id uint, status string) error {
	return m.UpdateStatusFunc(ctx, tx, id, status)
}

func (m *mockOrderRepository) UpdateTotalPrice(ctx context.Context, tx *sql.Tx, id uint, totalPrice float64) error {
	return m.UpdateTotalPriceFunc(ctx, tx, id, totalPrice)
}

// Tests - These test the validation logic by returning early (product lookup fails before transaction operations)

func TestReserveItems_ProductNotFound(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	productRepo := &mockProductRepository{
		FindByIDForUpdateFunc: func(ctx context.Context, tx *sql.Tx, productID int, companyID int) (*domain.Product, error) {
			return nil, dtoerrors.NewNotFoundError("product not found")
		},
	}

	// These should not be called
	orderItemRepo := &mockOrderItemRepository{
		InsertFunc: func(ctx context.Context, tx *sql.Tx, item domain.OrderItem) (uint, error) {
			t.Fatal("Insert should not be called when product not found")
			return 0, nil
		},
	}

	orderRepo := &mockOrderRepository{
		UpdateStatusFunc: func(ctx context.Context, tx *sql.Tx, id uint, status string) error {
			t.Fatal("UpdateStatus should not be called")
			return nil
		},
		UpdateTotalPriceFunc: func(ctx context.Context, tx *sql.Tx, id uint, totalPrice float64) error {
			t.Fatal("UpdateTotalPrice should not be called")
			return nil
		},
	}

	txMgr := &mockTransactionManager{
		BeginTxFunc: func(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
			return (*sql.Tx)(nil), nil
		},
	}

	svc := NewReservationService(txMgr, productRepo, orderItemRepo, orderRepo, logger)
	items := []dto.ReservationItem{{ProductID: 1, Quantity: 10, Price: 100.0}}

	result, err := svc.ReserveItems(ctx, 1, 1, items)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result.Status != dto.ReservationAllFailed {
		t.Errorf("expected ALL_FAILED, got %s", result.Status)
	}

	if len(result.Failures) != 1 || result.Failures[0].Reason != dto.ReasonNotFound {
		t.Errorf("expected ReasonNotFound, got %v", result.Failures)
	}
}

func TestReserveItems_ProductInactive(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	productRepo := &mockProductRepository{
		FindByIDForUpdateFunc: func(ctx context.Context, tx *sql.Tx, productID int, companyID int) (*domain.Product, error) {
			return &domain.Product{
				ID:         productID,
				IsActive:   false,
				HasStock:   true,
				Stockeable: true,
			}, nil
		},
	}

	orderItemRepo := &mockOrderItemRepository{
		InsertFunc: func(ctx context.Context, tx *sql.Tx, item domain.OrderItem) (uint, error) {
			t.Fatal("Insert should not be called when product inactive")
			return 0, nil
		},
	}

	orderRepo := &mockOrderRepository{
		UpdateStatusFunc: func(ctx context.Context, tx *sql.Tx, id uint, status string) error {
			t.Fatal("UpdateStatus should not be called")
			return nil
		},
		UpdateTotalPriceFunc: func(ctx context.Context, tx *sql.Tx, id uint, totalPrice float64) error {
			t.Fatal("UpdateTotalPrice should not be called")
			return nil
		},
	}

	txMgr := &mockTransactionManager{
		BeginTxFunc: func(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
			return (*sql.Tx)(nil), nil
		},
	}

	svc := NewReservationService(txMgr, productRepo, orderItemRepo, orderRepo, logger)
	items := []dto.ReservationItem{{ProductID: 1, Quantity: 10, Price: 100.0}}

	result, err := svc.ReserveItems(ctx, 1, 1, items)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result.Status != dto.ReservationAllFailed {
		t.Errorf("expected ALL_FAILED, got %s", result.Status)
	}

	if len(result.Failures) != 1 || result.Failures[0].Reason != dto.ReasonProductInactive {
		t.Errorf("expected ReasonProductInactive, got %v", result.Failures)
	}
}

func TestReserveItems_ProductNotStockeable(t *testing.T) {
	// NEW FIX: validation is now unconditional - Stockeable=false always fails
	ctx := context.Background()
	logger := zap.NewNop()

	productRepo := &mockProductRepository{
		FindByIDForUpdateFunc: func(ctx context.Context, tx *sql.Tx, productID int, companyID int) (*domain.Product, error) {
			return &domain.Product{
				ID:            productID,
				IsActive:      true,
				HasStock:      true,
				Stockeable:    false, // Not stockeable
				ReservedStock: intPtr(50),
				Stock:         intPtr(100),
			}, nil
		},
		IncrementReservedStockFunc: func(ctx context.Context, tx *sql.Tx, productID int, quantity int) error {
			t.Fatal("IncrementReservedStock should not be called when Stockeable=false")
			return nil
		},
	}

	orderItemRepo := &mockOrderItemRepository{
		InsertFunc: func(ctx context.Context, tx *sql.Tx, item domain.OrderItem) (uint, error) {
			t.Fatal("Insert should not be called when Stockeable=false")
			return 0, nil
		},
	}

	orderRepo := &mockOrderRepository{
		UpdateStatusFunc: func(ctx context.Context, tx *sql.Tx, id uint, status string) error {
			t.Fatal("UpdateStatus should not be called")
			return nil
		},
		UpdateTotalPriceFunc: func(ctx context.Context, tx *sql.Tx, id uint, totalPrice float64) error {
			t.Fatal("UpdateTotalPrice should not be called")
			return nil
		},
	}

	txMgr := &mockTransactionManager{
		BeginTxFunc: func(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
			return (*sql.Tx)(nil), nil
		},
	}

	svc := NewReservationService(txMgr, productRepo, orderItemRepo, orderRepo, logger)
	items := []dto.ReservationItem{{ProductID: 1, Quantity: 10, Price: 100.0}}

	result, err := svc.ReserveItems(ctx, 1, 1, items)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result.Status != dto.ReservationAllFailed {
		t.Errorf("expected ALL_FAILED, got %s", result.Status)
	}

	if len(result.Failures) != 1 || result.Failures[0].Reason != dto.ReasonProductNotStockeable {
		t.Errorf("expected ReasonProductNotStockeable, got %v", result.Failures)
	}
}

func TestReserveItems_ProductHasStockFalse(t *testing.T) {
	// NEW FIX: validation is now unconditional - HasStock=false always fails
	ctx := context.Background()
	logger := zap.NewNop()

	productRepo := &mockProductRepository{
		FindByIDForUpdateFunc: func(ctx context.Context, tx *sql.Tx, productID int, companyID int) (*domain.Product, error) {
			return &domain.Product{
				ID:            productID,
				IsActive:      true,
				HasStock:      false, // Stock control disabled
				Stockeable:    true,
				ReservedStock: intPtr(50),
				Stock:         intPtr(100),
			}, nil
		},
		IncrementReservedStockFunc: func(ctx context.Context, tx *sql.Tx, productID int, quantity int) error {
			t.Fatal("IncrementReservedStock should not be called when HasStock=false")
			return nil
		},
	}

	orderItemRepo := &mockOrderItemRepository{
		InsertFunc: func(ctx context.Context, tx *sql.Tx, item domain.OrderItem) (uint, error) {
			t.Fatal("Insert should not be called when HasStock=false")
			return 0, nil
		},
	}

	orderRepo := &mockOrderRepository{
		UpdateStatusFunc: func(ctx context.Context, tx *sql.Tx, id uint, status string) error {
			t.Fatal("UpdateStatus should not be called")
			return nil
		},
		UpdateTotalPriceFunc: func(ctx context.Context, tx *sql.Tx, id uint, totalPrice float64) error {
			t.Fatal("UpdateTotalPrice should not be called")
			return nil
		},
	}

	txMgr := &mockTransactionManager{
		BeginTxFunc: func(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
			return (*sql.Tx)(nil), nil
		},
	}

	svc := NewReservationService(txMgr, productRepo, orderItemRepo, orderRepo, logger)
	items := []dto.ReservationItem{{ProductID: 1, Quantity: 10, Price: 100.0}}

	result, err := svc.ReserveItems(ctx, 1, 1, items)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result.Status != dto.ReservationAllFailed {
		t.Errorf("expected ALL_FAILED, got %s", result.Status)
	}

	if len(result.Failures) != 1 || result.Failures[0].Reason != dto.ReasonProductNotStockeable {
		t.Errorf("expected ReasonProductNotStockeable, got %v", result.Failures)
	}
}

func TestReserveItems_FullyReserved(t *testing.T) {
	// CRITICAL BUG FIX: Reproduces the exact bug scenario - stock=2, reserved=2, available=0
	// With the fix, validation is unconditional: should return OUT_OF_STOCK, Insert NOT called
	ctx := context.Background()
	logger := zap.NewNop()

	productRepo := &mockProductRepository{
		FindByIDForUpdateFunc: func(ctx context.Context, tx *sql.Tx, productID int, companyID int) (*domain.Product, error) {
			return &domain.Product{
				ID:            productID,
				IsActive:      true,
				HasStock:      true,
				Stockeable:    true,
				ReservedStock: intPtr(2), // fully reserved
				Stock:         intPtr(2),
			}, nil
		},
		IncrementReservedStockFunc: func(ctx context.Context, tx *sql.Tx, productID int, quantity int) error {
			t.Fatal("IncrementReservedStock should not be called when available=0")
			return nil
		},
	}

	orderItemRepo := &mockOrderItemRepository{
		InsertFunc: func(ctx context.Context, tx *sql.Tx, item domain.OrderItem) (uint, error) {
			t.Fatal("Insert should NOT be called - stock validation should have failed")
			return 0, nil
		},
	}

	orderRepo := &mockOrderRepository{
		UpdateStatusFunc: func(ctx context.Context, tx *sql.Tx, id uint, status string) error {
			t.Fatal("UpdateStatus should not be called")
			return nil
		},
		UpdateTotalPriceFunc: func(ctx context.Context, tx *sql.Tx, id uint, totalPrice float64) error {
			t.Fatal("UpdateTotalPrice should not be called")
			return nil
		},
	}

	txMgr := &mockTransactionManager{
		BeginTxFunc: func(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
			return (*sql.Tx)(nil), nil
		},
	}

	svc := NewReservationService(txMgr, productRepo, orderItemRepo, orderRepo, logger)
	// Using productID 176, companyID 2 from the bug report
	items := []dto.ReservationItem{{ProductID: 176, Quantity: 1, Price: 100.0}}

	result, err := svc.ReserveItems(ctx, 1, 2, items)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result.Status != dto.ReservationAllFailed {
		t.Errorf("expected ALL_FAILED, got %s", result.Status)
	}

	if len(result.Failures) != 1 || result.Failures[0].Reason != dto.ReasonOutOfStock {
		t.Errorf("expected ReasonOutOfStock, got %v", result.Failures)
	}
}

func TestReserveItems_OutOfStock(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	productRepo := &mockProductRepository{
		FindByIDForUpdateFunc: func(ctx context.Context, tx *sql.Tx, productID int, companyID int) (*domain.Product, error) {
			return &domain.Product{
				ID:            productID,
				IsActive:      true,
				HasStock:      true,
				Stockeable:    true,
				ReservedStock: intPtr(100),
				Stock:         intPtr(0),
			}, nil
		},
		IncrementReservedStockFunc: func(ctx context.Context, tx *sql.Tx, productID int, quantity int) error {
			t.Fatal("IncrementReservedStock should not be called when available=0")
			return nil
		},
	}

	orderItemRepo := &mockOrderItemRepository{
		InsertFunc: func(ctx context.Context, tx *sql.Tx, item domain.OrderItem) (uint, error) {
			t.Fatal("Insert should not be called when out of stock")
			return 0, nil
		},
	}

	orderRepo := &mockOrderRepository{
		UpdateStatusFunc: func(ctx context.Context, tx *sql.Tx, id uint, status string) error {
			t.Fatal("UpdateStatus should not be called")
			return nil
		},
		UpdateTotalPriceFunc: func(ctx context.Context, tx *sql.Tx, id uint, totalPrice float64) error {
			t.Fatal("UpdateTotalPrice should not be called")
			return nil
		},
	}

	txMgr := &mockTransactionManager{
		BeginTxFunc: func(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
			return (*sql.Tx)(nil), nil
		},
	}

	svc := NewReservationService(txMgr, productRepo, orderItemRepo, orderRepo, logger)
	items := []dto.ReservationItem{{ProductID: 1, Quantity: 10, Price: 100.0}}

	result, err := svc.ReserveItems(ctx, 1, 1, items)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result.Status != dto.ReservationAllFailed {
		t.Errorf("expected ALL_FAILED, got %s", result.Status)
	}

	if len(result.Failures) != 1 || result.Failures[0].Reason != dto.ReasonOutOfStock {
		t.Errorf("expected ReasonOutOfStock, got %v", result.Failures)
	}
}

func TestReserveItems_InsufficientAvailable(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	productRepo := &mockProductRepository{
		FindByIDForUpdateFunc: func(ctx context.Context, tx *sql.Tx, productID int, companyID int) (*domain.Product, error) {
			return &domain.Product{
				ID:            productID,
				IsActive:      true,
				HasStock:      true,
				Stockeable:    true,
				ReservedStock: intPtr(80),
				Stock:         intPtr(100),
			}, nil
		},
		IncrementReservedStockFunc: func(ctx context.Context, tx *sql.Tx, productID int, quantity int) error {
			t.Fatal("IncrementReservedStock should not be called when insufficient available")
			return nil
		},
	}

	orderItemRepo := &mockOrderItemRepository{
		InsertFunc: func(ctx context.Context, tx *sql.Tx, item domain.OrderItem) (uint, error) {
			t.Fatal("Insert should not be called when insufficient available")
			return 0, nil
		},
	}

	orderRepo := &mockOrderRepository{
		UpdateStatusFunc: func(ctx context.Context, tx *sql.Tx, id uint, status string) error {
			t.Fatal("UpdateStatus should not be called")
			return nil
		},
		UpdateTotalPriceFunc: func(ctx context.Context, tx *sql.Tx, id uint, totalPrice float64) error {
			t.Fatal("UpdateTotalPrice should not be called")
			return nil
		},
	}

	txMgr := &mockTransactionManager{
		BeginTxFunc: func(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
			return (*sql.Tx)(nil), nil
		},
	}

	svc := NewReservationService(txMgr, productRepo, orderItemRepo, orderRepo, logger)
	// available = 20, need 30
	items := []dto.ReservationItem{{ProductID: 1, Quantity: 30, Price: 100.0}}

	result, err := svc.ReserveItems(ctx, 1, 1, items)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result.Status != dto.ReservationAllFailed {
		t.Errorf("expected ALL_FAILED, got %s", result.Status)
	}

	if len(result.Failures) != 1 || result.Failures[0].Reason != dto.ReasonInsufficientAvailable {
		t.Errorf("expected ReasonInsufficientAvailable, got %v", result.Failures)
	}
}
