# Spec 2: Stock Reservation Service

**Estado:** Draft
**Fecha:** 2026-02-15
**Módulo:** order (service + usecase)
**Depende de:** spec-1 (domain entities + repositories)

---

## 1. Objetivo

Definir la lógica de negocio y orquestación para el flujo de **reserve-and-add**: reservar stock de productos y crear order items dentro de una transacción atómica. Este spec cubre el **service** (lógica de dominio) y el **use case** (orquestación), sin endpoint HTTP.

**Alcance:**
- Service: validación de stock, reserva atómica, reglas de negocio
- Use Case: orquestación del flujo completo, manejo de transacción, retry ante deadlocks
- Modelo de resultado parcial (successes + failures)

**No incluido en este spec:**
- Domain entities ni queries SQL (spec-1)
- Endpoint HTTP, DTOs de request/response, controller (spec-3)
- Autenticación / autorización

---

## 2. Flujo de Orquestación (Use Case)

El use case orquesta el flujo completo de reserve-and-add. Recibe input ya validado del controller (spec-3) y coordina las capas inferiores.

### 2.1 Pseudocódigo del flujo

```
func ReserveAndAdd(ctx, orderId, companyId, items) -> ReservationResult:

  // 1. Pre-validaciones (fuera de transacción)
  order = OrderRepo.FindByID(orderId)
  if order == nil → return NotFoundError("order not found")
  if order.Status != PENDING → return ConflictError("order is not in PENDING status")
  if order.CompanyID != companyId → return ForbiddenError("company mismatch")

  companyConfig = CompanyConfigRepo.FindByCompanyID(companyId)
  if companyConfig == nil → return NotFoundError("company config not found")

  hasStockControl = companyConfig.HasStock

  // 2. Preparar items: ordenar por productId ASC (anti-deadlock)
  sortedItems = sort(items, by: productId ASC)

  // 3. Ejecutar transacción con retry
  result = executeWithRetry(func(tx) {
    return service.ReserveItems(ctx, tx, orderId, companyId, sortedItems, hasStockControl)
  })

  return result
```

### 2.2 Retry ante Deadlocks

```
func executeWithRetry(operation) -> result:
  maxAttempts = 3
  backoffs = [0ms, 100ms, 200ms]

  for attempt = 1..maxAttempts:
    result, err = operation()

    if err == nil:
      return result

    if isDeadlockError(err) AND attempt < maxAttempts:
      jitter = random(-20%, +20%) of backoffs[attempt]
      sleep(backoffs[attempt] + jitter)
      log.Warn("deadlock detected, retrying", attempt, orderId)
      continue

    return err

  return DeadlockError("max retries exceeded")
```

**Detección de deadlock:**
- MySQL error 1213: `ER_LOCK_DEADLOCK`
- MySQL error 1205: `ER_LOCK_WAIT_TIMEOUT`

---

## 3. Lógica de Negocio (Service)

### 3.1 ReserveItems - Core del servicio

El service procesa cada item dentro de la transacción, acumulando successes y failures.

```
func ReserveItems(ctx, tx, orderId, companyId, items, hasStockControl) -> ReservationResult:
  successes = []
  failures = []
  totalPrice = 0.0

  for each item in items:
    result = reserveSingleItem(ctx, tx, orderId, companyId, item, hasStockControl)

    if result.success:
      successes.append(result)
      totalPrice += item.price * item.quantity
    else:
      failures.append(result)

  // Decidir resultado final
  if len(successes) == 0:
    ROLLBACK tx
    return ReservationResult{
      status: ALL_FAILED,
      failures: failures,
    }

  // Al menos 1 success → COMMIT
  OrderRepo.UpdateStatus(tx, orderId, CREATED)
  OrderRepo.UpdateTotalPrice(tx, orderId, totalPrice)
  COMMIT tx

  if len(failures) == 0:
    return ReservationResult{status: ALL_SUCCESS, successes, failures}
  else:
    return ReservationResult{status: PARTIAL, successes, failures}
```

### 3.2 reserveSingleItem - Reserva individual

