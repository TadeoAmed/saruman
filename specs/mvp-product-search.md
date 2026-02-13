# Spec: MVP - Product Search

**Estado:** Draft
**Fecha:** 2026-02-13
**Módulo:** product-search (MVP)

---

## 1. Objetivo

MVP para establecer la **arquitectura base** del proyecto Saruman. Este módulo define los patrones, dependencias, estructura de carpetas y configuración que se reutilizarán en los módulos posteriores (`reserve-and-add`, `confirm`, `cancel`).

**Alcance:**
- Endpoint `POST /products/search` que consulta productos por IDs y companyId
- Validar stack tecnológico (Chi, Zap, Viper, MySQL driver)
- Establecer arquitectura hexagonal de 4 capas + DI
- Configuración desde variables de entorno
- Logs estructurados en JSON

**No incluido en este MVP:**
- Autenticación / autorización
- Métricas Prometheus / OpenTelemetry
- Transacciones / SELECT FOR UPDATE
- Modificación de stock

---

## 2. Arquitectura - Hexagonal (4 capas + DI)

```
main.go (bootstrap)
    │
    ▼
DI Container / Wire
    │
    ├─► Controller (HTTP) ── interfaz puerto entrada
    │       │
    │       ▼
    │   Use Case ── orquestación del flujo
    │       │
    │       ▼
    │   Service ── lógica de dominio (validaciones, transformaciones)
    │       │
    │       ▼
    └─► Repository (MySQL) ── interfaz puerto salida
```

### Responsabilidades por capa

| Capa | Responsabilidad |
|------|----------------|
| **Controller** | Parseo de request, validación de schema JSON, serialización de response, HTTP status codes |
| **Use Case** | Orquestación del flujo: llamar service, manejar errores, componer respuesta final |
| **Service** | Lógica de dominio: validar que productos existen, pertenecen a la company, están activos. Calcular campos derivados (availableStock). Separar encontrados de no encontrados |
| **Repository** | Queries SQL puras, mapeo de filas a entidades de dominio. Sin lógica de negocio |
| **DI/Wire** | Resolver dependencias: repository → service → use case → controller → router → main |

### Principio clave

Cada capa depende **solo de interfaces** (puertos), nunca de implementaciones concretas. Las dependencias se inyectan desde `main.go` hacia adentro.

---

## 3. Endpoint

### POST /products/search

**Request:**

```http
POST /products/search
Content-Type: application/json
```

```json
{
  "companyId": 12,
  "productIds": [101, 202, 305]
}
```

### Response 200 OK

```json
{
  "products": [
    {
      "id": 101,
      "externalId": 1001,
      "name": "Producto A",
      "description": "Descripción del producto",
      "price": 10.50,
      "stock": 100,
      "reservedStock": 15,
      "availableStock": 85,
      "category": "general",
      "isActive": true,
      "hasStock": true,
      "stockeable": true
    },
    {
      "id": 202,
      "externalId": 2002,
      "name": "Producto B",
      "description": "Otro producto",
      "price": 25.00,
      "stock": null,
      "reservedStock": null,
      "availableStock": 0,
      "category": "general",
      "isActive": true,
      "hasStock": false,
      "stockeable": false
    }
  ],
  "notFound": [305]
}
```

### Response 400 Bad Request

```json
{
  "error": "VALIDATION_ERROR",
  "message": "companyId is required",
  "details": [
    {
      "field": "companyId",
      "message": "companyId is required"
    }
  ]
}
```

### Validaciones de input

| Campo | Regla | Error |
|-------|-------|-------|
| `companyId` | Entero > 0, requerido | 400 `companyId is required` / `companyId must be a positive integer` |
| `productIds` | Array no vacío, requerido, max 100 elementos | 400 `productIds is required` / `productIds must not be empty` / `productIds exceeds maximum of 100` |
| `productIds[i]` | Entero > 0 | 400 `each productId must be a positive integer` |

---

## 4. Domain Model

```go
// internal/domain/product.go
package domain

import "time"

// Product representa un producto en el catálogo de una compañía.
type Product struct {
    ID            int
    ExternalID    int
    Name          string
    Description   string
    Price         float64  // decimal(10,2) en BD
    Stock         *int     // nullable en BD
    ReservedStock *int     // nullable en BD
    CompanyID     int
    TypeID        int
    Category      string
    IsActive      bool
    IsDeleted     bool
    HasStock      bool
    Stockeable    bool
    CreatedAt     time.Time
    UpdatedAt     time.Time
}

// AvailableStock calcula el stock disponible (stock - reserved_stock).
// Retorna 0 si stock o reserved_stock son nil.
func (p Product) AvailableStock() int {
    if p.Stock == nil || p.ReservedStock == nil {
        return 0
    }
    available := *p.Stock - *p.ReservedStock
    if available < 0 {
        return 0
    }
    return available
}
```

