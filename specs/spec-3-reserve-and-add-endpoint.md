# Spec 3: Reserve-and-Add Endpoint

**Estado:** Draft
**Fecha:** 2026-02-15
**Módulo:** order (controller + DTOs + wire + routing)
**Depende de:** spec-1 (domain + repositories), spec-2 (service + usecase)

---

## 1. Objetivo

Definir el endpoint HTTP `POST /orders/{orderId}/reserve-and-add` que expone el flujo de reserva de stock. Este spec cubre la **capa HTTP**: controller, DTOs, validación de input, serialización de response, routing y wiring de DI.

**Alcance:**
- Controller: parseo de request, validación, mapeo de resultado a HTTP response
- DTOs: request y response structures
- Validación de input (schema-level)
- Routing: registrar endpoint en Chi router
- Wire: DI manual del módulo completo (repo → service → usecase → controller)

**No incluido en este spec:**
- Domain entities ni queries SQL (spec-1)
- Lógica de negocio ni orquestación (spec-2)
- Autenticación / autorización (futuro)

---

## 2. Endpoint

### POST /orders/{orderId}/reserve-and-add

**Path Parameters:**
- `orderId` (string, requerido): ID numérico de la orden pre-existente en BD

**Request Headers:**
- `Content-Type: application/json` (requerido)

**Request Body:**

```json
{
  "companyId": 12,
  "items": [
    {
      "productId": 101,
      "quantity": 5,
      "price": "10.50"
    },
    {
      "productId": 202,
      "quantity": 2,
      "price": "25.00"
    }
  ]
}
```

---

## 3. DTOs

### 3.1 Request DTO

```go
// internal/dto/reserve_and_add_request.go
package dto

type ReserveAndAddRequest struct {
    CompanyID int                    `json:"companyId"`
    Items     []ReserveAndAddItem    `json:"items"`
}

type ReserveAndAddItem struct {
    ProductID int     `json:"productId"`
    Quantity  int     `json:"quantity"`
    Price     float64 `json:"price"`
}
```

### 3.2 Response DTO - Success (200 OK)

```json
{
  "traceId": "550e8400-e29b-41d4-a716-446655440000",
  "orderId": 123,
  "status": "CREATED",
  "totalPrice": 102.50,
  "addedItems": [101, 202],
  "successes": [
    { "productId": 101, "quantity": 5 },
    { "productId": 202, "quantity": 2 }
  ],
  "failures": [],
  "timestamp": "2026-02-15T10:30:45Z"
}
```

### 3.3 Response DTO - Partial (206 Partial Content)

```json
{
  "traceId": "550e8400-e29b-41d4-a716-446655440000",
  "orderId": 123,
  "status": "CREATED",
  "totalPrice": 52.50,
  "addedItems": [101],
  "successes": [
    { "productId": 101, "quantity": 5 }
  ],
  "failures": [
    { "productId": 202, "quantity": 2, "reason": "INSUFFICIENT_AVAILABLE" }
  ],
  "timestamp": "2026-02-15T10:30:45Z"
}
```

### 3.4 Response DTO - All Failed (422 Unprocessable Entity)

```json
{
  "traceId": "550e8400-e29b-41d4-a716-446655440000",
  "status": 422,
  "message": "No items could be reserved",
  "code": "NO_STOCK_AVAILABLE",
  "orderId": 123,
  "details": {
    "failures": [
      { "productId": 101, "quantity": 5, "reason": "INSUFFICIENT_AVAILABLE" },
      { "productId": 202, "quantity": 2, "reason": "OUT_OF_STOCK" }
    ]
  },
  "timestamp": "2026-02-15T10:30:45Z"
}
```

### 3.5 Response DTO - Validation Error (400 Bad Request)

```json
{
  "error": "VALIDATION_ERROR",
  "message": "Invalid request body",
  "details": [
    { "field": "companyId", "message": "companyId is required" },
    { "field": "items[1].quantity", "message": "quantity must be between 1 and 10000" }
  ]
}
```

### 3.6 Response DTOs en Go

