# Stock Reservation Service - Plan 2 Implementation

## 📋 Resumen

Implementación de la capa de **Service** y **UseCase** del módulo `order` para el flujo de reserva de stock. Incluye orquestación atómica con transacciones, validación de stock, manejo de deadlocks con retry, y ordenamiento anti-deadlock.

**Spec**: `specs/plan-2-stock-reservation-service.md`

---

## 🎯 Cambios principales

### 1. DTOs de Dominio (`internal/dto/reservation.go`)
- `ReservationStatus`: ALL_SUCCESS, PARTIAL, ALL_FAILED
- `FailureReason`: NOT_FOUND, OUT_OF_STOCK, INSUFFICIENT_AVAILABLE, PRODUCT_INACTIVE
- `ReservationResult`: resultado de la operación de reserva
- `ItemSuccess`, `ItemFailure`: detalles por item
- `ReservationItem`: input para reserva

### 2. Errores de Negocio (`internal/errors/errors.go`)
Agregados 3 tipos de error semántico:
- `ConflictError`: orden no está en estado PENDING (409)
- `ForbiddenError`: mismatch de compañía (403)
- `DeadlockError`: máximo de reintentos de deadlock excedido (503/500)

### 3. Service Layer (`internal/order/service/reservation_service.go`)
**`ReservationService`** - Maneja lógica de dominio pura y transacciones

**Interfaces que consume** (definidas en este paquete):
- `TransactionManager`: inicia transacciones
- `ProductRepository`: acceso a productos
- `OrderItemRepository`: inserción de items de orden
- `OrderRepository`: actualización de estado y total de orden

**Métodos principales:**
- `ReserveItems()`: Orquesta la reserva completa dentro de una transacción
  - BEGIN TRANSACTION (Repeatable Read isolation)
  - Procesa cada item validando stock y creando order items
  - COMMIT si hay éxitos, ROLLBACK si todos fallan
  - Retorna `ReservationResult` con detalles de éxitos/fallos

- `reserveSingleItem()` (privado): Lógica por item
  - Valida producto existe y está activo
  - Valida stock disponible (si hasStockControl)
  - Incrementa reserved_stock
  - Crea OrderItem

**Características de transacción:**
- Timeout: 5 segundos
- Isolation: REPEATABLE_READ (previene phantom reads)
- Logging: INFO en éxitos, WARN en fallos, ERROR en excepciones

### 4. UseCase Layer (`internal/order/usecase/reserve_and_add_use_case.go`)
**`ReserveAndAddUseCase`** - Orquestrador puro (SIN manejo de transacciones)

**Interfaces que consume** (definidas en este paquete):
- `StockReservationService`: servicio de reserva
- `OrderRepository`: validación de orden
- `CompanyConfigRepository`: configuración de compañía

**Métodos principales:**
- `ReserveItems()`: Orquestación de pre-validaciones y retry
  - Valida orden existe y está PENDING
  - Valida compañía coincide
  - Fetch configuración de compañía (hasStock)
  - Ordena items por productID ASC (anti-deadlock)
  - Llama service con retry

- `reserveItemsWithRetry()` (privado): Implementa retry con jitter
  - Máximo 3 intentos
  - Backoffs: [0ms, 100ms, 200ms]
  - Jitter: ±20% para evitar "thundering herd"
  - Detecta MySQL errors 1213 (deadlock) y 1205 (lock wait timeout)
  - Retorna DeadlockError después de agotar intentos

**Separación clara de responsabilidades:**
- Service: maneja transacciones y lógica de dominio
- UseCase: orquesta, valida, mapea datos, implementa retry

### 5. DI Manual (`internal/order/wire.go`)
Función `NewModule(db, logger)` que:
- Instancia repositorios concretos
- Instancia service (recibe `db` para manejar transacciones)
- Instancia usecase (NO recibe `db`, solo orquesta)
- Retorna `ReserveAndAddUseCase` listo para usar

### 6. Tests Unitarios
**`internal/order/service/reservation_service_test.go`** (9 casos)
- AllSuccess: validar reserva exitosa con múltiples items
- AllFailed_NotFound: validar manejo de productos no encontrados
- Partial: mezcla de éxitos y fallos
- OutOfStock: stock insuficiente (available == 0)
- InsufficientAvailable: cantidad solicitada > disponible
- ProductInactive: producto inactivo
- NoStockControl: validación skipped cuando hasStockControl=false
- ProductNotStockeable: validación skipped cuando Stockeable=false
- DBErrorOnIncrement: propagación de errores inesperados de DB

**`internal/order/usecase/reserve_and_add_use_case_test.go`** (9 casos)
- OrderNotFound, OrderNotPending, CompanyMismatch, CompanyConfigNotFound: validaciones de pre-condiciones
- AllSuccess, AllFailed: casos de éxito y fallo
- ItemsSortedByProductID: validar ordenamiento anti-deadlock
- DeadlockRetry: validar retry exitoso en deadlock
- DeadlockMaxRetries: validar error cuando se agotan intentos

**Patrón de mocking:**
- Implementar interfaces con structs que tienen campos de función
- Sin librerías externas de mocking (acorde a CLAUDE.md)

---

## 📊 Arquitectura

