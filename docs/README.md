# Saruman Architecture Diagrams

Este directorio contiene diagramas PlantUML que documentan la arquitectura e implementación del Stock Reservation Service.

## 📊 Diagramas disponibles

### 1. **architecture-hexagonal.puml**
Diagrama de la arquitectura hexagonal de 4 capas:
- **HTTP Layer**: Controllers (request/response)
- **Application Layer**: Use Cases (orchestration)
- **Domain Layer**: Services (business logic)
- **Infrastructure Layer**: Repositories (data access) + Database

Muestra el flujo de dependencias unidireccional que NUNCA debe invertirse.

---

### 2. **class-order-module.puml**
Diagrama de clases del módulo `order` mostrando:
- DTOs: `ReservationResult`, `ItemSuccess`, `ItemFailure`, `ReservationItem`
- Interfaces del Service: `TransactionManager`, `ProductRepository`, `OrderItemRepository`, `OrderRepository`
- Clase `ReservationService` (service layer)
- Interfaces del UseCase: `StockReservationService`, `OrderRepository`, `CompanyConfigRepository`
- Clase `ReserveAndAddUseCase` (usecase layer)
- Implementaciones concretas de repositorios (repository layer)
- Entidades de dominio: `Product`, `Order`, `OrderItem`, `CompanyConfig`

---

### 3. **sequence-reserve-items.puml**
Diagrama de secuencia del flujo completo de `ReserveItems`:

**Fase 1: Pre-validaciones (sin transacción)**
- Fetch orden
- Validar estado PENDING
- Validar company match
- Fetch company config

**Fase 2: Reserva (dentro de transacción)**
- BEGIN TRANSACTION (Repeatable Read isolation)
- Para cada item:
  - Fetch product con lock (FOR UPDATE)
  - Validar stock (si hasStockControl)
  - Incrementar reserved_stock
  - Crear OrderItem

**Fase 3: Commit o Rollback**
- Si todos fallaron: ROLLBACK
- Si algunos exitosos: UPDATE Order status/price + COMMIT
- Si todos exitosos: UPDATE Order + COMMIT

---

### 4. **sequence-deadlock-retry.puml**
Diagrama de secuencia del mecanismo de **retry con deadlock detection**:

**Retry Logic:**
- Máximo 3 intentos
- Backoffs: [0ms, 100ms, 200ms]
- Jitter: ±20% para evitar thundering herd
- Detección: MySQL error 1213 (deadlock) o 1205 (lock wait timeout)

**Comportamiento:**
- Error de deadlock + intentos restantes → Sleep + retry
- Error de deadlock + último intento → `DeadlockError("max retries exceeded")`
- Error no-deadlock → Retorna inmediatamente (sin retry)
- Éxito → Retorna resultado

---

### 5. **flowchart-reserve-single-item.puml**
Diagrama de flujo de decisión para la reserva de **un item individual**:

1. Fetch product con lock
2. Validar producto existe
3. Validar producto activo
4. Si hasStockControl:
   - Calcular stock disponible
   - Validar OUT_OF_STOCK
   - Validar INSUFFICIENT_AVAILABLE
   - Incrementar reserved_stock
5. Crear OrderItem
6. Retornar Success o Failure

---

### 6. **state-order-transaction.puml**
Diagrama de estados de la **orden y transacción**:

**Estados de Orden:**
- PENDING → CREATED (reserva exitosa/parcial)
- PENDING → PENDING (reserva completamente fallida)

**Estados de Transacción:**
- BEGIN TRANSACTION
- Processing Items (Isolation: REPEATABLE_READ)
- COMMIT (si successes > 0)
- ROLLBACK (si error DB o todos fallan)

**Resultados Posibles:**
- `ALL_SUCCESS`: todos los items reservados
- `PARTIAL`: algunos items exitosos, otros fallaron
- `ALL_FAILED`: ningún item reservado (transaction rolled back)

---

## 🔄 Flujo Completo: Vista General

```
Cliente
  ↓
[HTTP Request]
  ↓
UseCase.ReserveItems() [Pre-validaciones, ordenamiento]
  ↓
Service.ReserveItems() [Transacción BEGIN]
  ├─ Para cada item:
  │  ├─ FindByIDForUpdate (lock product)
  │  ├─ Validar stock
  │  ├─ IncrementReservedStock
  │  └─ Insert OrderItem
  │
  ├─ Decidir resultado
  └─ COMMIT o ROLLBACK
  ↓
UseCase.reserveItemsWithRetry() [Retry si deadlock]
  ↓
[HTTP Response] ReservationResult
  ↓
Cliente
```

---

## 📋 Patrón de DI (Inyección de Dependencias)

```
wire.go: NewModule(db, logger)
  ├─ orderRepo := NewMySQLOrderRepository(db)
  ├─ orderItemRepo := NewMySQLOrderItemRepository(db)
  ├─ productRepo := NewMySQLRepository(db)
  ├─ companyConfigRepo := NewMySQLCompanyConfigRepository(db)
  │
  ├─ service := NewReservationService(db, productRepo, orderItemRepo, orderRepo, logger)
  │
  └─ usecase := NewReserveAndAddUseCase(orderRepo, companyConfigRepo, service, logger)
```

---

## 🎯 Puntos Clave de Diseño

### 1. **Transacción en Service, no en UseCase**
- El service es responsable de BEGIN, COMMIT, ROLLBACK
- UseCase solo orquesta pre-validaciones y retry
- Separación clara de responsabilidades

### 2. **Interfaces Definidas en Consumidor**
- Service define sus propias interfaces (`ProductRepository`, `OrderItemRepository`, etc.)
- UseCase define sus propias interfaces (`StockReservationService`, etc.)
- Los implementadores (repositorios concretos) NO saben de estas interfaces
- Esto permite cambiar implementaciones sin afectar consumidores

### 3. **Ordenamiento ASC por ProductID**
- Previene deadlocks cuando múltiples transacciones acceden al mismo conjunto de productos
- Garantiza orden determinístico de locks

### 4. **Retry con Jitter**
- Evita "thundering herd": múltiples transacciones retrying simultáneamente
- Backoff exponencial: 0ms → 100ms → 200ms
- ±20% jitter aleatorio en cada backoff

### 5. **Isolation Level REPEATABLE_READ**
- Previene phantom reads y dirty reads
- Suficiente para este caso de uso
- Mejor performance que SERIALIZABLE

---

## 📝 Cómo usar estos diagramas

1. **Visualizar locally**: Usar herramientas como PlantUML IDE, VSCode extension, o web https://www.plantuml.com/plantuml/
2. **Generar imágenes**: `plantuml *.puml -o ../images/` (requiere PlantUML instalado)
3. **Documentación**: Incluir enlaces a estas imágenes en wikis o README

---

## 🔗 Referencias

- Plan de implementación: `specs/plan-2-stock-reservation-service.md`
- Arquitectura general: `docs/architecture.md`
- Contexto de negocio: `PROJECT_CONTEXT.md`
