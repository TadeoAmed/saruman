# Saruman - Arquitectura MVP Product Search

**Fecha:** 2026-02-13
**Estado:** MVP implementado
**Spec de referencia:** `specs/mvp-product-search.md`

---

## 1. Visión General

Saruman es un microservicio Go que reemplaza lógica de n8n para Vincula Latam. Este MVP establece la arquitectura base con un endpoint `POST /products/search` que consulta productos por IDs y companyId desde MySQL.

---

## 2. Stack Tecnológico

| Componente | Librería | Versión |
|-----------|----------|---------|
| Router HTTP | `go-chi/chi/v5` | v5.2.5 |
| MySQL Driver | `go-sql-driver/mysql` | v1.9.3 |
| Config | `spf13/viper` | v1.21.0 |
| Logging | `uber-go/zap` | v1.27.1 |
| UUID | `google/uuid` | v1.6.0 |

---

## 3. Estructura de Carpetas

```
saruman/
├── cmd/
│   └── server/
│       └── main.go                  # Bootstrap: config → DB → DI → server
├── internal/
│   ├── config/
│   │   └── config.go                # Viper: carga config desde env vars
│   ├── platform/
│   │   ├── logger/
│   │   │   └── logger.go            # Factory de Zap logger (JSON producción)
│   │   └── mysql/
│   │       └── connection.go        # Pool de conexiones MySQL
│   ├── errors/
│   │   └── errors.go                # ValidationError, InternalError
│   ├── domain/
│   │   └── product.go               # Entidad Product + AvailableStock()
│   ├── product/
│   │   ├── ports.go                 # Interfaces: SearchUseCase, Service, Repository
│   │   ├── dto.go                   # SearchProductsRequest/Response, ProductDTO
│   │   ├── repository.go            # MySQL: FindByIDsAndCompany (IN clause dinámica)
│   │   ├── service.go               # Lógica: encontrados vs no encontrados
│   │   ├── usecase.go               # Orquestación: service → ProductDTO mapping
│   │   ├── controller.go            # HTTP handler: parse, validate, respond
│   │   └── wire.go                  # DI local: repo → service → usecase → controller
│   └── server/
│       ├── router.go                # Chi router + middleware + rutas
│       └── server.go                # HTTP server lifecycle (start, graceful shutdown)
├── docs/
│   └── architecture.md              # Este archivo
├── specs/
│   └── mvp-product-search.md        # Spec del MVP
├── .env.example
├── go.mod
└── Makefile
```

---

## 4. Arquitectura por Capas (Hexagonal, 4 capas + DI)

```
HTTP Request
    │
    ▼
┌─────────────────────────────────────────┐
│  Controller (internal/product/controller)│  Parseo JSON, validación de schema,
│                                         │  HTTP status codes, serialización
└────────────────┬────────────────────────┘
                 │  SearchProductsRequest
                 ▼
┌─────────────────────────────────────────┐
│  UseCase (internal/product/usecase)     │  Orquestación del flujo,
│                                         │  mapeo domain → DTO
└────────────────┬────────────────────────┘
                 │  (ids []int, companyID int)
                 ▼
┌─────────────────────────────────────────┐
│  Service (internal/product/service)     │  Lógica de dominio: comparar IDs
│                                         │  pedidos vs encontrados → notFoundIDs
└────────────────┬────────────────────────┘
                 │  (ids []int, companyID int)
                 ▼
┌─────────────────────────────────────────┐
│  Repository (internal/product/repository)│  Query SQL pura, mapeo rows → domain
└────────────────┬────────────────────────┘
                 │
                 ▼
            MySQL (Product table)
```

Cada capa depende solo de **interfaces** (definidas en `ports.go`), nunca de implementaciones concretas.

### Inyección de Dependencias

DI manual sin framework. `wire.go` por módulo construye el grafo local:

```go
// internal/product/wire.go
func NewModule(db *sql.DB, logger *zap.Logger) *Controller {
    repo := NewMySQLRepository(db)      // Repository
    svc  := NewService(repo)            // Service
    uc   := NewSearchUseCase(svc)       // UseCase
    return NewController(uc, logger)    // Controller
}
```

`main.go` conecta todo: config → logger → DB → product.NewModule → router → server.

---

## 5. Endpoints

### POST /products/search

Busca productos por IDs dentro de una compañía.

**Request:**
```json
{
  "companyId": 12,
  "productIds": [101, 202, 305]
}
```

**Response 200:**
```json
{
  "products": [
    {
      "id": 101,
      "externalId": 1001,
      "name": "Producto A",
      "price": 10.50,
      "stock": 100,
      "reservedStock": 15,
      "availableStock": 85,
      "category": "general",
      "isActive": true,
      "hasStock": true,
      "stockeable": true
    }
  ],
  "notFound": [305]
}
```

**Response 400 (validación):**
```json
{
  "error": "VALIDATION_ERROR",
  "message": "companyId is required",
  "details": [
    { "field": "companyId", "message": "companyId is required" }
  ]
}
```

