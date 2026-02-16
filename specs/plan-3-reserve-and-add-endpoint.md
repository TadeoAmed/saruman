# Plan 3: Reserve-and-Add Endpoint

**Spec:** `specs/spec-3-reserve-and-add-endpoint.md`
**Fecha:** 2026-02-16
**Estado:** Listo para ejecución
**Depende de:** plan-1 (domain + repositories) y plan-2 (service + usecase) deben estar implementados

---

## Resumen

Implementar la capa HTTP del endpoint `POST /orders/{orderId}/reserve-and-add`. Este plan cubre exactamente lo que delimita spec-3: DTOs de request/response, validación de input en el controller, mapeo de errores de dominio a HTTP status codes, wiring completo del módulo `order` (reemplazando el wire.go provisional de plan-2), y registro de la ruta en el router Chi.

**No se toca:** domain entities, repositorios, service ni use case — eso es scope de plan-1 y plan-2.

---

## Análisis de dependencias

### Lo que ya existe (plan-1 y plan-2)

| Archivo | Estado |
|---------|--------|
| `internal/domain/order.go` | Implementado (`OrderStatusPending`, `OrderStatusCreated`) |
| `internal/domain/order_item.go` | Implementado |
| `internal/domain/product.go` | Implementado (`AvailableStock()`) |
| `internal/order/repository/order_repository.go` | Implementado (`FindByID`, `UpdateStatus`, `UpdateTotalPrice`) |
| `internal/order/repository/order_item_repository.go` | Implementado (`Insert`) |
| `internal/company/repository/company_config_repository.go` | Implementado |
| `internal/product/repository/products_repository.go` | Implementado (con `FindByIDForUpdate`, `IncrementReservedStock` — plan-1) |
| `internal/errors/errors.go` | Tiene `ValidationError`, `InternalError`, `NotFoundError`. **Falta** `ConflictError`, `ForbiddenError`, `DeadlockError` si plan-2 no los creó |
| `internal/dto/reservation.go` | Debe existir con `ReservationResult`, `ReservationStatus`, `ItemSuccess`, `ItemFailure`, `ReservationItem` (plan-2) |
| `internal/order/service/reservation_service.go` | Debe existir (plan-2) |
| `internal/order/usecase/reserve_and_add_use_case.go` | Debe existir (plan-2). Su método: `Execute(ctx, orderID uint, companyID int, items []dto.ReservationItem) (*dto.ReservationResult, error)` |
| `internal/order/wire.go` | Creado provisionalmente en plan-2 (retorna `*usecase.ReserveAndAddUseCase`). Este plan lo reemplaza para retornar `*controller.ReserveAndAddController` |

### Archivos nuevos que este plan crea

| Archivo | Acción |
|---------|--------|
| `internal/dto/reserve_and_add_request.go` | CREAR |
| `internal/dto/reserve_and_add_response.go` | CREAR |
| `internal/order/controller/reserve_and_add_controller.go` | CREAR |
| `internal/order/wire.go` | REEMPLAZAR (ahora retorna el controller) |
| `internal/server/router.go` | EDITAR (agregar ruta + parámetro orderController) |

### Archivos a verificar/editar si plan-2 no los completó

| Archivo | Verificar |
|---------|-----------|
| `internal/errors/errors.go` | Si faltan `ConflictError`, `ForbiddenError`, `DeadlockError` — agregarlos aquí |

---

## Pasos de implementación

---

### Paso 1: Verificar errores de dominio en `internal/errors/errors.go`

**Archivo:** `internal/errors/errors.go` (EDITAR si faltan tipos)

**Qué hacer:**

Verificar que existan `ConflictError`, `ForbiddenError` y `DeadlockError`. Si plan-2 ya los creó, saltar este paso. Si no, agregar:

