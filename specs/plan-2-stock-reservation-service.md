# Plan 2: Stock Reservation Service

**Spec:** `spec-2-stock-reservation-service.md`
**Fecha:** 2026-02-15
**Estado:** Pendiente de aprobación
**Depende de:** plan-1 (domain entities + repositories deben estar implementados)

---

## Resumen

Implementar la capa de service (`reservation_service.go`) y la capa de use case (`reserve_and_add_use_case.go`) del módulo `order`. El service contiene la lógica de negocio pura (validar producto, reservar stock, crear order item por cada item). El use case orquesta el flujo completo: pre-validaciones, ordenamiento anti-deadlock, transacción con retry, y construcción del resultado parcial.

No se toca HTTP ni DTOs — ese es scope de spec-3.

---

## Paso 1: Definir tipos de resultado de reserva

**Archivo:** `internal/dto/reservation.go` (CREAR o EDITAR — agregar tipos de reserva)

**Qué hacer:**
- Definir tipo `ReservationStatus string` con constantes:
  ```go
  // ReservationStatus indicates the overall result of a reservation operation.
  // ALL_SUCCESS: all items reserved, order committed. PARTIAL: some items reserved, some failed, order committed.
  // ALL_FAILED: no items reserved, transaction rolled back.
  type ReservationStatus string
  const (
      ReservationAllSuccess ReservationStatus = "ALL_SUCCESS"
      ReservationPartial    ReservationStatus = "PARTIAL"
      ReservationAllFailed  ReservationStatus = "ALL_FAILED"
  )
  ```

- Definir tipo `FailureReason string` con constantes:
  ```go
  // FailureReason describes why a single item could not be reserved.
  type FailureReason string
  const (
      // NOT_FOUND: product not found for the given company ID.
      ReasonNotFound FailureReason = "NOT_FOUND"
      // OUT_OF_STOCK: product has zero available stock (reserved + sold == total).
      ReasonOutOfStock FailureReason = "OUT_OF_STOCK"
      // INSUFFICIENT_AVAILABLE: available stock < requested quantity.
      ReasonInsufficientAvailable FailureReason = "INSUFFICIENT_AVAILABLE"
      // PRODUCT_INACTIVE: product.isActive == false, cannot be reserved.
      ReasonProductInactive FailureReason = "PRODUCT_INACTIVE"
  )
  ```

- Definir struct `ItemSuccess`:
  ```go
  // ItemSuccess records a successfully reserved item (product ID and quantity).
  type ItemSuccess struct {
      ProductID int
      Quantity  int
  }
  ```

- Definir struct `ItemFailure`:
  ```go
  // ItemFailure records a failed reservation attempt with the reason (stock unavailable, product inactive, etc.).
  type ItemFailure struct {
      ProductID int
      Quantity  int
      Reason    FailureReason
  }
  ```

- Definir struct `ReservationResult`:
  ```go
  // ReservationResult is the final output of a reserve-and-add operation.
  // Contains the overall status, order ID, total price of reserved items, and per-item successes and failures.
  type ReservationResult struct {
      Status     ReservationStatus
      OrderID    uint
      TotalPrice float64
      Successes  []ItemSuccess
      Failures   []ItemFailure
  }
  ```

- Definir struct `ReservationItem` (input del service y use case):
  ```go
  // ReservationItem is the input data for each product to be reserved.
  // Includes product ID, quantity requested, and unit price (for total price calculation).
  type ReservationItem struct {
      ProductID int
      Quantity  int
      Price     float64
  }
  ```

**Nota:** Estos tipos viven en `internal/dto/` (centralizados). Service, UseCase y Controller los importan desde aquí. Son DTOs de dominio (resultado de operación de negocio), no entidades persistidas.

---

## Paso 2: Definir errores de negocio del use case

**Archivo:** `internal/errors/errors.go` (EDITAR)

**Qué hacer:**
- Agregar tipo `ConflictError` (HTTP 409):
  ```go
  // ConflictError indicates the order cannot be processed due to invalid state (e.g., not PENDING).
  // Maps to HTTP 409 Conflict in the controller.
  type ConflictError struct{ Message string }
  func (e *ConflictError) Error() string { return e.Message }
  func NewConflictError(message string) *ConflictError { return &ConflictError{Message: message} }
  func IsConflictError(err error) (*ConflictError, bool) { ... }
  ```