```
func reserveSingleItem(ctx, tx, orderId, companyId, item, hasStockControl) -> ItemResult:

  // 1. Obtener producto con lock
  product = ProductRepo.FindByIDForUpdate(tx, item.productId, companyId)

  if product == nil:
    return failure(item.productId, item.quantity, NOT_FOUND)

  // 2. Validar estado del producto
  if !product.IsActive:
    return failure(item.productId, item.quantity, PRODUCT_INACTIVE)

  // 3. Validar stock (solo si company tiene control de stock)
  if hasStockControl AND product.HasStock AND product.Stockeable:
    available = product.AvailableStock()

    if available == 0:
      return failure(item.productId, item.quantity, OUT_OF_STOCK)

    if available < item.quantity:
      return failure(item.productId, item.quantity, INSUFFICIENT_AVAILABLE)

    // 4. Reservar stock
    ProductRepo.IncrementReservedStock(tx, item.productId, item.quantity)

  // 5. Crear order item
  orderItem = OrderItem{
    OrderID:   orderId,
    ProductID: item.productId,
    Quantity:  item.quantity,
    Price:     item.price,
  }
  OrderItemRepo.Insert(tx, orderItem)

  return success(item.productId, item.quantity)
```

---

## 4. Reglas de Negocio

### 4.1 Validación de Stock

| Condición | Resultado |
|-----------|-----------|
| `CompanyConfig.hasStock == false` | Skip validación de stock. Crear order item directamente. |
| `Product.hasStock == false` OR `Product.Stockeable == false` | Skip validación de stock para este producto. Crear order item directamente. |
| `Product.AvailableStock() == 0` | Failure: `OUT_OF_STOCK` |
| `Product.AvailableStock() < quantity` | Failure: `INSUFFICIENT_AVAILABLE` |
| `Product.AvailableStock() >= quantity` | Success: reservar stock + crear order item |

### 4.2 Validación de Producto

| Condición | Resultado |
|-----------|-----------|
| Producto no encontrado (con companyId match) | Failure: `NOT_FOUND` |
| `Product.isActive == false` | Failure: `PRODUCT_INACTIVE` |
| `Product.isDeleted == true` | Filtrado en query SQL (nunca llega al service) |

### 4.3 Validación de Orden

| Condición | Resultado | HTTP Status |
|-----------|-----------|-------------|
| Orden no encontrada | Error | 404 |
| `Order.status != PENDING` | Error | 409 Conflict |
| `Order.companyId != companyId` del request | Error | 403 Forbidden |

### 4.4 Resultado Parcial

| Escenario | Status resultado | HTTP Status (spec-3) |
|-----------|-----------------|---------------------|
| Todos los items reservados | `ALL_SUCCESS` | 200 OK |
| Algunos items reservados, otros fallaron | `PARTIAL` | 206 Partial Content |
| Ningún item reservado | `ALL_FAILED` | 422 Unprocessable Entity |

---

## 5. Modelo de Resultado

### 5.1 ReservationResult

```go
type ReservationStatus string

const (
    ReservationAllSuccess ReservationStatus = "ALL_SUCCESS"
    ReservationPartial    ReservationStatus = "PARTIAL"
    ReservationAllFailed  ReservationStatus = "ALL_FAILED"
)

type ReservationResult struct {
    Status     ReservationStatus
    OrderID    uint
    TotalPrice float64
    Successes  []ItemSuccess
    Failures   []ItemFailure
}

type ItemSuccess struct {
    ProductID int
    Quantity  int
}

type ItemFailure struct {
    ProductID int
    Quantity  int
    Reason    FailureReason
}
```

### 5.2 FailureReason

```go
type FailureReason string

const (
    ReasonNotFound              FailureReason = "NOT_FOUND"
    ReasonOutOfStock            FailureReason = "OUT_OF_STOCK"
    ReasonInsufficientAvailable FailureReason = "INSUFFICIENT_AVAILABLE"
    ReasonProductInactive       FailureReason = "PRODUCT_INACTIVE"
)
```

**Nota:** Estos tipos viven en el service o en un paquete compartido del módulo order (no en domain). Son resultado de la operación de negocio, no entidades persistidas.

---

## 6. Ordenamiento Anti-Deadlock

**Regla:** Antes de iniciar la transacción, los items se ordenan por `productId ASC`.

**Por qué:** Si dos transacciones concurrentes bloquean productos en diferente orden, se produce deadlock. Al garantizar que todos bloquean en el mismo orden (ASC), se elimina esta condición.