```go
// ConflictError indicates the order cannot be processed due to invalid state (e.g., not PENDING).
// Maps to HTTP 409 Conflict in the controller.
type ConflictError struct {
    Message string
}

func (e *ConflictError) Error() string { return e.Message }

func NewConflictError(message string) *ConflictError {
    return &ConflictError{Message: message}
}

func IsConflictError(err error) (*ConflictError, bool) {
    if ce, ok := err.(*ConflictError); ok {
        return ce, true
    }
    return nil, false
}

// ForbiddenError indicates the requesting company is not the owner of the order.
// Maps to HTTP 403 Forbidden in the controller.
type ForbiddenError struct {
    Message string
}

func (e *ForbiddenError) Error() string { return e.Message }

func NewForbiddenError(message string) *ForbiddenError {
    return &ForbiddenError{Message: message}
}

func IsForbiddenError(err error) (*ForbiddenError, bool) {
    if fe, ok := err.(*ForbiddenError); ok {
        return fe, true
    }
    return nil, false
}

// DeadlockError wraps MySQL deadlock errors (codes 1213, 1205) for retry logic.
// Returned when max retry attempts exceeded. Maps to HTTP 409 with retryable: true.
type DeadlockError struct {
    Message string
}

func (e *DeadlockError) Error() string { return e.Message }

func NewDeadlockError(message string) *DeadlockError {
    return &DeadlockError{Message: message}
}

func IsDeadlockError(err error) (*DeadlockError, bool) {
    if de, ok := err.(*DeadlockError); ok {
        return de, true
    }
    return nil, false
}
```

**Dependencias:** Ninguna.
**Criterios cubiertos:** Mapeo de errores a HTTP (sección 10 y tabla 5.1 del spec).

---

### Paso 2: Crear DTOs de request — `internal/dto/reserve_and_add_request.go`

**Archivo:** `internal/dto/reserve_and_add_request.go` (CREAR)

**Contenido completo:**

```go
package dto

// ReserveAndAddRequest is the HTTP request body for POST /orders/{orderId}/reserve-and-add.
type ReserveAndAddRequest struct {
    CompanyID int                  `json:"companyId"`
    Items     []ReserveAndAddItem  `json:"items"`
}

// ReserveAndAddItem represents a single product to reserve and add to the order.
type ReserveAndAddItem struct {
    ProductID int     `json:"productId"`
    Quantity  int     `json:"quantity"`
    Price     float64 `json:"price"`
}
```

**Notas:**
- `Price` es `float64`, no string. El spec muestra `"10.50"` en el JSON de ejemplo pero la definición Go en sección 3.1 usa `float64`. Se sigue la definición Go (float64 es el tipo correcto para cálculos de precio interno).
- No hay tags de validación — la validación se hace manualmente en el controller (sin librerías externas, según CLAUDE.md).

**Dependencias:** Ninguna.
**Criterios cubiertos:** Parseo del request body (sección 3.1 del spec).

---

### Paso 3: Crear DTOs de response — `internal/dto/reserve_and_add_response.go`

**Archivo:** `internal/dto/reserve_and_add_response.go` (CREAR)

**Contenido completo:**

```go
package dto

import "time"

// ReserveAndAddResponse is the HTTP response body for 200 OK and 206 Partial Content.
type ReserveAndAddResponse struct {
    TraceID    string           `json:"traceId"`
    OrderID    uint             `json:"orderId"`
    Status     string           `json:"status"`
    TotalPrice float64          `json:"totalPrice"`
    AddedItems []int            `json:"addedItems"`
    Successes  []ItemSuccessDTO `json:"successes"`
    Failures   []ItemFailureDTO `json:"failures"`
    Timestamp  time.Time        `json:"timestamp"`
}

// ItemSuccessDTO represents a successfully reserved item in the response.
type ItemSuccessDTO struct {
    ProductID int `json:"productId"`
    Quantity  int `json:"quantity"`
}

// ItemFailureDTO represents a failed reservation attempt in the response.
type ItemFailureDTO struct {
    ProductID int    `json:"productId"`
    Quantity  int    `json:"quantity"`
    Reason    string `json:"reason"`
}

// ReserveAndAddErrorResponse is the HTTP response body for 422 Unprocessable Entity and other error statuses.
type ReserveAndAddErrorResponse struct {
    TraceID   string                     `json:"traceId"`
    Status    int                        `json:"status"`
    Message   string                     `json:"message"`
    Code      string                     `json:"code"`
    OrderID   uint                       `json:"orderId,omitempty"`
    Details   *ReserveAndAddErrorDetails `json:"details,omitempty"`
    Retryable bool                       `json:"retryable,omitempty"`
    Timestamp time.Time                  `json:"timestamp"`
}

// ReserveAndAddErrorDetails holds the per-item failure list for error responses.
type ReserveAndAddErrorDetails struct {
    Failures []ItemFailureDTO `json:"failures"`
}
```