- Agregar tipo `ForbiddenError` (HTTP 403):
  ```go
  // ForbiddenError indicates the requesting company is not the owner of the order.
  // Maps to HTTP 403 Forbidden in the controller.
  type ForbiddenError struct{ Message string }
  func (e *ForbiddenError) Error() string { return e.Message }
  func NewForbiddenError(message string) *ForbiddenError { return &ForbiddenError{Message: message} }
  func IsForbiddenError(err error) (*ForbiddenError, bool) { ... }
  ```

- Agregar tipo `DeadlockError` (internal retry flag):
  ```go
  // DeadlockError wraps MySQL deadlock errors (codes 1213, 1205) for retry logic.
  // Returned when max retry attempts exceeded. Not directly mapped to HTTP response in this layer.
  type DeadlockError struct{ Message string }
  func (e *DeadlockError) Error() string { return e.Message }
  func NewDeadlockError(message string) *DeadlockError { return &DeadlockError{Message: message} }
  func IsDeadlockError(err error) (*DeadlockError, bool) { ... }
  ```

**Justificación:** El use case necesita retornar errores semánticos que el controller (spec-3) mapeará a 409, 403 y 503/500 respectivamente. Siguen el patrón exacto de `ValidationError` / `NotFoundError` (plan-1).

---

## Paso 3: Implementar `reservation_service.go`

**Archivo:** `internal/order/service/reservation_service.go` (NUEVO)

### 3.1 Interfaces que consume el service (definidas en este paquete)

```go
type TransactionManager interface {
    // BeginTx starts a new database transaction with specified isolation level.
    // Used by the service to control atomicity of stock reservation operations.
    BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

type ProductRepository interface {
    // FindByIDForUpdate retrieves a product by ID with an exclusive row lock (SELECT ... FOR UPDATE).
    // The lock prevents concurrent transactions from modifying the same product's reserved stock,
    // avoiding race conditions during concurrent reservations. Filters by companyId to ensure multi-tenancy.
    FindByIDForUpdate(ctx context.Context, tx *sql.Tx, productID int, companyID int) (*domain.Product, error)

    // IncrementReservedStock atomically increases the product's reserved stock by the given quantity.
    // Called after validating available stock. Must execute within the same transaction as FindByIDForUpdate.
    IncrementReservedStock(ctx context.Context, tx *sql.Tx, productID int, quantity int) error
}

type OrderItemRepository interface {
    // Insert creates a new order item (product + quantity + price) linked to an order.
    // Executes within the reservation transaction. Commits only if all items are processed successfully.
    Insert(ctx context.Context, tx *sql.Tx, item domain.OrderItem) (uint, error)
}

type OrderRepository interface {
    // UpdateStatus changes the order status from PENDING to CREATED.
    // Only called after at least one item is successfully reserved (atomicity guarantee).
    UpdateStatus(ctx context.Context, tx *sql.Tx, id uint, status string) error

    // UpdateTotalPrice sets the order's total price to the sum of reserved item prices.
    // Committed together with status update and order items in the same transaction.
    UpdateTotalPrice(ctx context.Context, tx *sql.Tx, id uint, totalPrice float64) error
}
```

**Nota:** `TransactionManager` es una interfaz que el service utiliza para iniciar transacciones. En implementación, será `*sql.DB` que satisface esta interfaz trivialmente.

**Regla CLAUDE.md:** Las interfaces se definen en el paquete que las CONSUME. El service las define aquí; los repositorios concretos de plan-1 las satisfacen implícitamente.

### 3.2 Struct y constructor

```go
type ReservationService struct {
    db            TransactionManager  // *sql.DB satisface esta interfaz
    productRepo   ProductRepository
    orderItemRepo OrderItemRepository
    orderRepo     OrderRepository
    logger        *zap.Logger
}

func NewReservationService(
    db TransactionManager,
    productRepo ProductRepository,
    orderItemRepo OrderItemRepository,
    orderRepo OrderRepository,
    logger *zap.Logger,
) *ReservationService
```