### Flujo de dependencias
```
Controller (spec-3)
    ↓
UseCase.ReserveItems()
    ├─ Pre-validaciones (sin transacción)
    ├─ Ordenamiento anti-deadlock
    └─ Service.ReserveItems() con retry
        ├─ BEGIN TRANSACTION
        ├─ Para cada item (ordenado):
        │  ├─ FindByIDForUpdate (lock)
        │  ├─ Validar stock
        │  ├─ IncrementReservedStock
        │  └─ Insert OrderItem
        ├─ COMMIT o ROLLBACK
        └─ Retorna ReservationResult
```

### Interfaces por capa
- **Service**: Define sus propias interfaces (TransactionManager, ProductRepository, etc.)
- **UseCase**: Define sus propias interfaces (StockReservationService, OrderRepository, CompanyConfigRepository)
- Implementadores (repositorios concretos) satisfacen implícitamente estas interfaces
- Permite cambiar implementaciones sin afectar consumidores ✓

---

## ✅ Verificación

```bash
✓ go build ./...          # Compilación exitosa
✓ go vet ./...            # Sin errores de análisis
✓ Service tests (9/9)     # Todos pasan
✓ UseCase tests (9/9)     # Todos pasan
```

### Archivos tocados
| Archivo | Acción |
|---------|--------|
| `internal/dto/reservation.go` | CREAR |
| `internal/errors/errors.go` | EDITAR (agregar 3 error types) |
| `internal/order/service/reservation_service.go` | CREAR |
| `internal/order/usecase/reserve_and_add_use_case.go` | CREAR |
| `internal/order/wire.go` | CREAR |
| `internal/order/service/reservation_service_test.go` | CREAR |
| `internal/order/usecase/reserve_and_add_use_case_test.go` | CREAR |
| `docs/architecture-hexagonal.puml` | CREAR |
| `docs/class-order-module.puml` | CREAR |
| `docs/sequence-reserve-items.puml` | CREAR |
| `docs/sequence-deadlock-retry.puml` | CREAR |
| `docs/flowchart-reserve-single-item.puml` | CREAR |
| `docs/state-order-transaction.puml` | CREAR |
| `docs/README.md` | CREAR |

---

## 🎯 Decisiones de Diseño

### 1. **Service maneja transacciones, UseCase no**
- Transacción es de responsabilidad de Service
- Garantiza atomicidad de la reserva
- UseCase solo orquesta pre-validaciones y retry

### 2. **Interfaces en consumidor, no en productor**
- Service define `ProductRepository`, `OrderItemRepository`, etc.
- UseCase define `StockReservationService`, `OrderRepository`, etc.
- Repositorios concretos NO saben de estas interfaces
- Ventaja: cambiar implementación sin afectar capas superiores

### 3. **Ordenamiento ASC por ProductID**
- Previene deadlocks cuando múltiples transacciones acceden mismo conjunto
- Garantiza determinismo en orden de locks

### 4. **Retry con Jitter**
- Evita "thundering herd"
- Backoff exponencial: 0ms → 100ms → 200ms
- ±20% jitter aleatorio

### 5. **Isolation Level REPEATABLE_READ**
- Previene phantom reads y dirty reads
- Balance entre seguridad y performance
- Suficiente para este caso de uso

### 6. **defer tx.Rollback() seguro**
- MySQL ignora rollback si ya fue committed
- Garantiza que NUNCA quedes con transacción abierta

---

## 📚 Documentación

Se agregó paquete `docs/` con 6 diagramas PlantUML:
- `architecture-hexagonal.puml`: Arquitectura 4-capas
- `class-order-module.puml`: Clases e interfaces del módulo
- `sequence-reserve-items.puml`: Flujo completo con transacción
- `sequence-deadlock-retry.puml`: Lógica de retry
- `flowchart-reserve-single-item.puml`: Decisiones por item
- `state-order-transaction.puml`: Estados de orden y transacción
- `README.md`: Explicación detallada

Visualizar en: https://www.plantuml.com/plantuml/

---

## 🔗 Próximos pasos

**Spec 3**: Agregar Controller HTTP
- Mapeo de request/response HTTP
- Status codes (200, 409, 403, 503, etc.)
- Integración con router Chi
- Validación de schema

---

## 📝 Checklist

- [x] Lógica de service implementada
- [x] Lógica de usecase implementada
- [x] DI manual configurado
- [x] Tests unitarios (18 casos totales)
- [x] Compilación exitosa (`go build`)
- [x] Analysis exitoso (`go vet`)
- [x] Diagramas PlantUML creados
- [x] Documentación actualizada
- [x] Comentarios conciso en código crítico
- [x] Sigue CLAUDE.md (arquitectura, convenciones, sin libs adicionales)

---

## 📞 Notas para revisor

1. **Patrón de DI**: Revisar que el wiring en `wire.go` sea correcto y que todas las dependencias se inyecten en el orden correcto.

2. **Transacciones**: Verificar que `defer tx.Rollback()` siempre esté presente para evitar leaks. MySQL ignora rollback post-commit, por lo que es seguro.

3. **Tests**: Los tests usan mocks simples (no librerías externas). Para tests de transacción real, se dejarían para integration tests (spec-3 lo considerará si es necesario).

4. **Retry logic**: El jitter previene thundering herd cuando múltiples transacciones detectan deadlock simultáneamente.

5. **Ordenamiento**: Items se ordenan por `productID ASC` en el usecase para prevenir deadlocks en acceso concurrente.

---

🤖 Generado con [Claude Code](https://claude.com/claude-code)
