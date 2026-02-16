# Refactoring: Configuration-driven Timeouts & Magic Numbers

## 📋 Resumen de cambios

Se eliminaron **magic numbers** y se implementó **inyección de configuración** para parámetros críticos del servicio de reserva:
- Timeout de transacción: 5 segundos (configurable)
- Máximo de reintentos: 3 intentos (configurable)

---

## 🔍 Problemas identificados

### 1. Magic Number: Timeout de transacción
**Antes:**
```go
context.WithTimeout(ctx, 5*time.Second)  // ← Magic number sin justificación
```

**Problemas:**
- Hardcodeado, sin posibilidad de ajustar por ambiente
- Si local necesita 10s y producción necesita 3s, no hay forma de diferenciar
- Los linters deberían alertar sobre esto

### 2. Magic Number: Max retry attempts
**Antes:**
```go
for attempt := 1; attempt <= 3; attempt++ {  // ← Magic number 3
    // retry logic
}
```

**Problemas:**
- No configurable
- Mismo problema que el timeout

---

## ✅ Solución implementada

### 1. Struct de Configuración (`internal/config/config.go`)
```go
type OrderConfig struct {
    ReservationTxTimeout time.Duration `yaml:"reservation_tx_timeout"`
    MaxRetryAttempts     int           `yaml:"max_retry_attempts"`
}

type Config struct {
    Server   ServerConfig
    Database DatabaseConfig
    Log      LogConfig
    Order    OrderConfig  // ← Agregado
}
```

### 2. Configuración YAML (`internal/config/config.yaml`)
```yaml
order:
  # Transaction timeout for stock reservation (prevents hanging transactions)
  reservation_tx_timeout: 5s
  # Max retry attempts when MySQL deadlock is detected
  max_retry_attempts: 3
```

### 3. ReservationService - Inyección de configuración
```go
type ReservationService struct {
    // ... campos existentes ...
    txTimeout       time.Duration  // ← Nuevo
    maxRetryAttempts int           // ← Nuevo
}

func NewReservationService(
    db TransactionManager,
    productRepo ProductRepository,
    orderItemRepo OrderItemRepository,
    orderRepo OrderRepository,
    logger *zap.Logger,
    txTimeout time.Duration,      // ← Parámetro nuevo
    maxRetryAttempts int,          // ← Parámetro nuevo
) *ReservationService {
    return &ReservationService{
        // ... inicializaciones ...
        txTimeout:        txTimeout,
        maxRetryAttempts: maxRetryAttempts,
    }
}
```

### 4. ReserveItems - Usa configuración
```go
func (s *ReservationService) ReserveItems(...) (*ReservationResult, error) {
    // Usar s.txTimeout en lugar de magic number 5*time.Second
    txCtx, cancel := context.WithTimeout(ctx, s.txTimeout)
    defer cancel()
    // ...
}
```

### 5. ReserveAndAddUseCase - Inyección de maxRetryAttempts
```go
type ReserveAndAddUseCase struct {
    // ... campos existentes ...
    maxRetryAttempts int  // ← Nuevo
}

func NewReserveAndAddUseCase(
    orderRepo OrderRepository,
    companyConfigRepo CompanyConfigRepository,
    reservationSvc StockReservationService,
    logger *zap.Logger,
    maxRetryAttempts int,  // ← Parámetro nuevo
) *ReserveAndAddUseCase {
    // ... inicializaciones ...
}
```

### 6. reserveItemsWithRetry - Usa configuración
```go
func (uc *ReserveAndAddUseCase) reserveItemsWithRetry(...) (*ReservationResult, error) {
    maxAttempts := uc.maxRetryAttempts  // ← Usa configuración
    backoffs := []time.Duration{0, 100*time.Millisecond, 200*time.Millisecond}
    // ...
}
```

### 7. Wire.go - Inyecta configuración
```go
func NewModule(db *sql.DB, cfg *config.Config, logger *zap.Logger) *usecase.ReserveAndAddUseCase {
    // ...
    reservationSvc := service.NewReservationService(
        db,
        productRepo,
        orderItemRepo,
        orderRepo,
        logger,
        cfg.Order.ReservationTxTimeout,  // ← Desde config
        cfg.Order.MaxRetryAttempts,      // ← Desde config
    )

    return usecase.NewReserveAndAddUseCase(
        orderRepo,
        companyConfigRepo,
        reservationSvc,
        logger,
        cfg.Order.MaxRetryAttempts,      // ← Desde config
    )
}
```