```
Ejemplo sin ordenar:
  TX-1: lock product 5, luego lock product 3  → DEADLOCK con TX-2
  TX-2: lock product 3, luego lock product 5

Ejemplo con ordenamiento ASC:
  TX-1: lock product 3, luego lock product 5  → OK, secuencial
  TX-2: lock product 3 (espera TX-1), luego lock product 5
```

---

## 7. Manejo de Transacciones

### 7.1 Scope de la transacción

```
BEGIN TRANSACTION (REPEATABLE READ)
  ├── Para cada item (ordenado por productId ASC):
  │   ├── SELECT ... FOR UPDATE (lock producto)
  │   ├── Validar stock disponible
  │   ├── UPDATE reserved_stock (si aplica)
  │   └── INSERT OrderItem
  ├── UPDATE Orders.status = CREATED
  ├── UPDATE Orders.totalPrice = sum(successes)
  └── COMMIT
```

### 7.2 Rollback

- Si **ningún** item tuvo éxito → `ROLLBACK` la transacción completa.
- Si **al menos 1** item tuvo éxito → `COMMIT` (los fallidos simplemente no se insertaron).
- Si ocurre error inesperado en cualquier punto → `ROLLBACK`.

### 7.3 Timeout

- Timeout de transacción: 5 segundos máximo.
- Si la transacción excede el timeout, se hace ROLLBACK y se retorna error.

---

## 8. Interfaces del Service y UseCase

### 8.1 Service Interface

```go
// Definida en el paquete que la consume (usecase)
type StockReservationService interface {
    ReserveItems(
        ctx context.Context,
        tx *sql.Tx,
        orderID uint,
        companyID int,
        items []ReservationItem,
        hasStockControl bool,
    ) (*ReservationResult, error)
}

type ReservationItem struct {
    ProductID int
    Quantity  int
    Price     float64
}
```

### 8.2 UseCase Interface

```go
// Definida en el paquete que la consume (controller)
type ReserveAndAddUseCase interface {
    Execute(
        ctx context.Context,
        orderID uint,
        companyID int,
        items []ReservationItem,
    ) (*ReservationResult, error)
}
```

---

## 9. Logging

Eventos a loggear en el service/usecase:

| Evento | Level | Campos |
|--------|-------|--------|
| Reserve-and-add started | INFO | orderId, companyId, itemCount |
| Pre-validation passed | DEBUG | orderId, orderStatus, hasStockControl |
| Item reserved successfully | INFO | orderId, productId, quantity |
| Item reservation failed | WARN | orderId, productId, quantity, reason |
| Deadlock detected, retrying | WARN | orderId, attempt, maxAttempts |
| Transaction committed | INFO | orderId, successCount, failureCount, totalPrice |
| Transaction rolled back (all failed) | WARN | orderId, failureCount |
| Transaction error | ERROR | orderId, error |

---

## 10. Estructura de Carpetas

```
internal/
└── order/
    ├── service/
    │   └── reservation_service.go    # ReserveItems, reserveSingleItem
    ├── usecase/
    │   └── reserve_and_add_use_case.go  # Execute, retry, orquestación
    └── ...                           # repository (spec-1), controller (spec-3)
```

---

## 11. Criterios de Aceptación

- [ ] Use case orquesta: pre-validaciones → ordenar items → transacción con retry → resultado
- [ ] Service reserva items dentro de transacción: lock → validar → reservar → insert
- [ ] Items se procesan ordenados por `productId ASC` (anti-deadlock)
- [ ] Si `CompanyConfig.hasStock == false`, se crean order items sin validar stock
- [ ] Si `Product.hasStock == false` o `Product.Stockeable == false`, se crea order item sin validar stock
- [ ] Resultado parcial: successes + failures con razón específica por item
- [ ] Si todos fallan → ROLLBACK, status `ALL_FAILED`
- [ ] Si al menos 1 success → COMMIT, update order status a `CREATED` y totalPrice
- [ ] Retry automático ante deadlock (error 1213/1205): máx 3 intentos con backoff + jitter
- [ ] Order pre-validada: debe existir, status PENDING, companyId coincide
- [ ] Logging estructurado en cada paso crítico (start, item result, commit/rollback, retry)
- [ ] Timeout de transacción de 5 segundos