**Notas sobre mapeo BD → Domain:**
- `price` (decimal 10,2) → `float64` (suficiente para lectura; en módulos de escritura futura se evaluará `shopspring/decimal`)
- `stock` y `reserved_stock` son nullable → punteros `*int`
- `isActive`, `isDeleted`, `hasStock`, `Stockeable` son `tinyint(1)` → `bool`

---

## 5. Interfaces (Puertos)

```go
// internal/product/ports.go
package product

import (
    "context"
    "saruman/internal/domain"
)

// --- Puerto de entrada (driven by controller) ---

// SearchUseCase define la operación de búsqueda de productos.
type SearchUseCase interface {
    SearchProducts(ctx context.Context, req SearchProductsRequest) (*SearchProductsResponse, error)
}

// --- Puerto de dominio ---

// Service define la lógica de dominio para productos.
type Service interface {
    GetProductsByIDsAndCompany(ctx context.Context, ids []int, companyID int) (found []domain.Product, notFoundIDs []int, err error)
}

// --- Puerto de salida (drives repository) ---

// Repository define el acceso a datos de productos.
type Repository interface {
    FindByIDsAndCompany(ctx context.Context, ids []int, companyID int) ([]domain.Product, error)
}
```

---

## 6. DTOs

```go
// internal/product/dto.go
package product

// SearchProductsRequest es el body del POST /products/search.
type SearchProductsRequest struct {
    CompanyID  int   `json:"companyId"`
    ProductIDs []int `json:"productIds"`
}

// SearchProductsResponse es la respuesta del endpoint.
type SearchProductsResponse struct {
    Products []ProductDTO `json:"products"`
    NotFound []int        `json:"notFound"`
}

// ProductDTO representa un producto en la respuesta JSON.
type ProductDTO struct {
    ID             int     `json:"id"`
    ExternalID     int     `json:"externalId"`
    Name           string  `json:"name"`
    Description    string  `json:"description"`
    Price          float64 `json:"price"`
    Stock          *int    `json:"stock"`
    ReservedStock  *int    `json:"reservedStock"`
    AvailableStock int     `json:"availableStock"`
    Category       string  `json:"category"`
    IsActive       bool    `json:"isActive"`
    HasStock       bool    `json:"hasStock"`
    Stockeable     bool    `json:"stockeable"`
}
```

---

## 7. Estructura de Carpetas

```
saruman/
├── cmd/
│   └── server/
│       └── main.go                  # Bootstrap: config, DB, DI, server start
├── internal/
│   ├── config/
│   │   └── config.go                # Viper: carga config desde env/files
│   ├── platform/
│   │   └── mysql/
│   │       └── connection.go        # DB connection pool setup
│   ├── domain/
│   │   └── product.go               # Product entity + AvailableStock()
│   ├── product/
│   │   ├── controller.go            # HTTP handler: parseo, validación, response
│   │   ├── usecase.go               # Orquestación: service → response
│   │   ├── service.go               # Lógica dominio: encontrados vs no encontrados
│   │   ├── repository.go            # MySQL queries: FindByIDsAndCompany
│   │   ├── dto.go                   # Request/Response DTOs
│   │   ├── ports.go                 # Interfaces (puertos de entrada/salida)
│   │   └── wire.go                  # DI wiring: construye el módulo completo
│   └── server/
│       ├── router.go                # Chi router setup + rutas
│       └── server.go                # HTTP server lifecycle (start, graceful shutdown)
├── .env.example
├── go.mod
└── Makefile
```

### Convenciones

- **Un paquete por módulo de dominio** (`product/`). Módulos futuros (`order/`, `stock/`) seguirán el mismo patrón.
- **`internal/`** para código no exportable.
- **`platform/`** para adaptadores de infraestructura (MySQL, Redis futuro, etc.).
- **`domain/`** para entidades compartidas entre módulos.
- **`wire.go`** por módulo resuelve DI local; `main.go` conecta todo.

---

## 8. Stack Tecnológico MVP

| Componente | Librería | Justificación |
|-----------|----------|---------------|
| Router HTTP | `go-chi/chi/v5` | Ligero, idiomático, compatible con `net/http`, middleware composable |
| MySQL Driver | `go-sql-driver/mysql` | Driver estándar de facto para MySQL en Go |
| Config | `spf13/viper` | Carga config desde env vars y archivos, ampliamente adoptado |
| Logging | `uber-go/zap` | Structured JSON logging de alto rendimiento |
| UUID | `google/uuid` | Para traceIds futuros y generación de identificadores |

---

## 9. Configuración

### .env.example

```env
# Server
SERVER_PORT=8080

# Database
DB_HOST=localhost
DB_PORT=3306
DB_USER=saruman
DB_PASSWORD=secret
DB_NAME=vincula
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5
DB_CONN_MAX_LIFETIME=5m

# Logging
LOG_LEVEL=info
```

### Config struct