**Responsabilidad:** El service maneja el ciclo de vida de la transacción. Recibe `TransactionManager` para iniciar `BeginTx` e inicia la transacción dentro del método `ReserveItems`.

### 3.3 Método `ReserveItems`

Firma:
```go
// ReserveItems is the core business logic that reserves stock for multiple products in a single atomic transaction.
// Initiates a REPEATABLE READ transaction, processes each item's availability and reservation,
// updates order status and total price on partial/full success, and returns detailed results per item.
// Handles transaction commit/rollback: commits on >=1 success, rolls back on all failures or DB errors.
func (s *ReservationService) ReserveItems(
    ctx context.Context,
    orderID uint,
    companyID int,
    items []ReservationItem,
    hasStockControl bool,
) (*ReservationResult, error)
```

Lógica (el service maneja la transacción internamente):

**Bloque 1: Iniciar transacción con timeout**
- `txCtx, cancel = context.WithTimeout(ctx, 5*time.Second)`; `defer cancel()`
- `tx, err = s.db.BeginTx(txCtx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})`; si error, retornar
- `defer tx.Rollback()` — seguro llamar aunque ya se hizo commit

**Bloque 2: Procesar items**
- Inicializar slices `successes []ItemSuccess` y `failures []ItemFailure`, acumulador `totalPrice float64`
- Para cada item: llamar `s.reserveSingleItem(txCtx, tx, orderID, companyID, item, hasStockControl)`
  - Si success: append a `successes`, sumar `item.Price * float64(item.Quantity)` a `totalPrice`
  - Si failure: append a `failures`
  - Si error DB inesperado (de `reserveSingleItem`): `logger.Error("reservation error", ...)`, retornar error (el `defer` rollback)

**Bloque 3: Decidir resultado y commit**
- Si `len(successes) == 0`: `logger.Warn("transaction rolled back (all failed)", ...)`, retornar `&ReservationResult{Status: ReservationAllFailed, OrderID: orderID, Failures: failures}` sin error
- Si `len(successes) > 0`:
  - `s.orderRepo.UpdateStatus(txCtx, tx, orderID, domain.OrderStatusCreated)`; si error, retornar
  - `s.orderRepo.UpdateTotalPrice(txCtx, tx, orderID, totalPrice)`; si error, retornar
  - `tx.Commit()`; si error, retornar
  - `logger.Info("transaction committed", zap.Uint("orderId", orderID), zap.Int("successCount", len(successes)), ...)`
  - Retornar `&ReservationResult{Status: ALL_SUCCESS o PARTIAL, TotalPrice: totalPrice, ...}`

**Logging dentro de ReserveItems:**
- `INFO` por cada item con success: `orderId`, `productId`, `quantity`
- `WARN` por cada item con failure: `orderId`, `productId`, `quantity`, `reason`

### 3.4 Método privado `reserveSingleItem`

Firma:
```go
// reserveSingleItem processes a single product reservation within the active transaction.
// Validates product existence, status, and stock availability (conditional on company config).
// On success: increments reserved_stock and inserts order item. On failure: returns ItemFailure with reason.
// Propagates unexpected DB errors; business-logic failures (stock unavailable, product inactive) are ItemFailure.
func (s *ReservationService) reserveSingleItem(
    ctx context.Context,
    tx *sql.Tx,
    orderID uint,
    companyID int,
    item ReservationItem,
    hasStockControl bool,
) (success *ItemSuccess, failure *ItemFailure, err error)
```

Lógica (en orden):
1. `productRepo.FindByIDForUpdate(ctx, tx, item.ProductID, companyID)` — si error `NotFoundError`, retornar failure con `ReasonNotFound`
2. Si `!product.IsActive`, retornar failure con `ReasonProductInactive`
3. Si `hasStockControl && product.HasStock && product.Stockeable`:
   - `available = product.AvailableStock()` (método del domain, ya implementado en spec-1)
   - Si `available == 0`: retornar failure con `ReasonOutOfStock`
   - Si `available < item.Quantity`: retornar failure con `ReasonInsufficientAvailable`
   - `productRepo.IncrementReservedStock(ctx, tx, item.ProductID, item.Quantity)` — si error, propagarlo como `error` (error inesperado de DB)
