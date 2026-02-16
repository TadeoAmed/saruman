# Spec 1: Order Domain & Data Access

**Estado:** Draft
**Fecha:** 2026-02-15
**Módulo:** order (domain + repository)
**Depende de:** MVP product-search (arquitectura base establecida)

---

## 1. Objetivo

Definir las entidades de dominio y repositorios de lectura necesarios para soportar el flujo de reserve-and-add. Este spec cubre las **estructuras de datos y acceso a BD**, sin lógica de negocio ni endpoint HTTP.

**Alcance:**
- Entidad `Order` (domain)
- Entidad `OrderItem` (domain)
- Entidad `CompanyConfig` (domain)
- Repository para `Order` (lectura + update status)
- Repository para `OrderItem` (insert)
- Repository para `CompanyConfig` (lectura)
- Repository para `Product` (lectura con lock + update reserved_stock)

**No incluido en este spec:**
- Lógica de orquestación (spec-2)
- Endpoint HTTP ni DTOs de request/response (spec-3)
- Autenticación / autorización

---

## 2. Domain Entities

### 2.1 Order

Mapea la tabla `Orders` del schema existente.

```go
// internal/domain/order.go
package domain

import "time"

type Order struct {
    ID         uint
    CompanyID  int
    FirstName  string
    LastName   string
    Email      string
    Phone      *string
    Address    *string
    Status     string    // PENDING, CREATED, CANCELED
    TotalPrice float64   // decimal(10,2)
    CreatedAt  time.Time
    UpdatedAt  time.Time
}
```

**Mapeo BD -> Domain:**

| Columna BD | Campo Go | Tipo BD | Tipo Go |
|-----------|----------|---------|---------|
| `id` | ID | `int unsigned NOT NULL AUTO_INCREMENT` | `uint` |
| `companyId` | CompanyID | `int NOT NULL DEFAULT '1'` | `int` |
| `firstName` | FirstName | `varchar(100) NOT NULL` | `string` |
| `lastName` | LastName | `varchar(100) NOT NULL` | `string` |
| `email` | Email | `varchar(150) NOT NULL` | `string` |
| `phone` | Phone | `varchar(30) DEFAULT NULL` | `*string` |
| `address` | Address | `varchar(255) DEFAULT NULL` | `*string` |
| `status` | Status | `varchar(50) DEFAULT 'pending'` | `string` |
| `totalPrice` | TotalPrice | `decimal(10,2) DEFAULT '0.00'` | `float64` |
| `createdAt` | CreatedAt | `datetime DEFAULT CURRENT_TIMESTAMP` | `time.Time` |
| `updatedAt` | UpdatedAt | `datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE` | `time.Time` |

**Constantes de estado:**

```go
const (
    OrderStatusPending  = "PENDING"
    OrderStatusCreated  = "CREATED"
    OrderStatusCanceled = "CANCELED"
)
```

### 2.2 OrderItem

Mapea la tabla `OrderItems` del schema existente.

```go
// internal/domain/order_item.go
package domain

type OrderItem struct {
    ID        uint
    OrderID   uint
    ProductID int
    Quantity  int
    Price     float64 // decimal(10,2)
}
```

**Mapeo BD -> Domain:**

| Columna BD | Campo Go | Tipo BD | Tipo Go |
|-----------|----------|---------|---------|
| `id` | ID | `int unsigned NOT NULL AUTO_INCREMENT` | `uint` |
| `orderId` | OrderID | `int unsigned NOT NULL` | `uint` |
| `productId` | ProductID | `int NOT NULL` | `int` |
| `quantity` | Quantity | `int DEFAULT '1'` | `int` |
| `price` | Price | `decimal(10,2) NOT NULL` | `float64` |

### 2.3 CompanyConfig

Mapea la tabla `CompanyConfig` del schema existente.

```go
// internal/domain/company_config.go
package domain

import "time"

type CompanyConfig struct {
    ID                int
    CompanyID         int
    FieldsOrderConfig string // JSON raw string
    HasStock          bool
    CreatedAt         time.Time
    UpdatedAt         time.Time
}
```

**Mapeo BD -> Domain:**

| Columna BD | Campo Go | Tipo BD | Tipo Go |
|-----------|----------|---------|---------|
| `id` | ID | `int NOT NULL AUTO_INCREMENT` | `int` |
| `companyId` | CompanyID | `int NOT NULL` | `int` |
| `fieldsOrderConfig` | FieldsOrderConfig | `json NOT NULL` | `string` |
| `hasStock` | HasStock | `tinyint(1) NOT NULL DEFAULT '0'` | `bool` |
| `createdAt` | CreatedAt | `datetime NOT NULL DEFAULT CURRENT_TIMESTAMP` | `time.Time` |
| `updatedAt` | UpdatedAt | `datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE` | `time.Time` |

**Campo crítico:** `HasStock` determina si la company tiene habilitado control de stock. Si `false`, el flujo de reserva no valida disponibilidad de stock.

---

## 3. Repository Interfaces

Las interfaces se definen en el paquete que las **consume** (spec-2 service), no en el que las implementa. Aquí se documentan las operaciones que cada repository debe soportar.

### 3.1 OrderRepository

```go
// Operaciones requeridas por el service (spec-2)
type OrderRepository interface {
    FindByID(ctx context.Context, id uint) (*domain.Order, error)
    UpdateStatus(ctx context.Context, tx *sql.Tx, id uint, status string) error
    UpdateTotalPrice(ctx context.Context, tx *sql.Tx, id uint, totalPrice float64) error
}
```

