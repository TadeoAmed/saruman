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
| YAML Config | `go.yaml.in/yaml/v3` | v3.0.4 |
| Logging | `uber-go/zap` | v1.27.1 |
| UUID | `google/uuid` | v1.6.0 |

---

## 3. Estructura de Carpetas

```
saruman/
├── .vscode/
│   └── launch.json                  # Config de debug para VS Code
├── cmd/
│   └── server/
│       └── main.go                  # Bootstrap: config → DB → DI → server
├── internal/
│   ├── commons/
│   │   └── config.go                # LoadConfig(): lee YAML y parsea a Config struct
│   ├── config/
│   │   ├── config.go                # Structs: Config, ServerConfig, DatabaseConfig, LogConfig
│   │   └── config.yaml              # Archivo YAML con valores de configuración
│   ├── infrastructure/
│   │   ├── logger/
│   │   │   └── logger.go            # Factory de Zap logger (JSON producción)
│   │   └── mysql/
│   │       └── connection.go        # Pool de conexiones MySQL
│   ├── errors/
│   │   └── errors.go                # ValidationError, InternalError
│   ├── domain/
│   │   └── product.go               # Entidad Product + AvailableStock()
│   ├── dto/
│   │   └── dto.go                   # SearchProductsRequest/Response, ProductDTO
│   ├── product/
│   │   ├── controller/
│   │   │   └── get_products_controller.go  # HTTP handler: parse, validate, respond
│   │   ├── repository/
│   │   │   └── products_repository.go      # MySQL: FindByIDsAndCompany (IN clause dinámica)
│   │   ├── service/
│   │   │   └── products_service.go         # Lógica: encontrados vs no encontrados
│   │   ├── usecase/
│   │   │   └── search_products_use_case.go # Orquestación: service → ProductDTO mapping
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
├── Makefile
└── README.md
```

---

## 4. Arquitectura por Capas (Hexagonal, 4 capas + DI)

```
HTTP Request
    │
    ▼
┌──────────────────────────────────────────────────┐
│  Controller (internal/product/controller/)       │  Parseo JSON, validación de schema,
│                                                  │  HTTP status codes, serialización
└────────────────┬─────────────────────────────────┘
                 │  dto.SearchProductsRequest
                 ▼
┌──────────────────────────────────────────────────┐
│  UseCase (internal/product/usecase/)             │  Orquestación del flujo,
│                                                  │  mapeo domain → DTO
└────────────────┬─────────────────────────────────┘
                 │  (ids []int, companyID int)
                 ▼
┌──────────────────────────────────────────────────┐
│  Service (internal/product/service/)             │  Lógica de dominio: comparar IDs
│                                                  │  pedidos vs encontrados → notFoundIDs
└────────────────┬─────────────────────────────────┘
                 │  (ids []int, companyID int)
                 ▼
┌──────────────────────────────────────────────────┐
│  Repository (internal/product/repository/)       │  Query SQL pura, mapeo rows → domain
└────────────────┬─────────────────────────────────┘
                 │
                 ▼
            MySQL (Product table)
```

Cada capa depende solo de **interfaces**, nunca de implementaciones concretas. Los DTOs están centralizados en `internal/dto/`.

### Inyección de Dependencias

DI manual sin framework. `wire.go` por módulo construye el grafo local:

```go
// internal/product/wire.go
func NewModule(db *sql.DB, logger *zap.Logger) *controller.Controller {
    repo := repository.NewMySQLRepository(db)      // Repository
    svc  := service.NewService(repo)               // Service
    uc   := usecase.NewSearchUseCase(svc)           // UseCase
    return controller.NewController(uc, logger)    // Controller
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

La configuración se carga desde `internal/config/config.yaml` usando `commons.LoadConfig()` (`internal/commons/config.go`), que lee el archivo YAML y lo deserializa en structs definidas en `internal/config/config.go`.

**Archivo `internal/config/config.yaml`:**
```yaml
server:
  port: 8080
database:
  host: localhost
  port: 3306
  user: saruman
  password: secret
  name: vincula
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: 5m
log:
  level: info
```

**Structs de configuración (`internal/config/config.go`):**
```go
type Config struct {
    Server   ServerConfig   `yaml:"server"`
    Database DatabaseConfig `yaml:"database"`
    Log      LogConfig      `yaml:"log"`
}
```

| Sección | Campo | Descripción |
|---------|-------|-------------|
| `server` | `port` | Puerto HTTP |
| `database` | `host` | Host MySQL |
| `database` | `port` | Puerto MySQL |
| `database` | `user` | Usuario MySQL |
| `database` | `password` | Password MySQL |
| `database` | `name` | Base de datos |
| `database` | `max_open_conns` | Pool: conexiones abiertas máximas |
| `database` | `max_idle_conns` | Pool: conexiones idle máximas |
| `database` | `conn_max_lifetime` | Pool: lifetime máximo por conexión |
| `log` | `level` | Nivel de log (debug/info/warn/error) |

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
| `internal/infrastructure/logger/logger.go` agregado | Factory para Zap logger (spec asume Zap pero no define inicialización) |
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
