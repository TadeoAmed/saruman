package order

import (
	"database/sql"

	companyrepo "saruman/internal/company/repository"
	"saruman/internal/config"
	orderrepo "saruman/internal/order/repository"
	"saruman/internal/order/service"
	"saruman/internal/order/usecase"
	productrepo "saruman/internal/product/repository"
	"go.uber.org/zap"
)

func NewModule(db *sql.DB, cfg *config.Config, logger *zap.Logger) *usecase.ReserveAndAddUseCase {
	orderRepo := orderrepo.NewMySQLOrderRepository(db)
	orderItemRepo := orderrepo.NewMySQLOrderItemRepository(db)
	productRepo := productrepo.NewMySQLRepository(db)
	companyConfigRepo := companyrepo.NewMySQLCompanyConfigRepository(db)

	reservationSvc := service.NewReservationService(
		db,
		productRepo,
		orderItemRepo,
		orderRepo,
		logger,
		cfg.Order.ReservationTxTimeout,
		cfg.Order.MaxRetryAttempts,
	)

	return usecase.NewReserveAndAddUseCase(
		orderRepo,
		companyConfigRepo,
		reservationSvc,
		logger,
		cfg.Order.MaxRetryAttempts,
	)
}