### 3.2 OrderItemRepository

```go
type OrderItemRepository interface {
    Insert(ctx context.Context, tx *sql.Tx, item domain.OrderItem) (uint, error)
}
```

### 3.3 CompanyConfigRepository

```go
type CompanyConfigRepository interface {
    FindByCompanyID(ctx context.Context, companyID int) (*domain.CompanyConfig, error)
}
```

### 3.4 ProductRepository (extensión)

El product repository del MVP se extiende con operaciones transaccionales:

```go
type ProductRepository interface {
    // Existente del MVP
    FindByIDsAndCompany(ctx context.Context, ids []int, companyID int) ([]domain.Product, error)

    // Nuevas para reserve-and-add
    FindByIDForUpdate(ctx context.Context, tx *sql.Tx, productID int, companyID int) (*domain.Product, error)
    IncrementReservedStock(ctx context.Context, tx *sql.Tx, productID int, quantity int) error
}
```

---

## 4. SQL Queries

### 4.1 Order - FindByID

```sql
SELECT id, companyId, firstName, lastName, email, phone, address,
       status, totalPrice, createdAt, updatedAt
FROM Orders
WHERE id = ?;
```

**Notas:**
- No filtra por `isDeleted` porque la tabla `Orders` no tiene esa columna.
- Retorna error si no existe (not found).

### 4.2 Order - UpdateStatus

```sql
UPDATE Orders
SET status = ?
WHERE id = ?;
```

### 4.3 Order - UpdateTotalPrice

```sql
UPDATE Orders
SET totalPrice = ?
WHERE id = ?;
```

### 4.4 OrderItem - Insert

```sql
INSERT INTO OrderItems (orderId, productId, quantity, price)
VALUES (?, ?, ?, ?);
```

**Notas:**
- Retorna `LastInsertId` para asignar al domain entity.
- La tabla `OrderItems` tiene FK constraint a `Orders(id)` con `ON DELETE CASCADE`.

### 4.5 CompanyConfig - FindByCompanyID

```sql
SELECT id, companyId, fieldsOrderConfig, hasStock, createdAt, updatedAt
FROM CompanyConfig
WHERE companyId = ?;
```

**Notas:**
- `companyId` tiene UNIQUE constraint, así que retorna 0 o 1 fila.
- Si no existe config para la company, retornar error `NotFound`.

### 4.6 Product - FindByIDForUpdate

```sql
SELECT id, external_id, name, description, price, stock, reserved_stock,
       companyId, typeId, category, isActive, isDeleted, hasStock, Stockeable,
       createdAt, updatedAt
FROM Product
WHERE id = ?
  AND companyId = ?
  AND isDeleted = 0
FOR UPDATE;
```

**Notas:**
- `FOR UPDATE` adquiere row-level lock dentro de la transacción.
- Filtra `isDeleted = 0` para excluir borrados lógicos.
- NO filtra por `isActive` en la query; la validación de estado activo la hace el service.

### 4.7 Product - IncrementReservedStock

```sql
UPDATE Product
SET reserved_stock = COALESCE(reserved_stock, 0) + ?
WHERE id = ?;
```

**Notas:**
- `COALESCE` maneja el caso de `reserved_stock` null (lo trata como 0).
- Este UPDATE solo se ejecuta después de validar disponibilidad en el service.

---

## 5. Consideraciones sobre Transacciones

- `FindByID` (Order) y `FindByCompanyID` (CompanyConfig) se ejecutan **fuera** de la transacción principal. Son lecturas de validación previas.
- `FindByIDForUpdate`, `IncrementReservedStock`, `Insert` (OrderItem), `UpdateStatus`, y `UpdateTotalPrice` se ejecutan **dentro** de la transacción (`*sql.Tx` como parámetro).
- El service (spec-2) es responsable de abrir y cerrar la transacción.

---

## 6. Estructura de Carpetas

```
internal/
├── domain/
│   ├── product.go           # Ya existe (MVP)
│   ├── order.go             # NUEVO
│   ├── order_item.go        # NUEVO
│   └── company_config.go    # NUEVO
├── order/
│   ├── repository/
│   │   ├── order_repository.go        # NUEVO
│   │   └── order_item_repository.go   # NUEVO
│   └── ...                            # service, usecase, controller en specs 2-3
├── company/
│   └── repository/
│       └── company_config_repository.go  # NUEVO
└── product/
    └── repository/
        └── product_repository.go      # EXTENDER con ForUpdate + IncrementReservedStock
```

---

## 7. Criterios de Aceptación

- [ ] Entidades `Order`, `OrderItem`, `CompanyConfig` definidas en `internal/domain/`
- [ ] Cada entidad mapea correctamente las columnas de su tabla BD (tipos Go, nullable → punteros)
- [ ] Constantes de status de Order definidas (`PENDING`, `CREATED`, `CANCELED`)
- [ ] OrderRepository implementa `FindByID` y `UpdateStatus` y `UpdateTotalPrice`
- [ ] OrderItemRepository implementa `Insert` con retorno de ID generado
- [ ] CompanyConfigRepository implementa `FindByCompanyID`
- [ ] ProductRepository extendido con `FindByIDForUpdate` (con `FOR UPDATE`) y `IncrementReservedStock`
- [ ] Queries usan parameterized placeholders (`?`) - nunca string interpolation
- [ ] Métodos transaccionales aceptan `*sql.Tx` como parámetro
- [ ] Métodos de solo lectura (pre-transacción) usan `*sql.DB` directamente
- [ ] Nombres de columnas en SQL respetan el schema existente (camelCase mixto)