```go
// internal/dto/reserve_and_add_response.go
package dto

import "time"

type ReserveAndAddResponse struct {
    TraceID    string              `json:"traceId"`
    OrderID    uint                `json:"orderId"`
    Status     string              `json:"status"`
    TotalPrice float64             `json:"totalPrice"`
    AddedItems []int               `json:"addedItems"`
    Successes  []ItemSuccessDTO    `json:"successes"`
    Failures   []ItemFailureDTO    `json:"failures"`
    Timestamp  time.Time           `json:"timestamp"`
}

type ItemSuccessDTO struct {
    ProductID int `json:"productId"`
    Quantity  int `json:"quantity"`
}

type ItemFailureDTO struct {
    ProductID int    `json:"productId"`
    Quantity  int    `json:"quantity"`
    Reason    string `json:"reason"`
}

type ReserveAndAddErrorResponse struct {
    TraceID   string                    `json:"traceId"`
    Status    int                       `json:"status"`
    Message   string                    `json:"message"`
    Code      string                    `json:"code"`
    OrderID   uint                      `json:"orderId"`
    Details   *ReserveAndAddErrorDetails `json:"details,omitempty"`
    Timestamp time.Time                 `json:"timestamp"`
}

type ReserveAndAddErrorDetails struct {
    Failures []ItemFailureDTO `json:"failures"`
}
```

---

## 4. Validación de Input

El controller valida el request **antes** de pasar al use case.

### 4.1 Reglas de Validación

| Campo | Regla | Mensaje de error |
|-------|-------|-----------------|
| `orderId` (path) | Numérico, > 0 | `orderId must be a positive integer` |
| `companyId` | Requerido, entero > 0 | `companyId is required` / `companyId must be a positive integer` |
| `items` | Requerido, array no vacío | `items is required` / `items must not be empty` |
| `items` | Máx 100 elementos | `items exceeds maximum of 100` |
| `items[].productId` | Requerido, entero > 0 | `productId is required` / `productId must be a positive integer` |
| `items[].quantity` | Requerido, 1 <= qty <= 10000 | `quantity must be between 1 and 10000` |
| `items[].price` | Requerido, >= 0 | `price must be non-negative` |
| `items` | Sin productIds duplicados | `duplicate productId: {id}` |

### 4.2 Flujo de Validación

```
1. Parsear orderId del path parameter (chi.URLParam)
   - Si no es numérico o <= 0 → 400

2. Decodificar JSON body → ReserveAndAddRequest
   - Si body malformado → 400

3. Validar campos del request
   - Acumular todos los errores de validación
   - Retornar 400 con details[] conteniendo todos los errores

4. Si válido → llamar usecase.Execute(ctx, orderId, companyId, items)
```

---

## 5. Mapeo de Resultado a HTTP Response

El controller mapea el resultado del use case a la respuesta HTTP correspondiente.

### 5.1 Tabla de Mapeo

| Resultado Use Case | HTTP Status | Response Body |
|-------------------|-------------|---------------|
| `ALL_SUCCESS` | 200 OK | ReserveAndAddResponse con successes, failures vacío |
| `PARTIAL` | 206 Partial Content | ReserveAndAddResponse con successes y failures |
| `ALL_FAILED` | 422 Unprocessable Entity | ReserveAndAddErrorResponse con failures en details |
| `NotFoundError` (order) | 404 Not Found | Error response genérico |
| `ConflictError` (order status) | 409 Conflict | Error response genérico |
| `ForbiddenError` (company mismatch) | 403 Forbidden | Error response genérico |
| `DeadlockError` (retries agotados) | 409 Conflict | Error response con `retryable: true` |
| Error inesperado | 500 Internal Server Error | Error response genérico |

### 5.2 TraceID

- Generar `traceId` (UUID v4) al inicio del request en el controller.
- Incluir en todas las respuestas y en los logs.
- Usar `google/uuid` para generación.

### 5.3 Timestamp

- Usar `time.Now().UTC()` al momento de construir la respuesta.
- Formato ISO 8601 (Go: `time.RFC3339`).

---

## 6. Routing

### 6.1 Registro en Chi Router

```go
// internal/server/router.go
r.Route("/orders/{orderId}", func(r chi.Router) {
    r.Post("/reserve-and-add", orderController.ReserveAndAdd)
})
```

### 6.2 Middleware aplicado

Los middleware globales ya configurados en el MVP aplican:
1. `middleware.Recoverer` - Panic recovery
2. `middleware.RequestID` - Request ID generation
3. Logging middleware - Request/response logging

---

## 7. Wire (DI)

### 7.1 Módulo Order - wire.go

```go
// internal/order/wire.go
package order

func NewModule(db *sql.DB, logger *zap.Logger) *controller.ReserveAndAddController {
    // Repositories
    orderRepo := orderrepository.New(db, logger)
    orderItemRepo := orderitemrepository.New(db, logger)
    companyConfigRepo := companyconfigrepository.New(db, logger)
    productRepo := productrepository.New(db, logger)  // extended

    // Service
    reservationService := service.NewReservationService(
        productRepo,
        orderItemRepo,
        logger,
    )

    // Use Case
    reserveAndAddUC := usecase.NewReserveAndAddUseCase(
        db,  // for transaction management
        orderRepo,
        companyConfigRepo,
        reservationService,
        logger,
    )

    // Controller
    ctrl := controller.NewReserveAndAddController(reserveAndAddUC, logger)

    return ctrl
}
```

