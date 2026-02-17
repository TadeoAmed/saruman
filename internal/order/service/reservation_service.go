package service

import (
	"context"
	"database/sql"
	"time"

	"saruman/internal/domain"
	"saruman/internal/dto"
	"go.uber.org/zap"
)

type TransactionManager interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

type ProductRepository interface {
	FindByIDForUpdate(ctx context.Context, tx *sql.Tx, productID int, companyID int) (*domain.Product, error)
	IncrementReservedStock(ctx context.Context, tx *sql.Tx, productID int, quantity int) error
}

type OrderItemRepository interface {
	Insert(ctx context.Context, tx *sql.Tx, item domain.OrderItem) (uint, error)
}

type OrderRepository interface {
	UpdateStatus(ctx context.Context, tx *sql.Tx, id uint, status string) error
	UpdateTotalPrice(ctx context.Context, tx *sql.Tx, id uint, totalPrice float64) error
}

type ReservationService struct {
	db              TransactionManager
	productRepo     ProductRepository
	orderItemRepo   OrderItemRepository
	orderRepo       OrderRepository
	logger          *zap.Logger
	txTimeout       time.Duration
	maxRetryAttempts int
}

func NewReservationService(
	db TransactionManager,
	productRepo ProductRepository,
	orderItemRepo OrderItemRepository,
	orderRepo OrderRepository,
	logger *zap.Logger,
	txTimeout time.Duration,
	maxRetryAttempts int,
) *ReservationService {
	return &ReservationService{
		db:               db,
		productRepo:      productRepo,
		orderItemRepo:    orderItemRepo,
		orderRepo:        orderRepo,
		logger:           logger,
		txTimeout:        txTimeout,
		maxRetryAttempts: maxRetryAttempts,
	}
}

func (s *ReservationService) ReserveItems(
	ctx context.Context,
	orderID uint,
	companyID int,
	items []dto.ReservationItem,
) (*dto.ReservationResult, error) {
	// Bloque 1: Iniciar transacción con timeout
	txCtx, cancel := context.WithTimeout(ctx, s.txTimeout)
	defer cancel()

	tx, err := s.db.BeginTx(txCtx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		s.logger.Error("failed to begin transaction", zap.Error(err))
		return nil, err
	}
	// Ensure rollback on any exit path. MySQL ignores rollback if already committed.
	// Only defer if tx is not nil (for testing compatibility)
	if tx != nil {
		defer tx.Rollback()
	}

	// Bloque 2: Procesar items
	successes := []dto.ItemSuccess{}
	failures := []dto.ItemFailure{}
	totalPrice := 0.0

	for _, item := range items {
		success, failure, err := s.reserveSingleItem(txCtx, tx, orderID, companyID, item)
		if err != nil {
			s.logger.Error("reservation error", zap.Uint("orderId", orderID), zap.Int("productId", item.ProductID), zap.Error(err))
			return nil, err
		}

		if success != nil {
			successes = append(successes, *success)
			totalPrice += item.Price * float64(item.Quantity)
			s.logger.Info("item reserved successfully", zap.Uint("orderId", orderID), zap.Int("productId", item.ProductID), zap.Int("quantity", item.Quantity))
		}

		if failure != nil {
			failures = append(failures, *failure)
			s.logger.Warn("item reservation failed", zap.Uint("orderId", orderID), zap.Int("productId", item.ProductID), zap.Int("quantity", item.Quantity), zap.String("reason", string(failure.Reason)))
		}
	}

	// Bloque 3: Decidir resultado y commit
	if len(successes) == 0 {
		s.logger.Warn("transaction rolled back (all failed)", zap.Uint("orderId", orderID), zap.Int("failureCount", len(failures)))
		return &dto.ReservationResult{
			Status:   dto.ReservationAllFailed,
			OrderID:  orderID,
			Failures: failures,
		}, nil
	}

	// len(successes) > 0: commit and return result
	err = s.orderRepo.UpdateStatus(txCtx, tx, orderID, domain.OrderStatusCreated)
	if err != nil {
		s.logger.Error("failed to update order status", zap.Uint("orderId", orderID), zap.Error(err))
		return nil, err
	}

	err = s.orderRepo.UpdateTotalPrice(txCtx, tx, orderID, totalPrice)
	if err != nil {
		s.logger.Error("failed to update order total price", zap.Uint("orderId", orderID), zap.Error(err))
		return nil, err
	}

	err = tx.Commit()
	if err != nil {
		s.logger.Error("failed to commit transaction", zap.Uint("orderId", orderID), zap.Error(err))
		return nil, err
	}

	s.logger.Info("transaction committed", zap.Uint("orderId", orderID), zap.Int("successCount", len(successes)), zap.Float64("totalPrice", totalPrice))

	status := dto.ReservationAllSuccess
	if len(failures) > 0 {
		status = dto.ReservationPartial
	}

	return &dto.ReservationResult{
		Status:     status,
		OrderID:    orderID,
		TotalPrice: totalPrice,
		Successes:  successes,
		Failures:   failures,
	}, nil
}

func (s *ReservationService) reserveSingleItem(
	ctx context.Context,
	tx *sql.Tx,
	orderID uint,
	companyID int,
	item dto.ReservationItem,
) (*dto.ItemSuccess, *dto.ItemFailure, error) {
	// 1. Fetch product with lock
	product, err := s.productRepo.FindByIDForUpdate(ctx, tx, item.ProductID, companyID)
	if err != nil {
		return nil, &dto.ItemFailure{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
			Reason:    dto.ReasonNotFound,
		}, nil
	}

	// 2. Check if product is active
	if !product.IsActive {
		return nil, &dto.ItemFailure{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
			Reason:    dto.ReasonProductInactive,
		}, nil
	}

	// 3. Check product-level stockeability (ALWAYS runs, replaces old conditional)
	if !product.HasStock || !product.Stockeable {
		return nil, &dto.ItemFailure{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
			Reason:    dto.ReasonProductNotStockeable,
		}, nil
	}

	// 4. Check available stock (ALWAYS runs now)
	available := product.AvailableStock()
	if available == 0 {
		return nil, &dto.ItemFailure{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
			Reason:    dto.ReasonOutOfStock,
		}, nil
	}

	if available < item.Quantity {
		return nil, &dto.ItemFailure{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
			Reason:    dto.ReasonInsufficientAvailable,
		}, nil
	}

	// 5. Reserve stock
	err = s.productRepo.IncrementReservedStock(ctx, tx, item.ProductID, item.Quantity)
	if err != nil {
		return nil, nil, err
	}

	// 6. Create order item (solo se llega aquí si todos los checks pasan)
	orderItem := domain.OrderItem{
		OrderID:   orderID,
		ProductID: item.ProductID,
		Quantity:  item.Quantity,
		Price:     item.Price,
	}

	_, err = s.orderItemRepo.Insert(ctx, tx, orderItem)
	if err != nil {
		return nil, nil, err
	}

	// 7. Return success
	return &dto.ItemSuccess{
		ProductID: item.ProductID,
		Quantity:  item.Quantity,
	}, nil, nil
}
