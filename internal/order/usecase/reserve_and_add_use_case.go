package usecase

import (
	"context"
	"errors"
	"math/rand"
	"sort"
	"time"

	"github.com/go-sql-driver/mysql"
	"saruman/internal/domain"
	dtoerrors "saruman/internal/errors"
	"saruman/internal/dto"
	"go.uber.org/zap"
)

type StockReservationService interface {
	ReserveItems(
		ctx context.Context,
		orderID uint,
		companyID int,
		items []dto.ReservationItem,
		hasStockControl bool,
	) (*dto.ReservationResult, error)
}

type OrderRepository interface {
	FindByID(ctx context.Context, id uint) (*domain.Order, error)
}

type CompanyConfigRepository interface {
	FindByCompanyID(ctx context.Context, companyID int) (*domain.CompanyConfig, error)
}

type ReserveAndAddUseCase struct {
	orderRepo         OrderRepository
	companyConfigRepo CompanyConfigRepository
	reservationSvc    StockReservationService
	logger            *zap.Logger
	maxRetryAttempts  int
}

func NewReserveAndAddUseCase(
	orderRepo OrderRepository,
	companyConfigRepo CompanyConfigRepository,
	reservationSvc StockReservationService,
	logger *zap.Logger,
	maxRetryAttempts int,
) *ReserveAndAddUseCase {
	return &ReserveAndAddUseCase{
		orderRepo:        orderRepo,
		companyConfigRepo: companyConfigRepo,
		reservationSvc:   reservationSvc,
		logger:           logger,
		maxRetryAttempts: maxRetryAttempts,
	}
}

func (uc *ReserveAndAddUseCase) ReserveItems(
	ctx context.Context,
	orderID uint,
	companyID int,
	items []dto.ReservationItem,
) (*dto.ReservationResult, error) {
	// Bloque 1: Logging de inicio
	uc.logger.Info("reserve-and-add started", zap.Uint("orderId", orderID), zap.Int("companyId", companyID), zap.Int("itemCount", len(items)))

	// Bloque 2: Pre-validaciones (fuera de transacción)
	order, err := uc.orderRepo.FindByID(ctx, orderID)
	if err != nil {
		if _, ok := dtoerrors.IsNotFoundError(err); ok {
			return nil, dtoerrors.NewNotFoundError("order not found")
		}
		return nil, err
	}

	if order.Status != domain.OrderStatusPending {
		return nil, dtoerrors.NewConflictError("order is not in PENDING status")
	}

	if order.CompanyID != companyID {
		return nil, dtoerrors.NewForbiddenError("company mismatch")
	}

	companyConfig, err := uc.companyConfigRepo.FindByCompanyID(ctx, companyID)
	if err != nil {
		if _, ok := dtoerrors.IsNotFoundError(err); ok {
			return nil, dtoerrors.NewNotFoundError("company config not found")
		}
		return nil, err
	}

	hasStockControl := companyConfig.HasStock
	uc.logger.Debug("pre-validation passed", zap.Uint("orderId", orderID), zap.String("orderStatus", order.Status), zap.Bool("hasStockControl", hasStockControl))

	// Bloque 3: Ordenar items por productId ASC (anti-deadlock)
	sort.Slice(items, func(i, j int) bool { return items[i].ProductID < items[j].ProductID })

	// Bloque 4: Llamar service con retry
	return uc.reserveItemsWithRetry(ctx, orderID, companyID, items, hasStockControl)
}

func (uc *ReserveAndAddUseCase) reserveItemsWithRetry(
	ctx context.Context,
	orderID uint,
	companyID int,
	items []dto.ReservationItem,
	hasStockControl bool,
) (*dto.ReservationResult, error) {
	maxAttempts := uc.maxRetryAttempts
	// Backoff intervals: attempt 1 (0ms), attempt 2 (100ms), attempt 3 (200ms), etc.
	backoffs := []time.Duration{0, 100 * time.Millisecond, 200 * time.Millisecond}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result, err := uc.reservationSvc.ReserveItems(ctx, orderID, companyID, items, hasStockControl)
		if err == nil {
			return result, nil
		}

		if isDeadlockError(err) {
			if attempt < maxAttempts {
				// Calculate jitter: ±20% of backoff base
				jitter := backoffs[attempt-1] * time.Duration(0.8+rand.Float64()*0.4)
				time.Sleep(backoffs[attempt-1] + jitter)
				uc.logger.Warn("deadlock detected, retrying", zap.Int("attempt", attempt), zap.Int("maxAttempts", maxAttempts), zap.Uint("orderId", orderID))
				continue
			}
			// Last attempt with deadlock, fall through to return DeadlockError after loop
			break
		}

		// Non-deadlock error, return immediately
		return nil, err
	}

	return nil, dtoerrors.NewDeadlockError("max retries exceeded")
}

func isDeadlockError(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1213 || mysqlErr.Number == 1205
	}
	return false
}