4. `orderItemRepo.Insert(ctx, tx, domain.OrderItem{...})` — si error, propagarlo
5. Retornar success

**Nota sobre errores inesperados de DB:** Si `IncrementReservedStock` o `Insert` fallan por error de DB (no de negocio), `reserveSingleItem` los retorna como `error`. El service los propaga hacia el use case, que hace rollback de la transacción.

---

## Paso 4: Implementar `reserve_and_add_use_case.go`

**Archivo:** `internal/order/usecase/reserve_and_add_use_case.go` (NUEVO)

### 4.1 Interfaces que consume el use case (definidas en este paquete)

```go
type StockReservationService interface {
    // ReserveItems orchestrates the atomic reservation of multiple products within a single transaction.
    // Returns a ReservationResult with successes and failures per item. Handles transaction lifecycle internally.
    // The use case retries this method on deadlock (MySQL errors 1213, 1205) up to 3 times.
    ReserveItems(
        ctx context.Context,
        orderID uint,
        companyID int,
        items []service.ReservationItem,
        hasStockControl bool,
    ) (*service.ReservationResult, error)
}

type OrderRepository interface {
    // FindByID retrieves an order by ID for pre-validation (check status and company ownership).
    // Used outside of transaction to verify order exists and is in PENDING status before reservation.
    FindByID(ctx context.Context, id uint) (*domain.Order, error)
}

type CompanyConfigRepository interface {
    // FindByCompanyID retrieves company-wide settings, particularly the hasStock flag.
    // Determines whether the use case should enforce stock validation during reservation.
    FindByCompanyID(ctx context.Context, companyID int) (*domain.CompanyConfig, error)
}
```

**Nota:** El service NO recibe `*sql.Tx` como parámetro. El service es quien maneja internamente la transacción. El use case solo orquesta pre-validaciones y llamada al service con retry.

### 4.2 Struct y constructor

```go
type ReserveAndAddUseCase struct {
    orderRepo         OrderRepository
    companyConfigRepo CompanyConfigRepository
    reservationSvc    StockReservationService
    logger            *zap.Logger
}

func NewReserveAndAddUseCase(
    orderRepo OrderRepository,
    companyConfigRepo CompanyConfigRepository,
    reservationSvc StockReservationService,
    logger *zap.Logger,
) *ReserveAndAddUseCase
```

**Responsabilidad del use case:** Orquestación pura (pre-validaciones, mapeo de datos, llamada al service con retry). NO maneja transacciones ni infraestructura. El service es quien maneja la transacción internamente.

### 4.3 Método `Execute`

Firma:
```go
// Execute orchestrates the complete reserve-and-add workflow: validates order state and company,
// sorts items by productId (anti-deadlock), and calls the reservation service with automatic retry on deadlock.
// No transaction logic here; all transactional concerns are delegated to the service.
func (uc *ReserveAndAddUseCase) Execute(
    ctx context.Context,
    orderID uint,
    companyID int,
    items []service.ReservationItem,
) (*service.ReservationResult, error)
```

Lógica en orden:

**Bloque 1: Logging de inicio**
- `logger.Info("reserve-and-add started", zap.Uint("orderId", orderID), zap.Int("companyId", companyID), zap.Int("itemCount", len(items)))`

**Bloque 2: Pre-validaciones (fuera de transacción)**
- `orderRepo.FindByID(ctx, orderID)` — si `NotFoundError`, retornar `NewNotFoundError("order not found")`
- Si `order.Status != domain.OrderStatusPending`, retornar `NewConflictError("order is not in PENDING status")`
- Si `order.CompanyID != companyID`, retornar `NewForbiddenError("company mismatch")`
- `companyConfigRepo.FindByCompanyID(ctx, companyID)` — si `NotFoundError`, retornar `NewNotFoundError("company config not found")`
- `hasStockControl = companyConfig.HasStock`
- `logger.Debug("pre-validation passed", zap.Uint("orderId", orderID), zap.String("orderStatus", order.Status), zap.Bool("hasStockControl", hasStockControl))`