**Notas:**
- `Retryable bool` con `omitempty` — solo aparece en la respuesta cuando es `true` (caso `DeadlockError`).
- `OrderID` con `omitempty` — para errores que ocurren antes de identificar la orden (ej: 400), el campo queda omitido.
- `Failures` en `ReserveAndAddResponse` se inicializa siempre como slice vacío `[]ItemFailureDTO{}` en el controller para evitar `null` en JSON (importante para el cliente).
- `AddedItems` también se inicializa como `[]int{}` para evitar `null`.

**Dependencias:** Ninguna.
**Criterios cubiertos:** Secciones 3.2, 3.3, 3.4, 3.6 del spec.

---

### Paso 4: Crear el controller — `internal/order/controller/reserve_and_add_controller.go`

**Archivo:** `internal/order/controller/reserve_and_add_controller.go` (CREAR)

Este es el paso más extenso. Se detalla por sub-secciones.

#### 4.1 Interface del use case (definida en este paquete — regla CLAUDE.md)

```go
// ReserveAndAddUseCase is the interface consumed by the controller.
// Defined here, in the consumer package, following CLAUDE.md interface conventions.
type ReserveAndAddUseCase interface {
    Execute(
        ctx context.Context,
        orderID uint,
        companyID int,
        items []dto.ReservationItem,
    ) (*dto.ReservationResult, error)
}
```

**Nota crítica:** `dto.ReservationItem` es el tipo definido en `internal/dto/reservation.go` (plan-2). El controller mapea `dto.ReserveAndAddItem` (request HTTP) → `dto.ReservationItem` (dominio) antes de llamar al use case.

#### 4.2 Struct y constructor

```go
type ReserveAndAddController struct {
    useCase ReserveAndAddUseCase
    logger  *zap.Logger
}

func NewReserveAndAddController(useCase ReserveAndAddUseCase, logger *zap.Logger) *ReserveAndAddController {
    return &ReserveAndAddController{
        useCase: useCase,
        logger:  logger,
    }
}
```

#### 4.3 Handler `ReserveAndAdd`

Firma: `func (c *ReserveAndAddController) ReserveAndAdd(w http.ResponseWriter, r *http.Request)`

Implementación paso a paso:

**Bloque 1: Generar traceId**
```go
traceID := uuid.New().String()
logger := c.logger.With(zap.String("traceId", traceID))
```

**Bloque 2: Parsear orderId del path**
```go
orderIDStr := chi.URLParam(r, "orderId")
orderIDParsed, err := strconv.ParseUint(orderIDStr, 10, 64)
if err != nil || orderIDParsed == 0 {
    c.writeErrorResponse(w, http.StatusBadRequest, traceID, 0, "VALIDATION_ERROR",
        "orderId must be a positive integer", nil, false)
    return
}
orderID := uint(orderIDParsed)
```

**Bloque 3: Decodificar body JSON**
```go
var req dto.ReserveAndAddRequest
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    c.writeValidationErrors(w, traceID, []apperrors.ValidationDetail{
        {Field: "body", Message: "request body must be valid JSON"},
    })
    return
}
```

**Bloque 4: Validar campos del request (acumular todos los errores)**

Llamar a `c.validateRequest(req)` que retorna `[]apperrors.ValidationDetail`. Si hay errores, responder 400 con la lista completa. Ver sección 4.4.

**Bloque 5: Mapear DTO HTTP → DTO de dominio**
```go
items := make([]dto.ReservationItem, len(req.Items))
for i, item := range req.Items {
    items[i] = dto.ReservationItem{
        ProductID: item.ProductID,
        Quantity:  item.Quantity,
        Price:     item.Price,
    }
}
```

**Bloque 6: Ejecutar use case**
```go
result, err := c.useCase.Execute(r.Context(), orderID, req.CompanyID, items)
```

**Bloque 7: Manejar errores del use case**

Usar type switches con los helpers de `apperrors`:
```go
if err != nil {
    logger.Error("reserve-and-add failed", zap.Error(err), zap.Uint("orderId", orderID))
    switch {
    case apperrors.IsNotFoundError:    → 404
    case apperrors.IsConflictError:    → 409
    case apperrors.IsForbiddenError:   → 403
    case apperrors.IsDeadlockError:    → 409 con retryable: true
    default:                           → 500
    }
    return
}
```

