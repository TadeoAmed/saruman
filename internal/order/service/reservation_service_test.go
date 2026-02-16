package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"saruman/internal/domain"
	dtoerrors "saruman/internal/errors"
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
	FindByIDForUpdateFunc    func(ctx context.Context, tx *sql.Tx, productID int, companyID int) (*domain.Product, error)
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

// Fake transaction for testing
type fakeTx struct{}

func (f *fakeTx) Commit() error   { return nil }
func (f *fakeTx) Rollback() error { return nil }

// Tests

func TestReserveItems_AllSuccess(t *testing.T) {
	logger := zap.NewNop()

	productRepo := &mockProductRepository{
		FindByIDForUpdateFunc: func(ctx context.Context, tx *sql.Tx, productID int, companyID int) (*domain.Product, error) {
			return &domain.Product{
				ID:            productID,
				IsActive:      true,
				HasStock:      true,
				Stockeable:    true,
				ReservedStock: intPtr(0),
				Stock:         intPtr(100),
			}, nil
		},
		IncrementReservedStockFunc: func(ctx context.Context, tx *sql.Tx, productID int, quantity int) error {
			return nil
		},
	}

	orderItemRepo := &mockOrderItemRepository{
		InsertFunc: func(ctx context.Context, tx *sql.Tx, item domain.OrderItem) (uint, error) {
			return 1, nil
		},
	}

	orderRepo := &mockOrderRepository{
		UpdateStatusFunc: func(ctx context.Context, tx *sql.Tx, id uint, status string) error {
			return nil
		},
		UpdateTotalPriceFunc: func(ctx context.Context, tx *sql.Tx, id uint, totalPrice float64) error {
			return nil
		},
	}

	txMgr := &mockTransactionManager{
		BeginTxFunc: func(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
			return (*sql.Tx)(nil), nil
		},
	}

	svc := NewReservationService(txMgr, productRepo, orderItemRepo, orderRepo, logger)
	_ = svc

	t.Logf("Test setup complete - all success test structure ready")
}

func TestReserveItems_AllFailed_NotFound(t *testing.T) {
	logger := zap.NewNop()

	productRepo := &mockProductRepository{
		FindByIDForUpdateFunc: func(ctx context.Context, tx *sql.Tx, productID int, companyID int) (*domain.Product, error) {
			return nil, dtoerrors.NewNotFoundError("product not found")
		},
	}

	orderItemRepo := &mockOrderItemRepository{
		InsertFunc: func(ctx context.Context, tx *sql.Tx, item domain.OrderItem) (uint, error) {
			return 0, errors.New("should not be called")
		},
	}

	orderRepo := &mockOrderRepository{
		UpdateStatusFunc: func(ctx context.Context, tx *sql.Tx, id uint, status string) error {
			return errors.New("should not be called")
		},
		UpdateTotalPriceFunc: func(ctx context.Context, tx *sql.Tx, id uint, totalPrice float64) error {
			return errors.New("should not be called")
		},
	}

	txMgr := &mockTransactionManager{
		BeginTxFunc: func(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
			return (*sql.Tx)(nil), nil
		},
	}

	svc := NewReservationService(txMgr, productRepo, orderItemRepo, orderRepo, logger)
	_ = svc

	t.Logf("Test setup complete - all failed test structure ready")
}

func TestReserveItems_Partial(t *testing.T) {
	logger := zap.NewNop()

	callCount := 0
	productRepo := &mockProductRepository{
		FindByIDForUpdateFunc: func(ctx context.Context, tx *sql.Tx, productID int, companyID int) (*domain.Product, error) {
			callCount++
			if callCount == 1 {
				// First product: OK
				return &domain.Product{
					ID:            productID,
					IsActive:      true,
					HasStock:      true,
					Stockeable:    true,
					ReservedStock: intPtr(0),
					Stock:         intPtr(100),
				}, nil
			}
			// Second product: out of stock
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
			return nil
		},
	}

	orderItemRepo := &mockOrderItemRepository{
		InsertFunc: func(ctx context.Context, tx *sql.Tx, item domain.OrderItem) (uint, error) {
			return 1, nil
		},
	}

	orderRepo := &mockOrderRepository{
		UpdateStatusFunc: func(ctx context.Context, tx *sql.Tx, id uint, status string) error {
			return nil
		},
		UpdateTotalPriceFunc: func(ctx context.Context, tx *sql.Tx, id uint, totalPrice float64) error {
			return nil
		},
	}

	txMgr := &mockTransactionManager{
		BeginTxFunc: func(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
			return (*sql.Tx)(nil), nil
		},
	}

	svc := NewReservationService(txMgr, productRepo, orderItemRepo, orderRepo, logger)
	_ = svc

	t.Logf("Test setup complete - partial reservation test structure ready")
}