**Bloque 3: Ordenar items por productId ASC (anti-deadlock)**
- Usar `sort.Slice(items, func(i, j int) bool { return items[i].ProductID < items[j].ProductID })`

**Bloque 4: Llamar service con retry**
- Llamar `uc.executeWithRetry(ctx, orderID, companyID, items, hasStockControl)` que encapsula el retry lógico
- Retornar result o error
- El service maneja internamente la transacción (Begin, Commit, Rollback)

### 4.4 Método privado `executeWithRetry`

```go
// executeWithRetry calls the reservation service up to 3 times, retrying only on MySQL deadlock errors (1213, 1205).
// Implements exponential backoff [0ms, 100ms, 200ms] with ±20% jitter to distribute retry attempts.
// Non-deadlock errors fail immediately. Returns the final result or DeadlockError if all retries exhausted.
func (uc *ReserveAndAddUseCase) executeWithRetry(
    ctx context.Context,
    orderID uint,
    companyID int,
    items []service.ReservationItem,
    hasStockControl bool,
) (*service.ReservationResult, error)
```

Lógica:
- `maxAttempts = 3`
- `backoffs = []time.Duration{0, 100 * time.Millisecond, 200 * time.Millisecond}`
- Loop `for attempt := 1; attempt <= maxAttempts; attempt++`:
  1. Llamar `uc.reservationSvc.ReserveItems(ctx, orderID, companyID, items, hasStockControl)` — el service maneja internamente la transacción
  2. Si no hay error: retornar result
  3. Si es deadlock (`isDeadlockError(err)`) y `attempt < maxAttempts`:
     - Calcular jitter: `±20%` del backoff base con `rand.Float64()`
     - `time.Sleep(backoffs[attempt-1] + jitter)` con verificación de `ctx.Done()`
     - `logger.Warn("deadlock detected, retrying", zap.Int("attempt", attempt), zap.Int("maxAttempts", maxAttempts), zap.Uint("orderId", orderID))`
     - Continuar loop
  4. Si no es deadlock o se agotaron intentos: retornar error
- Después del loop: retornar `NewDeadlockError("max retries exceeded")`

**Detección de deadlock (helper):**
```go
func isDeadlockError(err error) bool {
    // Detectar MySQL error 1213 (ER_LOCK_DEADLOCK) y 1205 (ER_LOCK_WAIT_TIMEOUT)
    // usando github.com/go-sql-driver/mysql MySQLError
    var mysqlErr *mysql.MySQLError
    if errors.As(err, &mysqlErr) {
        return mysqlErr.Number == 1213 || mysqlErr.Number == 1205
    }
    return false
}
```

**Nota:** El use case NO inicia transacciones. Solo orquesta: pre-validaciones → ordenamiento → retry lógico. El service es quien maneja la transacción internamente.

---

## Paso 5: Actualizar `wire.go` del módulo order

**Archivo:** `internal/order/wire.go` (CREAR o EDITAR — no existía en plan-1)

**Qué hacer:**
- Función `NewModule(db *sql.DB, logger *zap.Logger) *usecase.ReserveAndAddUseCase` (el controller llegará en spec-3; por ahora el wire expone el use case directamente)
- Instanciar repos concretos:
  ```go
  orderRepo     := repository.NewMySQLOrderRepository(db)
  orderItemRepo := repository.NewMySQLOrderItemRepository(db)
  productRepo   := repository.NewMySQLProductRepository(db)    // ya existente, extendido en plan-1
  ```
- Instanciar `CompanyConfigRepository`:
  ```go
  companyConfigRepo := companyrepository.NewMySQLCompanyConfigRepository(db)
  ```
- Instanciar service (pasa `db` al service, que maneja transacciones internamente):
  ```go
  reservationSvc := service.NewReservationService(db, productRepo, orderItemRepo, orderRepo, logger)
  ```
- Instanciar use case (NO pasa `db`, solo orquesta):
  ```go
  return usecase.NewReserveAndAddUseCase(orderRepo, companyConfigRepo, reservationSvc, logger)
  ```

**Nota:** El import de `companyrepository` referencia `internal/company/repository/` (creado en plan-1). Cuando spec-3 agregue el controller, `NewModule` cambiará para retornar `*controller.Controller`.