**Bloque 8: Mapear resultado a response HTTP**

Tabla de mapeo según spec sección 5.1:
- `dto.ReservationAllSuccess` → 200 con `ReserveAndAddResponse`
- `dto.ReservationPartial` → 206 con `ReserveAndAddResponse`
- `dto.ReservationAllFailed` → 422 con `ReserveAndAddErrorResponse`

Construcción del `ReserveAndAddResponse` para 200/206:
```go
successes := make([]dto.ItemSuccessDTO, len(result.Successes))
addedItems := make([]int, len(result.Successes))
for i, s := range result.Successes {
    successes[i] = dto.ItemSuccessDTO{ProductID: s.ProductID, Quantity: s.Quantity}
    addedItems[i] = s.ProductID
}

failures := make([]dto.ItemFailureDTO, len(result.Failures))
for i, f := range result.Failures {
    failures[i] = dto.ItemFailureDTO{
        ProductID: f.ProductID,
        Quantity:  f.Quantity,
        Reason:    string(f.Reason),
    }
}

response := dto.ReserveAndAddResponse{
    TraceID:    traceID,
    OrderID:    result.OrderID,
    Status:     string(result.Status),
    TotalPrice: result.TotalPrice,
    AddedItems: addedItems,
    Successes:  successes,
    Failures:   failures,
    Timestamp:  time.Now().UTC(),
}
```

#### 4.4 Método privado `validateRequest`

Firma: `func (c *ReserveAndAddController) validateRequest(req dto.ReserveAndAddRequest) []apperrors.ValidationDetail`

Reglas de validación (acumular todos los errores en un slice, nunca retornar early):

| Validación | Condición | Mensaje |
|------------|-----------|---------|
| companyId requerido | `req.CompanyID == 0` | `"companyId is required"` |
| companyId positivo | `req.CompanyID < 0` | `"companyId must be a positive integer"` |
| items requerido | `req.Items == nil` | `"items is required"` |
| items no vacío | `len(req.Items) == 0` | `"items must not be empty"` |
| items máximo 100 | `len(req.Items) > 100` | `"items exceeds maximum of 100"` |
| Por cada item[i]: | | |
| productId requerido | `item.ProductID == 0` | `"productId is required"` |
| productId positivo | `item.ProductID < 0` | `"productId must be a positive integer"` |
| quantity rango | `item.Quantity < 1 \|\| item.Quantity > 10000` | `"quantity must be between 1 and 10000"` |
| price no negativo | `item.Price < 0` | `"price must be non-negative"` |
| productIds duplicados | set de IDs ya vistos | `fmt.Sprintf("duplicate productId: %d", item.ProductID)` |

Campos de `ValidationDetail.Field` para items: `fmt.Sprintf("items[%d].productId", i)`, `fmt.Sprintf("items[%d].quantity", i)`, etc.

**Nota sobre early return en items:** Si `req.Items == nil` o vacío, no iterar — agregar ese error y retornar la lista sin continuar con las validaciones de items individuales. Si `len > 100`, agregar ese error pero NO iterar los items (evitar validar 1000 items de un request inválido).

#### 4.5 Métodos auxiliares de respuesta

```go
// writeJSON serializes data as JSON and writes it with the given status code.
func (c *ReserveAndAddController) writeJSON(w http.ResponseWriter, status int, data any)

// writeValidationErrors writes a 400 response with the accumulated validation details.
// Uses the same ValidationError response format as the existing product controller.
func (c *ReserveAndAddController) writeValidationErrors(w http.ResponseWriter, traceID string, details []apperrors.ValidationDetail)

// writeErrorResponse writes a structured error response for domain errors (404, 403, 409, 500).
func (c *ReserveAndAddController) writeErrorResponse(
    w http.ResponseWriter,
    status int,
    traceID string,
    orderID uint,
    code string,
    message string,
    failures []dto.ItemFailureDTO,
    retryable bool,
)
```

**Nota sobre `writeValidationErrors`:** La respuesta de validación (400) usa un formato diferente al de otros errores (no tiene `traceId` en el body — ver spec sección 3.5). Esto es intencional — el spec define dos formatos de error distintos.

**Imports requeridos:**
```go
import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "strconv"
    "time"

    "saruman/internal/dto"
    apperrors "saruman/internal/errors"

    "github.com/go-chi/chi/v5"
    "github.com/google/uuid"
    "go.uber.org/zap"
)
```