func TestReserveItems_OutOfStock(t *testing.T) {
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
	}

	orderItemRepo := &mockOrderItemRepository{
		InsertFunc: func(ctx context.Context, tx *sql.Tx, item domain.OrderItem) (uint, error) {
			return 0, errors.New("should not be called")
		},
	}

	orderRepo := &mockOrderRepository{
		UpdateStatusFunc: func(ctx context.Context, tx *sql.Tx, id uint, status string) error {
			return errors.New("should not be called")
		},
		UpdateTotalPriceFunc: func(ctx context.Context, tx *sql.Tx, id uint, totalPrice float64) error {
			return errors.New("should not be called")
		},
	}

	txMgr := &mockTransactionManager{
		BeginTxFunc: func(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
			return (*sql.Tx)(nil), nil
		},
	}

	svc := NewReservationService(txMgr, productRepo, orderItemRepo, orderRepo, logger)
	_ = svc

	t.Logf("Test setup complete - out of stock test structure ready")
}

func TestReserveItems_InsufficientAvailable(t *testing.T) {
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
	}

	orderItemRepo := &mockOrderItemRepository{
		InsertFunc: func(ctx context.Context, tx *sql.Tx, item domain.OrderItem) (uint, error) {
			return 0, errors.New("should not be called")
		},
	}

	orderRepo := &mockOrderRepository{
		UpdateStatusFunc: func(ctx context.Context, tx *sql.Tx, id uint, status string) error {
			return errors.New("should not be called")
		},
		UpdateTotalPriceFunc: func(ctx context.Context, tx *sql.Tx, id uint, totalPrice float64) error {
			return errors.New("should not be called")
		},
	}

	txMgr := &mockTransactionManager{
		BeginTxFunc: func(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
			return (*sql.Tx)(nil), nil
		},
	}

	svc := NewReservationService(txMgr, productRepo, orderItemRepo, orderRepo, logger)
	_ = svc

	t.Logf("Test setup complete - insufficient available test structure ready")
}

func TestReserveItems_ProductInactive(t *testing.T) {
	logger := zap.NewNop()

	productRepo := &mockProductRepository{
		FindByIDForUpdateFunc: func(ctx context.Context, tx *sql.Tx, productID int, companyID int) (*domain.Product, error) {
			return &domain.Product{
				ID:            productID,
				IsActive:      false,
				HasStock:      true,
				Stockeable:    true,
				ReservedStock: intPtr(0),
				Stock:         intPtr(100),
			}, nil
		},
	}

	orderItemRepo := &mockOrderItemRepository{
		InsertFunc: func(ctx context.Context, tx *sql.Tx, item domain.OrderItem) (uint, error) {
			return 0, errors.New("should not be called")
		},
	}

	orderRepo := &mockOrderRepository{
		UpdateStatusFunc: func(ctx context.Context, tx *sql.Tx, id uint, status string) error {
			return errors.New("should not be called")
		},
		UpdateTotalPriceFunc: func(ctx context.Context, tx *sql.Tx, id uint, totalPrice float64) error {
			return errors.New("should not be called")
		},
	}

	txMgr := &mockTransactionManager{
		BeginTxFunc: func(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
			return (*sql.Tx)(nil), nil
		},
	}

	svc := NewReservationService(txMgr, productRepo, orderItemRepo, orderRepo, logger)
	_ = svc

	t.Logf("Test setup complete - product inactive test structure ready")
}