---

## Paso 6: Tests unitarios del service

**Archivo:** `internal/order/service/reservation_service_test.go` (NUEVO)

**Qué testear (tabla de casos):**

| Test | Setup | Resultado esperado |
|------|-------|--------------------|
| `TestReserveItems_AllSuccess` | 2 productos activos con stock suficiente, `hasStockControl=true` | `ALL_SUCCESS`, 2 successes, 0 failures, `TotalPrice > 0` |
| `TestReserveItems_AllFailed_NotFound` | Todos los productos retornan `NotFoundError` | `ALL_FAILED`, 0 successes, failures con `NOT_FOUND` |
| `TestReserveItems_Partial` | 1 producto ok, 1 sin stock | `PARTIAL`, 1 success, 1 failure `OUT_OF_STOCK` |
| `TestReserveItems_OutOfStock` | `available == 0` | Failure `OUT_OF_STOCK` |
| `TestReserveItems_InsufficientAvailable` | `available < quantity` | Failure `INSUFFICIENT_AVAILABLE` |
| `TestReserveItems_ProductInactive` | `product.IsActive == false` | Failure `PRODUCT_INACTIVE` |
| `TestReserveItems_NoStockControl` | `hasStockControl=false`, sin stock | `ALL_SUCCESS` (skip validación) |
| `TestReserveItems_ProductNotStockeable` | `product.Stockeable=false` | `ALL_SUCCESS` (skip validación) |
| `TestReserveItems_DBErrorOnIncrement` | `IncrementReservedStock` retorna error DB | Error propagado, no ItemFailure |

**Patrón de mocks:** Implementar interfaces `ProductRepository` y `OrderItemRepository` como structs con campos de función (`FindByIDForUpdateFn func(...) (...)`) dentro del archivo de test. No usar librerías de mocking externas (CLAUDE.md: no agregar libs sin justificación).

---

## Paso 7: Tests unitarios del use case

**Archivo:** `internal/order/usecase/reserve_and_add_use_case_test.go` (NUEVO)

**Qué testear:**

| Test | Setup | Resultado esperado |
|------|-------|--------------------|
| `TestExecute_OrderNotFound` | `orderRepo.FindByID` retorna `NotFoundError` | Error `NotFoundError` |
| `TestExecute_OrderNotPending` | `order.Status = "CREATED"` | Error `ConflictError` |
| `TestExecute_CompanyMismatch` | `order.CompanyID != companyID` | Error `ForbiddenError` |
| `TestExecute_CompanyConfigNotFound` | `companyConfigRepo` retorna `NotFoundError` | Error `NotFoundError` |
| `TestExecute_AllSuccess` | Todo ok, service retorna `ALL_SUCCESS` | Result `ALL_SUCCESS`, UpdateStatus y UpdateTotalPrice llamados |
| `TestExecute_AllFailed` | Service retorna `ALL_FAILED` | Result `ALL_FAILED`, NO UpdateStatus llamado, ROLLBACK |
| `TestExecute_ItemsSortedByProductID` | Items en orden inverso | Service recibe items ordenados ASC |
| `TestExecute_DeadlockRetry` | Primera llamada retorna deadlock, segunda ok | Result ok, 2 intentos |
| `TestExecute_DeadlockMaxRetries` | Todas las llamadas retornan deadlock | Error `DeadlockError` |

**Nota sobre tests de transacción:** Para testear `runTransaction` en aislamiento se necesita una DB de test o un mock de `*sql.DB`. Dado que el proyecto usa `database/sql` puro, los tests del use case que involucran `db.BeginTx` requerirán una instancia real de MySQL de test (integration test) o usar la técnica de inyectar un `TxBeginner` interface. Documentar esta decisión en el test file con un comentario.

**Alternativa pragmática documentada en el test:** Separar en dos grupos:
- Tests que no requieren DB (pre-validaciones): unit tests puros con mocks
- Tests de `runTransaction` + retry: marcar con `//go:build integration` y usar DB de test

---

## Paso 8: Compilar y verificar

**Comandos en orden:**
```bash
go build ./...
go vet ./...
go test ./internal/order/service/... -v
go test ./internal/order/usecase/... -v -run Unit
```

