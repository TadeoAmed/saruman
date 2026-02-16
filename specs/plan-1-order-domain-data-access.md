# Plan 1: Order Domain & Data Access

**Spec:** `spec-1-order-domain-data-access.md`
**Fecha:** 2026-02-15
**Estado:** Pendiente de aprobación

---

## Resumen

Implementar las entidades de dominio (`Order`, `OrderItem`, `CompanyConfig`) y los repositorios de lectura/escritura necesarios para el flujo reserve-and-add. También extender el `ProductRepository` existente con operaciones transaccionales (`FindByIDForUpdate`, `IncrementReservedStock`).

---

## Paso 1: Crear entidad `Order` en domain

**Archivo:** `internal/domain/order.go` (NUEVO)

**Qué hacer:**
- Definir struct `Order` con campos: `ID uint`, `CompanyID int`, `FirstName string`, `LastName string`, `Email string`, `Phone *string`, `Address *string`, `Status string`, `TotalPrice float64`, `CreatedAt time.Time`, `UpdatedAt time.Time`
- Definir constantes de status: `OrderStatusPending = "PENDING"`, `OrderStatusCreated = "CREATED"`, `OrderStatusCanceled = "CANCELED"`

**Patrón a seguir:** `internal/domain/product.go` — struct plano con tipos Go nativos, punteros para nullable.

---

## Paso 2: Crear entidad `OrderItem` en domain

**Archivo:** `internal/domain/order_item.go` (NUEVO)

**Qué hacer:**
- Definir struct `OrderItem` con campos: `ID uint`, `OrderID uint`, `ProductID int`, `Quantity int`, `Price float64`

---

## Paso 3: Crear entidad `CompanyConfig` en domain

**Archivo:** `internal/domain/company_config.go` (NUEVO)

**Qué hacer:**
- Definir struct `CompanyConfig` con campos: `ID int`, `CompanyID int`, `FieldsOrderConfig string`, `HasStock bool`, `CreatedAt time.Time`, `UpdatedAt time.Time`

---

## Paso 4: Agregar `NotFoundError` a errores custom

**Archivo:** `internal/errors/errors.go` (EDITAR)

**Qué hacer:**
- Agregar tipo `NotFoundError` con campo `Message string`
- Agregar constructor `NewNotFoundError(message string) *NotFoundError`
- Agregar helper `IsNotFoundError(err error) (*NotFoundError, bool)`

**Justificación:** El spec requiere retornar error `NotFound` cuando `CompanyConfig` o `Order` no existen. El módulo de errores existente no tiene este tipo. Sigue el patrón exacto de `ValidationError` / `InternalError`.

---

## Paso 5: Crear `OrderRepository`

**Archivo:** `internal/order/repository/order_repository.go` (NUEVO)

**Qué hacer:**
- Struct `MySQLOrderRepository` con campo `db *sql.DB`
- Constructor `NewMySQLOrderRepository(db *sql.DB) *MySQLOrderRepository`
- Método `FindByID(ctx context.Context, id uint) (*domain.Order, error)`:
  - Query: `SELECT id, companyId, firstName, lastName, email, phone, address, status, totalPrice, createdAt, updatedAt FROM Orders WHERE id = ?`
  - Usa `r.db.QueryRowContext` (single row)
  - Retorna `NotFoundError` si `sql.ErrNoRows`
- Método `UpdateStatus(ctx context.Context, tx *sql.Tx, id uint, status string) error`:
  - Query: `UPDATE Orders SET status = ? WHERE id = ?`
  - Usa `tx.ExecContext` (dentro de transacción)
- Método `UpdateTotalPrice(ctx context.Context, tx *sql.Tx, id uint, totalPrice float64) error`:
  - Query: `UPDATE Orders SET totalPrice = ? WHERE id = ?`
  - Usa `tx.ExecContext` (dentro de transacción)

**Patrón a seguir:** `internal/product/repository/products_repository.go` — constructor con `*sql.DB`, error wrapping con `%w`, parameterized queries.

---

## Paso 6: Crear `OrderItemRepository`

**Archivo:** `internal/order/repository/order_item_repository.go` (NUEVO)