### 7.2 main.go Integration

```go
// En main.go, después de inicializar DB y logger:
orderController := order.NewModule(db, logger)
// Pasar al router para registrar rutas
```

---

## 8. Controller - Pseudocódigo

```
func (c *Controller) ReserveAndAdd(w http.ResponseWriter, r *http.Request):
  // 1. Generate traceId
  traceId = uuid.New().String()
  logger = c.logger.With(zap.String("traceId", traceId))

  // 2. Parse orderId from path
  orderIdStr = chi.URLParam(r, "orderId")
  orderId, err = strconv.ParseUint(orderIdStr, 10, 64)
  if err → respond 400

  // 3. Decode request body
  var req dto.ReserveAndAddRequest
  err = json.NewDecoder(r.Body).Decode(&req)
  if err → respond 400

  // 4. Validate request
  validationErrors = validate(orderId, req)
  if len(validationErrors) > 0 → respond 400 with details

  // 5. Map DTO → domain items
  items = mapToReservationItems(req.Items)

  // 6. Execute use case
  result, err = c.useCase.Execute(ctx, uint(orderId), req.CompanyID, items)

  // 7. Handle errors
  if err:
    switch err type:
      NotFoundError → respond 404
      ConflictError → respond 409
      ForbiddenError → respond 403
      DeadlockError → respond 409 (retryable)
      default → respond 500

  // 8. Map result → response DTO
  switch result.Status:
    ALL_SUCCESS → respond 200 with ReserveAndAddResponse
    PARTIAL → respond 206 with ReserveAndAddResponse
    ALL_FAILED → respond 422 with ReserveAndAddErrorResponse
```

---

## 9. Estructura de Carpetas (Módulo order completo)

```
internal/
├── dto/
│   ├── reserve_and_add_request.go    # NUEVO
│   └── reserve_and_add_response.go   # NUEVO
├── order/
│   ├── controller/
│   │   └── reserve_and_add_controller.go  # NUEVO
│   ├── usecase/
│   │   └── reserve_and_add_use_case.go    # spec-2
│   ├── service/
│   │   └── reservation_service.go         # spec-2
│   ├── repository/
│   │   ├── order_repository.go            # spec-1
│   │   └── order_item_repository.go       # spec-1
│   └── wire.go                            # NUEVO
├── company/
│   └── repository/
│       └── company_config_repository.go   # spec-1
├── product/
│   └── repository/
│       └── product_repository.go          # Extendido (spec-1)
└── server/
    └── router.go                          # Extender con nueva ruta
```

---

## 10. Error Types

Los error types custom permiten al controller mapear errores de dominio a HTTP status codes.

```go
// internal/errors/errors.go (extender existente)

type NotFoundError struct {
    Message string
}

type ConflictError struct {
    Message string
}

type ForbiddenError struct {
    Message string
}

type DeadlockError struct {
    Message string
}
```

El controller hace type assertion para determinar el HTTP status code:
- `*errors.NotFoundError` → 404
- `*errors.ConflictError` → 409
- `*errors.ForbiddenError` → 403
- `*errors.DeadlockError` → 409 con `retryable: true`
- default error → 500

---

## 11. Criterios de Aceptación

- [ ] `POST /orders/{orderId}/reserve-and-add` registrado en Chi router
- [ ] Controller parsea `orderId` del path y body JSON correctamente
- [ ] Validación de input: companyId > 0, items no vacío, productId > 0, 1 <= quantity <= 10000, price >= 0
- [ ] Validación rechaza productIds duplicados en items
- [ ] Items limitados a máximo 100 por request
- [ ] Validación acumula todos los errores y retorna 400 con `details[]`
- [ ] Response 200 OK cuando todos los items se reservan exitosamente
- [ ] Response 206 Partial Content cuando algunos items fallan
- [ ] Response 422 cuando ningún item se puede reservar (con failures en details)
- [ ] Response 404 cuando la orden no existe
- [ ] Response 409 cuando la orden no está en status PENDING
- [ ] Response 403 cuando companyId no coincide con la orden
- [ ] TraceId (UUID v4) incluido en toda respuesta y logs
- [ ] Timestamp en formato ISO 8601 en toda respuesta
- [ ] `addedItems` contiene los productIds de los items reservados exitosamente
- [ ] `totalPrice` calculado como suma de (price * quantity) de items exitosos
- [ ] Error types custom mapeados correctamente a HTTP status codes
- [ ] DI manual en `wire.go`: repo → service → usecase → controller
- [ ] wire.go exporta función `NewModule(db, logger)` que retorna el controller