**Validaciones:**

| Campo | Regla |
|-------|-------|
| `companyId` | Requerido, entero > 0 |
| `productIds` | Requerido, no vacío, max 100 elementos |
| `productIds[i]` | Cada elemento entero > 0 |

### GET /health

Liveness check.

**Response 200:**
```json
{"status":"ok"}
```

---

## 6. Middleware

Tres middleware aplicados en orden en el router Chi:

1. **Recoverer** (`chi/middleware.Recoverer`): Captura panics, retorna 500 en lugar de crash.
2. **Request ID** (custom): Lee `X-Request-ID` del header; si no existe, genera UUID v4. Inyecta en response header.
3. **Logging** (custom con Zap): Registra method, path, status, duration y requestId en JSON al completar cada request.

---

## 7. Configuración

Variables de entorno cargadas con Viper (`internal/config/config.go`):

| Variable | Default | Descripción |
|----------|---------|-------------|
| `SERVER_PORT` | `8080` | Puerto HTTP |
| `DB_HOST` | `localhost` | Host MySQL |
| `DB_PORT` | `3306` | Puerto MySQL |
| `DB_USER` | `saruman` | Usuario MySQL |
| `DB_PASSWORD` | `secret` | Password MySQL |
| `DB_NAME` | `vincula` | Base de datos |
| `DB_MAX_OPEN_CONNS` | `25` | Pool: conexiones abiertas máximas |
| `DB_MAX_IDLE_CONNS` | `5` | Pool: conexiones idle máximas |
| `DB_CONN_MAX_LIFETIME` | `5m` | Pool: lifetime máximo por conexión |
| `LOG_LEVEL` | `info` | Nivel de log (debug/info/warn/error) |

---

## 8. Query SQL

```sql
SELECT id, external_id, name, description, price, stock, reserved_stock,
       companyId, typeId, category, isActive, isDeleted, hasStock, Stockeable,
       createdAt, updatedAt
FROM Product
WHERE id IN (?, ?, ?)
  AND companyId = ?
  AND isDeleted = 0;
```

- `IN` clause construida dinámicamente según cantidad de IDs.
- Filtro `isDeleted = 0` excluye productos borrados lógicamente.
- Nombres de columnas respetan esquema existente (camelCase mixto).
- `stock` y `reserved_stock` son nullable → se mapean a `*int` en Go.

---

## 9. Domain Model

```go
type Product struct {
    ID, ExternalID, CompanyID, TypeID int
    Name, Description, Category       string
    Price                              float64
    Stock, ReservedStock               *int      // nullable
    IsActive, IsDeleted, HasStock, Stockeable bool
    CreatedAt, UpdatedAt               time.Time
}
```

**`AvailableStock()`**: Calcula `stock - reserved_stock`. Retorna 0 si alguno es nil o el resultado es negativo.

---

## 10. Flujo Interno

```
1. Controller: parsea JSON → SearchProductsRequest
2. Controller: valida companyId > 0, productIds no vacío, cada id > 0
3. Controller → UseCase.SearchProducts(ctx, req)
4. UseCase → Service.GetProductsByIDsAndCompany(ctx, ids, companyID)
5. Service → Repository.FindByIDsAndCompany(ctx, ids, companyID)
6. Repository: ejecuta SQL, mapea rows → []domain.Product
7. Service: compara IDs pedidos vs encontrados → (found, notFoundIDs)
8. UseCase: mapea domain.Product → ProductDTO (con AvailableStock())
9. Controller: serializa SearchProductsResponse → JSON 200
```

---

## 11. Server Lifecycle

- **Start**: Escucha en `:PORT` con timeouts (read: 10s, write: 10s, idle: 30s).
- **Graceful Shutdown**: Escucha SIGINT/SIGTERM. Al recibir señal, ejecuta `server.Shutdown()` con timeout de 10s para drenar conexiones activas.

---

## 12. Desviaciones del Spec Original

| Cambio | Justificación |
|--------|--------------|
| `internal/errors/errors.go` agregado | El spec no define error types; necesarios para manejo limpio entre capas |
| `internal/platform/logger/logger.go` agregado | Factory para Zap logger (spec asume Zap pero no define inicialización) |
| `GET /health` endpoint agregado | Liveness check básico para verificar que el servidor está vivo |
| Middleware request-id, logging, recovery | Esenciales para un MVP funcional en producción |
| Validación en Controller (no UseCase) | La validación de schema HTTP pertenece al Controller según responsabilidades de capa |

---

## 13. Pendiente (fuera del MVP)

- [ ] Autenticación / autorización (Bearer token, API Key)
- [ ] Métricas Prometheus
- [ ] Trazas OpenTelemetry
- [ ] Tests unitarios e integración
- [ ] Dockerfile (multistage build)
- [ ] Módulo `reserve-and-add` (transacciones, SELECT FOR UPDATE)
- [ ] Módulos `confirm` y `cancel`
- [ ] Rate limiting