**Qué hacer:**
- Struct `MySQLOrderItemRepository` con campo `db *sql.DB`
- Constructor `NewMySQLOrderItemRepository(db *sql.DB) *MySQLOrderItemRepository`
- Método `Insert(ctx context.Context, tx *sql.Tx, item domain.OrderItem) (uint, error)`:
  - Query: `INSERT INTO OrderItems (orderId, productId, quantity, price) VALUES (?, ?, ?, ?)`
  - Usa `tx.ExecContext` (dentro de transacción)
  - Retorna `LastInsertId()` casteado a `uint`

---

## Paso 7: Crear `CompanyConfigRepository`

**Archivo:** `internal/company/repository/company_config_repository.go` (NUEVO)

**Qué hacer:**
- Struct `MySQLCompanyConfigRepository` con campo `db *sql.DB`
- Constructor `NewMySQLCompanyConfigRepository(db *sql.DB) *MySQLCompanyConfigRepository`
- Método `FindByCompanyID(ctx context.Context, companyID int) (*domain.CompanyConfig, error)`:
  - Query: `SELECT id, companyId, fieldsOrderConfig, hasStock, createdAt, updatedAt FROM CompanyConfig WHERE companyId = ?`
  - Usa `r.db.QueryRowContext` (single row, UNIQUE constraint)
  - Retorna `NotFoundError` si `sql.ErrNoRows`

---

## Paso 8: Extender `ProductRepository` con operaciones transaccionales

**Archivo:** `internal/product/repository/products_repository.go` (EDITAR)

**Qué hacer:**
- Agregar método `FindByIDForUpdate(ctx context.Context, tx *sql.Tx, productID int, companyID int) (*domain.Product, error)`:
  - Query: `SELECT id, external_id, name, description, price, stock, reserved_stock, companyId, typeId, category, isActive, isDeleted, hasStock, Stockeable, createdAt, updatedAt FROM Product WHERE id = ? AND companyId = ? AND isDeleted = 0 FOR UPDATE`
  - Usa `tx.QueryRowContext` (dentro de transacción, row-level lock)
  - Retorna `NotFoundError` si `sql.ErrNoRows`
- Agregar método `IncrementReservedStock(ctx context.Context, tx *sql.Tx, productID int, quantity int) error`:
  - Query: `UPDATE Product SET reserved_stock = COALESCE(reserved_stock, 0) + ? WHERE id = ?`
  - Usa `tx.ExecContext` (dentro de transacción)

---

## Paso 9: Compilar y verificar

**Comando:** `go build ./...`

**Qué verificar:**
- Todos los archivos compilan sin errores
- No hay imports no utilizados
- Los tipos coinciden con lo esperado por el spec

---

## Archivos tocados (resumen)

| # | Archivo | Acción |
|---|---------|--------|
| 1 | `internal/domain/order.go` | CREAR |
| 2 | `internal/domain/order_item.go` | CREAR |
| 3 | `internal/domain/company_config.go` | CREAR |
| 4 | `internal/errors/errors.go` | EDITAR (agregar NotFoundError) |
| 5 | `internal/order/repository/order_repository.go` | CREAR |
| 6 | `internal/order/repository/order_item_repository.go` | CREAR |
| 7 | `internal/company/repository/company_config_repository.go` | CREAR |
| 8 | `internal/product/repository/products_repository.go` | EDITAR (agregar 2 métodos) |

**No se toca:** `main.go`, `wire.go`, `router.go`, DTOs, controllers, usecases — eso corresponde a specs 2 y 3.

---

## Decisiones de diseño

1. **`NotFoundError` en `internal/errors/`**: El spec pide retornar "error NotFound" desde repos. Se agrega al módulo de errores existente siguiendo el patrón ya establecido (`ValidationError`, `InternalError`).

2. **Repos nuevos no se conectan a `wire.go` aún**: Este spec solo crea las implementaciones. El wiring ocurrirá en spec-2 cuando se cree el service que los consume.

3. **Interfaces NO se definen aquí**: Siguiendo CLAUDE.md, las interfaces se definen en el **consumidor** (el service de spec-2), no en el repositorio. Los repos solo exponen structs concretos.

4. **`*sql.DB` en struct, `*sql.Tx` en método**: Los repos guardan `*sql.DB` para lecturas fuera de transacción. Los métodos transaccionales reciben `*sql.Tx` como parámetro, según el spec.