**Dependencias:** Pasos 1, 2 y 3.
**Criterios cubiertos:** Todos los criterios de aceptación de la sección 11 del spec, excepto routing y DI.

---

### Paso 5: Reemplazar `internal/order/wire.go`

**Archivo:** `internal/order/wire.go` (REEMPLAZAR)

Plan-2 creó un `wire.go` provisional que retornaba `*usecase.ReserveAndAddUseCase`. Este paso lo reemplaza para retornar `*controller.ReserveAndAddController`, cerrando el ciclo de DI completo.

**Contenido completo:**

```go
package order

import (
    "database/sql"

    companyrepository "saruman/internal/company/repository"
    orderrepository   "saruman/internal/order/repository"
    "saruman/internal/order/controller"
    "saruman/internal/order/service"
    "saruman/internal/order/usecase"
    productrepository "saruman/internal/product/repository"

    "go.uber.org/zap"
)

// NewModule wires together all dependencies for the order module.
// Returns the controller ready to be registered in the router.
// Dependency chain: repos → service → usecase → controller.
func NewModule(db *sql.DB, logger *zap.Logger) *controller.ReserveAndAddController {
    // Repositories
    orderRepo        := orderrepository.NewMySQLOrderRepository(db)
    orderItemRepo    := orderrepository.NewMySQLOrderItemRepository(db)
    productRepo      := productrepository.NewMySQLRepository(db)
    companyConfigRepo := companyrepository.NewMySQLCompanyConfigRepository(db)

    // Service (owns transaction lifecycle)
    reservationSvc := service.NewReservationService(db, productRepo, orderItemRepo, orderRepo, logger)

    // Use case (orchestrates, no transaction logic)
    reserveAndAddUC := usecase.NewReserveAndAddUseCase(orderRepo, companyConfigRepo, reservationSvc, logger)

    // Controller (HTTP layer)
    return controller.NewReserveAndAddController(reserveAndAddUC, logger)
}
```

**Notas:**
- El alias `orderrepository` agrupa los dos repos del módulo order en un solo import.
- `productrepository.NewMySQLRepository` es el constructor existente del product repository (ya extendido en plan-1 con `FindByIDForUpdate` e `IncrementReservedStock`).
- Si el nombre del constructor de `companyrepository` difiere del plan-1, ajustar al nombre real del archivo generado.

**Dependencias:** Paso 4 (el controller debe existir para poder retornarlo).
**Criterios cubiertos:** `wire.go` exporta `NewModule(db, logger)` que retorna el controller (criterio de DI manual).

---

### Paso 6: Actualizar `internal/server/router.go`

**Archivo:** `internal/server/router.go` (EDITAR)

**Cambios:**

1. Agregar parámetro `orderCtrl *ordercontroller.ReserveAndAddController` a la firma de `NewRouter`.
2. Agregar import del paquete `ordercontroller "saruman/internal/order/controller"`.
3. Registrar la ruta dentro de `NewRouter`:

```go
r.Route("/orders/{orderId}", func(r chi.Router) {
    r.Post("/reserve-and-add", orderCtrl.ReserveAndAdd)
})
```

**Firma actualizada de `NewRouter`:**
```go
func NewRouter(
    productCtrl *productcontroller.Controller,
    orderCtrl *ordercontroller.ReserveAndAddController,
    logger *zap.Logger,
) *chi.Mux
```

**Nota sobre `main.go`:** El proyecto no tiene `main.go` todavía (o está vacío). Cuando se cree, deberá instanciar los módulos así:
```go
productCtrl := product.NewModule(db, logger)
orderCtrl   := order.NewModule(db, logger)
router      := server.NewRouter(productCtrl, orderCtrl, logger)
```

Si ya existe un `main.go`, agregar la línea `orderCtrl := order.NewModule(db, logger)` y pasar `orderCtrl` a `NewRouter`.

**Dependencias:** Paso 5.
**Criterios cubiertos:** `POST /orders/{orderId}/reserve-and-add` registrado en Chi router.

---

### Paso 7: Tests del controller — `internal/order/controller/reserve_and_add_controller_test.go`

**Archivo:** `internal/order/controller/reserve_and_add_controller_test.go` (CREAR)