**Qué verificar:**
- Todos los archivos compilan sin errores
- Las interfaces en `service` son satisfechas por los repos de plan-1 (el compilador lo verifica)
- Las interfaces en `usecase` son satisfechas por `ReservationService` y repos (ídem)
- Los unit tests del service pasan sin DB real
- No hay imports circulares entre `service` y `usecase` (el service no importa el usecase)

---

## Orden de ejecución

```
Paso 1 (reservation_result.go)
    ↓
Paso 2 (errors: ConflictError, ForbiddenError, DeadlockError)
    ↓
Paso 3 (reservation_service.go) — depende de tipos de Paso 1 y errores
    ↓
Paso 4 (reserve_and_add_use_case.go) — depende de Paso 3 (interfaces) y Paso 2 (errors)
    ↓
Paso 5 (wire.go) — depende de Paso 3 y Paso 4
    ↓
Paso 6 (service tests) — depende de Paso 3
    ↓
Paso 7 (use case tests) — depende de Paso 4
    ↓
Paso 8 (compilar y verificar)
```

---

## Archivos tocados (resumen)

| # | Archivo | Acción |
|---|---------|--------|
| 1 | `internal/dto/reservation.go` | CREAR (ReservationResult, ItemSuccess, ItemFailure, FailureReason, ReservationStatus) |
| 2 | `internal/errors/errors.go` | EDITAR (agregar ConflictError, ForbiddenError, DeadlockError) |
| 3 | `internal/order/service/reservation_service.go` | CREAR |
| 4 | `internal/order/usecase/reserve_and_add_use_case.go` | CREAR |
| 5 | `internal/order/wire.go` | CREAR |
| 6 | `internal/order/service/reservation_service_test.go` | CREAR |
| 7 | `internal/order/usecase/reserve_and_add_use_case_test.go` | CREAR |

**No se toca:** domain entities, repos (plan-1), DTOs HTTP, controllers, router — eso es scope de spec-3.

---

## Decisiones de diseño

1. **`ReservationResult`, `ItemSuccess`, `ItemFailure` en `internal/dto/`:** Los tipos de resultado son DTOs de dominio (resultado de una operación de negocio), no entidades persistidas ni tipos de infraestructura. Van en `internal/dto/` centralizados. Esto permite que el controller (spec-3) acceda directamente a estos tipos sin importar `service` internamente. El service y usecase importan desde `dto/`.

2. **`*sql.DB` en el service, no en el use case:** El service controla el ciclo de vida de la transacción (`BeginTx`, `Commit`, `Rollback`) porque la reserva de stock es una operación atómica que requiere transacción. El use case solo orquesta pre-validaciones, mapeo de datos y retry lógico. El service recibe `TransactionManager` (interfaz) para no acoplarse a `*sql.DB` directamente.

3. **`defer tx.Rollback()` siempre presente:** Es el patrón idiomático de Go para garantizar rollback en caso de error. MySQL ignora el rollback si ya se hizo commit. Evita leaks de transacción en cualquier path de error.

4. **`TransactionManager` interface para `*sql.DB`:** En lugar de acoplar el service a `*sql.DB` directamente, se define una interfaz mínima `TransactionManager` que solo requiere `BeginTx`. Esto permite tests con mocks sin necesidad de una DB real. Se implementa trivialmente con `*sql.DB`, cumpliendo con la regla de "interfaces en el consumidor".

5. **Interfaces en el consumidor, no en el productor (CLAUDE.md):** `StockReservationService` se define en el paquete `usecase`. `ProductRepository` y `OrderItemRepository` se definen en el paquete `service`. Ningún repositorio sabe que existe una interface que lo describe.

6. **Retry con jitter para evitar thundering herd:** El spec especifica `±20%` de jitter. Se implementa con `rand.Float64()` en el rango `[0.8, 1.2]` multiplicado por el backoff base. No se necesita `rand.Seed` en Go 1.20+ (semilla automática).

7. **`sort.Slice` en lugar de `sort.Sort`:** `sort.Slice` es suficiente para el slice de `ReservationItem` sin implementar la interface `sort.Interface`. Mantiene el código mínimo.