```go
// internal/config/config.go
type Config struct {
    Server   ServerConfig
    Database DatabaseConfig
    Log      LogConfig
}

type ServerConfig struct {
    Port int // default: 8080
}

type DatabaseConfig struct {
    Host            string
    Port            int
    User            string
    Password        string
    Name            string
    MaxOpenConns    int           // default: 25
    MaxIdleConns    int           // default: 5
    ConnMaxLifetime time.Duration // default: 5m
}

type LogConfig struct {
    Level string // default: "info"
}
```

---

## 10. Query SQL

```sql
SELECT id, external_id, name, description, price, stock, reserved_stock,
       companyId, typeId, category, isActive, isDeleted, hasStock, Stockeable,
       createdAt, updatedAt
FROM Product
WHERE id IN (?, ?, ?)
  AND companyId = ?
  AND isDeleted = 0;
```

**Notas:**
- `IN (?, ?, ?)` se construye dinámicamente según la cantidad de IDs recibidos.
- Filtro `isDeleted = 0` para excluir productos borrados lógicamente.
- Los nombres de columnas respetan el esquema existente (camelCase mixto: `companyId`, `typeId`, `isActive`, `Stockeable`).
- No se filtra por `isActive` en la query; el service reporta el estado en la respuesta para que el caller decida.

---

## 11. Flujo interno del endpoint

```
1. Controller recibe POST /products/search
   ├─ Parsea JSON body → SearchProductsRequest
   ├─ Valida: companyId > 0, productIds no vacío, cada id > 0
   └─ Si inválido → 400 Bad Request

2. Controller llama UseCase.SearchProducts(ctx, req)

3. UseCase llama Service.GetProductsByIDsAndCompany(ctx, ids, companyId)

4. Service llama Repository.FindByIDsAndCompany(ctx, ids, companyId)

5. Repository ejecuta query SQL
   ├─ Construye IN clause dinámica
   ├─ Ejecuta query
   └─ Mapea rows → []domain.Product

6. Service recibe []domain.Product
   ├─ Compara IDs recibidos vs IDs encontrados
   ├─ Calcula notFoundIDs (los que no vinieron de la BD)
   └─ Retorna (found, notFoundIDs, nil)

7. UseCase recibe (found, notFoundIDs, nil)
   ├─ Mapea domain.Product → ProductDTO (incluyendo AvailableStock())
   └─ Retorna SearchProductsResponse{Products, NotFound}

8. Controller serializa response → JSON 200 OK
```

---

## 12. Criterios de Aceptación

- [ ] `go run cmd/server/main.go` levanta el servidor en el puerto configurado
- [ ] `POST /products/search` con body válido retorna los productos encontrados
- [ ] Productos no encontrados se retornan en `notFound[]`
- [ ] `availableStock` se calcula correctamente como `stock - reserved_stock`
- [ ] Productos con `stock` o `reserved_stock` null retornan `availableStock: 0`
- [ ] Validación de input retorna 400 con mensaje descriptivo y campo `details`
- [ ] Logs estructurados en JSON al stdout (Zap)
- [ ] Cada capa depende solo de interfaces, no de implementaciones concretas
- [ ] DI resuelve todas las dependencias en `main.go` (sin contenedor, wiring manual)
- [ ] Graceful shutdown del servidor HTTP
- [ ] Connection pool de MySQL configurado desde variables de entorno

---

## 13. Referencia al Esquema de BD

Este MVP lee de la tabla `Product` documentada en `PROJECT_CONTEXT.md` sección 5.1. Campos relevantes:

| Columna BD | Campo Domain | Tipo BD | Tipo Go |
|-----------|-------------|---------|---------|
| `id` | ID | `int NOT NULL AUTO_INCREMENT` | `int` |
| `external_id` | ExternalID | `int NOT NULL` | `int` |
| `name` | Name | `varchar(255) NOT NULL` | `string` |
| `description` | Description | `text NOT NULL` | `string` |
| `price` | Price | `decimal(10,2) NOT NULL` | `float64` |
| `stock` | Stock | `int DEFAULT NULL` | `*int` |
| `reserved_stock` | ReservedStock | `int DEFAULT NULL` | `*int` |
| `companyId` | CompanyID | `int NOT NULL` | `int` |
| `typeId` | TypeID | `int NOT NULL` | `int` |
| `category` | Category | `varchar(100) NOT NULL DEFAULT 'general'` | `string` |
| `isActive` | IsActive | `tinyint(1) DEFAULT '1'` | `bool` |
| `isDeleted` | IsDeleted | `tinyint(1) DEFAULT '0'` | `bool` |
| `hasStock` | HasStock | `tinyint(1) DEFAULT '0'` | `bool` |
| `Stockeable` | Stockeable | `tinyint(1) NOT NULL DEFAULT '1'` | `bool` |
| `createdAt` | CreatedAt | `datetime DEFAULT CURRENT_TIMESTAMP` | `time.Time` |
| `updatedAt` | UpdatedAt | `datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE` | `time.Time` |