**Patrón de mocks:** Igual que en los tests existentes del proyecto — struct con campo de función en el archivo de test. No se agregan librerías externas.

```go
type mockUseCase struct {
    ExecuteFn func(ctx context.Context, orderID uint, companyID int, items []dto.ReservationItem) (*dto.ReservationResult, error)
}

func (m *mockUseCase) Execute(ctx context.Context, orderID uint, companyID int, items []dto.ReservationItem) (*dto.ReservationResult, error) {
    return m.ExecuteFn(ctx, orderID, companyID, items)
}
```

**Tabla de tests:**

| Test | Request | Mock retorna | Esperado |
|------|---------|--------------|---------|
| `TestReserveAndAdd_AllSuccess` | orderId=1, companyId=10, 2 items válidos | `ReservationAllSuccess`, 2 successes | 200, `addedItems` con 2 IDs, `failures: []` |
| `TestReserveAndAdd_Partial` | orderId=1, companyId=10, 2 items válidos | `ReservationPartial`, 1 success + 1 failure | 206, successes y failures poblados |
| `TestReserveAndAdd_AllFailed` | orderId=1, companyId=10, 2 items válidos | `ReservationAllFailed`, 2 failures | 422, failures en `details` |
| `TestReserveAndAdd_OrderNotFound` | orderId=999 | `NotFoundError` | 404 |
| `TestReserveAndAdd_OrderNotPending` | orderId=1 | `ConflictError` | 409 |
| `TestReserveAndAdd_CompanyMismatch` | orderId=1 | `ForbiddenError` | 403 |
| `TestReserveAndAdd_Deadlock` | orderId=1 | `DeadlockError` | 409, `retryable: true` en body |
| `TestReserveAndAdd_InternalError` | orderId=1 | error genérico | 500 |
| `TestReserveAndAdd_InvalidOrderId` | orderId="abc" | — (no llega al use case) | 400 |
| `TestReserveAndAdd_InvalidOrderIdZero` | orderId="0" | — | 400 |
| `TestReserveAndAdd_MalformedJSON` | body inválido | — | 400 |
| `TestReserveAndAdd_MissingCompanyId` | companyId=0 | — | 400, field `companyId` en details |
| `TestReserveAndAdd_EmptyItems` | items=[] | — | 400, field `items` en details |
| `TestReserveAndAdd_ItemsExceedMax` | 101 items | — | 400, field `items` en details |
| `TestReserveAndAdd_DuplicateProductIds` | items con productId repetido | — | 400, `duplicate productId` en details |
| `TestReserveAndAdd_QuantityOutOfRange` | quantity=0 o quantity=10001 | — | 400, field `items[N].quantity` en details |
| `TestReserveAndAdd_NegativePrice` | price=-1 | — | 400, field `items[N].price` en details |
| `TestReserveAndAdd_AccumulatesAllValidationErrors` | companyId=0 + items vacío | — | 400, details con 2 errores |
| `TestReserveAndAdd_TraceIdPresentInResponse` | request válido | `ALL_SUCCESS` | traceId no vacío en body |
| `TestReserveAndAdd_TimestampPresentInResponse` | request válido | `ALL_SUCCESS` | timestamp en RFC3339 |
| `TestReserveAndAdd_AddedItemsMatchSuccesses` | request válido, 2 successes | `ALL_SUCCESS` | addedItems = [productId1, productId2] |

**Setup de cada test:**
```go
func makeRequest(t *testing.T, orderId, body string) (*httptest.ResponseRecorder, *http.Request) {
    req := httptest.NewRequest(http.MethodPost, "/orders/"+orderId+"/reserve-and-add", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    // Inyectar orderId en el contexto de Chi para chi.URLParam
    rctx := chi.NewRouteContext()
    rctx.URLParams.Add("orderId", orderId)
    req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
    return httptest.NewRecorder(), req
}
```

**Dependencias:** Paso 4.
**Criterios cubiertos:** Todos los criterios de aceptación de la sección 11 del spec (verificables por test).

---

### Paso 8: Compilar y verificar

**Comandos en orden:**

```bash
go build ./...
go vet ./...
go test ./internal/order/controller/... -v
go test ./internal/... -v -run "^Test" -count=1
```