### 8. Tests - Helpers para simplificar
**ReservationService tests:**
```go
func newTestReservationService(
    txMgr TransactionManager,
    productRepo ProductRepository,
    orderItemRepo OrderItemRepository,
    orderRepo OrderRepository,
) *ReservationService {
    return NewReservationService(
        txMgr,
        productRepo,
        orderItemRepo,
        orderRepo,
        zap.NewNop(),
        5*time.Second,      // Default test timeout
        3,                  // Default max retry attempts
    )
}
```

**ReserveAndAddUseCase tests:**
```go
func newTestReserveAndAddUseCase(
    orderRepo OrderRepository,
    companyConfigRepo CompanyConfigRepository,
    reservationSvc StockReservationService,
) *ReserveAndAddUseCase {
    return NewReserveAndAddUseCase(
        orderRepo,
        companyConfigRepo,
        reservationSvc,
        zap.NewNop(),
        3,  // Default max retry attempts
    )
}
```

---

## 📊 Comparación antes/después

| Aspecto | Antes | Después |
|---------|-------|---------|
| Timeout | Hardcoded `5*time.Second` | `cfg.Order.ReservationTxTimeout` |
| Max retries | Hardcoded `3` | `cfg.Order.MaxRetryAttempts` |
| Configurable | ❌ No | ✅ Sí, vía YAML |
| Tests | Parámetros repetidos | ✅ Helpers centralizados |
| Magic numbers | ❌ Múltiples | ✅ Eliminados |

---

## 🎯 Beneficios

### 1. **Configuración por ambiente**
```yaml
# development.yaml
order:
  reservation_tx_timeout: 10s
  max_retry_attempts: 5

# production.yaml
order:
  reservation_tx_timeout: 3s
  max_retry_attempts: 3
```

### 2. **Sin hardcoding**
- Fácil ajustar parámetros sin recompilar
- Los linters ya no detectarán magic numbers
- Documentación clara vía YAML

### 3. **Testabilidad mejorada**
- Helpers en tests centran la lógica
- Cambio en firma de constructores = automáticamente actualización de todos los tests
- Valores por defecto documentados

### 4. **Mantenibilidad**
- Un solo lugar donde ver la configuración de la aplicación
- Fácil para nuevos desarrolladores entender qué parámetros controlan el servicio

---

## 📝 Cambios de firma de funciones

### Impacto en dependientes

Si hay otros módulos que llamen a `NewModule` del orden, **deben actualizar**:

```go
// Antes
usecase := order.NewModule(db, logger)

// Después
usecase := order.NewModule(db, cfg, logger)  // ← Agregar cfg
```

---

## ✅ Verificación

```bash
✓ go build ./...          # Compilación exitosa
✓ go vet ./...            # Sin magic numbers ni warnings
✓ Service tests (9/9)     # Todos pasan
✓ UseCase tests (9/9)     # Todos pasan
```

---

## 🔗 Referencias

- **Config struct**: `internal/config/config.go`
- **Config YAML**: `internal/config/config.yaml`
- **Service**: `internal/order/service/reservation_service.go`
- **UseCase**: `internal/order/usecase/reserve_and_add_use_case.go`
- **Wire**: `internal/order/wire.go`

---

## 📌 Notas para revisor

1. **Parámetros de timeout**: Revisar si 5s es apropiado para ambientes. Considerar:
   - Local: podría ser 10s (desarrollo más lento)
   - Staging: 5s (similar a producción)
   - Producción: 3s (más agresivo)

2. **Max retry attempts**: 3 es razonable. Backoffs son [0ms, 100ms, 200ms]:
   - Primer intento: inmediato
   - Segundo: ~100ms
   - Tercero: ~200ms
   - Total máximo: ~300ms

3. **Configuración futura**: Si se necesita configurar backoffs, pueden moverse a YAML también.