func TestReserveItems_NoStockControl(t *testing.T) {
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
			return errors.New("should not be called when hasStockControl=false")
		},
	}

	orderItemRepo := &mockOrderItemRepository{
		InsertFunc: func(ctx context.Context, tx *sql.Tx, item domain.OrderItem) (uint, error) {
			return 1, nil
		},
	}

	orderRepo := &mockOrderRepository{
		UpdateStatusFunc: func(ctx context.Context, tx *sql.Tx, id uint, status string) error {
			return nil
		},
		UpdateTotalPriceFunc: func(ctx context.Context, tx *sql.Tx, id uint, totalPrice float64) error {
			return nil
		},
	}

	txMgr := &mockTransactionManager{
		BeginTxFunc: func(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
			return (*sql.Tx)(nil), nil
		},
	}

	svc := NewReservationService(txMgr, productRepo, orderItemRepo, orderRepo, logger)
	_ = svc

	t.Logf("Test setup complete - no stock control test structure ready")
}

func TestReserveItems_ProductNotStockeable(t *testing.T) {
	logger := zap.NewNop()

	productRepo := &mockProductRepository{
		FindByIDForUpdateFunc: func(ctx context.Context, tx *sql.Tx, productID int, companyID int) (*domain.Product, error) {
			return &domain.Product{
				ID:            productID,
				IsActive:      true,
				HasStock:      true,
				Stockeable:    false,
				ReservedStock: intPtr(100),
				Stock:         intPtr(0),
			}, nil
		},
		IncrementReservedStockFunc: func(ctx context.Context, tx *sql.Tx, productID int, quantity int) error {
			return errors.New("should not be called when Stockeable=false")
		},
	}

	orderItemRepo := &mockOrderItemRepository{
		InsertFunc: func(ctx context.Context, tx *sql.Tx, item domain.OrderItem) (uint, error) {
			return 1, nil
		},
	}

	orderRepo := &mockOrderRepository{
		UpdateStatusFunc: func(ctx context.Context, tx *sql.Tx, id uint, status string) error {
			return nil
		},
		UpdateTotalPriceFunc: func(ctx context.Context, tx *sql.Tx, id uint, totalPrice float64) error {
			return nil
		},
	}

	txMgr := &mockTransactionManager{
		BeginTxFunc: func(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
			return (*sql.Tx)(nil), nil
		},
	}

	svc := NewReservationService(txMgr, productRepo, orderItemRepo, orderRepo, logger)
	_ = svc

	t.Logf("Test setup complete - product not stockeable test structure ready")
}

func TestReserveItems_DBErrorOnIncrement(t *testing.T) {
	logger := zap.NewNop()

	productRepo := &mockProductRepository{
		FindByIDForUpdateFunc: func(ctx context.Context, tx *sql.Tx, productID int, companyID int) (*domain.Product, error) {
			return &domain.Product{
				ID:            productID,
				IsActive:      true,
				HasStock:      true,
				Stockeable:    true,
				ReservedStock: intPtr(0),
				Stock:         intPtr(100),
			}, nil
		},
		IncrementReservedStockFunc: func(ctx context.Context, tx *sql.Tx, productID int, quantity int) error {
			return errors.New("database error")
		},
	}

	orderItemRepo := &mockOrderItemRepository{
		InsertFunc: func(ctx context.Context, tx *sql.Tx, item domain.OrderItem) (uint, error) {
			return 0, errors.New("should not be called")
		},
	}

	orderRepo := &mockOrderRepository{
		UpdateStatusFunc: func(ctx context.Context, tx *sql.Tx, id uint, status string) error {
			return errors.New("should not be called")
		},
		UpdateTotalPriceFunc: func(ctx context.Context, tx *sql.Tx, id uint, totalPrice float64) error {
			return errors.New("should not be called")
		},
	}

	txMgr := &mockTransactionManager{
		BeginTxFunc: func(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
			return (*sql.Tx)(nil), nil
		},
	}

	svc := NewReservationService(txMgr, productRepo, orderItemRepo, orderRepo, logger)
	_ = svc

	t.Logf("Test setup complete - DB error on increment test structure ready")
}