**Qué verificar:**
- Todos los archivos compilan sin errores ni warnings de `go vet`
- La interface `ReserveAndAddUseCase` definida en el controller es satisfecha por `*usecase.ReserveAndAddUseCase` (el compilador lo verifica vía `wire.go`)
- `NewRouter` actualizado compila con los dos parámetros de controller
- Los tests del controller pasan sin DB real (todos son unit tests puros con mocks)
- `addedItems` nunca es `null` en JSON (verificar con `json.Marshal` en test)
- `failures` nunca es `null` cuando está vacío (verificar con `json.Marshal` en test)

---

## Orden de ejecución

```
Paso 1: Verificar/agregar errores de dominio (ConflictError, ForbiddenError, DeadlockError)
    ↓
Paso 2: Crear reserve_and_add_request.go (DTOs de request)
Paso 3: Crear reserve_and_add_response.go (DTOs de response)
    ↓ (Pasos 2 y 3 son independientes, se pueden hacer en paralelo)
Paso 4: Crear reserve_and_add_controller.go (depende de Pasos 1, 2 y 3)
    ↓
Paso 5: Reemplazar wire.go (depende de Paso 4)
    ↓
Paso 6: Actualizar router.go (depende de Paso 5)
    ↓
Paso 7: Tests del controller (depende de Paso 4)
    ↓
Paso 8: Compilar y verificar
```

---

## Archivos tocados (resumen)

| # | Archivo | Acción | Motivo |
|---|---------|--------|--------|
| 1 | `internal/errors/errors.go` | EDITAR (si faltan tipos) | `ConflictError`, `ForbiddenError`, `DeadlockError` para mapeo HTTP |
| 2 | `internal/dto/reserve_and_add_request.go` | CREAR | Request body del endpoint |
| 3 | `internal/dto/reserve_and_add_response.go` | CREAR | Response bodies (success, partial, error) |
| 4 | `internal/order/controller/reserve_and_add_controller.go` | CREAR | Capa HTTP: parseo, validación, mapeo de respuestas |
| 5 | `internal/order/wire.go` | REEMPLAZAR | DI completo: ahora retorna el controller en lugar del use case |
| 6 | `internal/server/router.go` | EDITAR | Registrar ruta + agregar parámetro orderCtrl |
| 7 | `internal/order/controller/reserve_and_add_controller_test.go` | CREAR | Tests unitarios del controller con mocks |

**No se toca:** `internal/domain/`, `internal/order/repository/`, `internal/order/service/`, `internal/order/usecase/`, `internal/company/`, `internal/product/`.

---

## Decisiones de diseño

1. **Interface `ReserveAndAddUseCase` en el paquete `controller`:** Sigue la regla de CLAUDE.md — las interfaces se definen en el paquete consumidor. El controller define qué necesita del use case; el use case concreto la satisface implícitamente sin saberlo.

2. **Mapeo `ReserveAndAddItem` → `ReservationItem` en el controller, no en el use case:** El controller es el responsable de transformar los DTOs HTTP en tipos de dominio antes de llamar al use case. Esto mantiene al use case libre de conocer el formato HTTP del request.

3. **`failures` y `addedItems` nunca `null` en JSON:** Inicializar siempre con `make([]T, 0)` o `make([]T, len(source))`. Un cliente REST que recibe `null` en lugar de `[]` puede romper sin cambios de contrato.

4. **Formato de error 400 distinto al de otros errores:** El spec define dos formatos: el de validación (sección 3.5, con `error`, `message`, `details[]`) y el de errores de dominio (sección 3.4, con `traceId`, `status`, `message`, `code`). El controller implementa ambos como métodos separados (`writeValidationErrors` vs `writeErrorResponse`).

5. **`DeadlockError` → 409 con `retryable: true`:** El spec lo define en la tabla 5.1. El campo `retryable` en `ReserveAndAddErrorResponse` usa `omitempty` para no aparecer en respuestas normales de conflicto.

6. **`wire.go` reemplaza completamente el de plan-2:** Plan-2 lo dejó provisional retornando el use case. Reemplazarlo es más limpio que modificarlo, ya que la firma de `NewModule` cambia (`*usecase.ReserveAndAddUseCase` → `*controller.ReserveAndAddController`).

7. **Validación acumula todos los errores:** No se hace early return en el método `validateRequest`. Se acumulan todos los problemas del request en un slice y se retornan en una sola respuesta 400. Esto mejora la experiencia del cliente que puede corregir múltiples problemas en un solo ciclo.
