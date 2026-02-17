package order

import (
	"database/sql"

	companyrepo "saruman/internal/company/repository"
	ordercontroller "saruman/internal/order/controller"
	orderrepo "saruman/internal/order/repository"
	"saruman/internal/order/service"
	"saruman/internal/order/usecase"
	productrepo "saruman/internal/product/repository"
	"go.uber.org/zap"
)

func NewModule(db *sql.DB, logger *zap.Logger) *ordercontroller.ReserveAndAddController {
	orderRepo := orderrepo.NewMySQLOrderRepository(db)
	orderItemRepo := orderrepo.NewMySQLOrderItemRepository(db)
	productRepo := productrepo.NewMySQLRepository(db)
	companyConfigRepo := companyrepo.NewMySQLCompanyConfigRepository(db)

	reservationSvc := service.NewReservationService(db, productRepo, orderItemRepo, orderRepo, logger)

	uc := usecase.NewReserveAndAddUseCase(orderRepo, companyConfigRepo, reservationSvc, logger)
	return ordercontroller.NewReserveAndAddController(uc, logger)
}
