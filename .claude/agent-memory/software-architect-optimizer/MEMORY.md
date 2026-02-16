# Software Architect Optimizer - Memory

## Proyecto: Saruman (Vincula Latam)
- Microservicio Go, arquitectura hexagonal 4 capas: Controller -> UseCase -> Service -> Repository -> MySQL
- Stack: go-chi/chi/v5, database/sql + go-sql-driver/mysql, uber-go/zap, google/uuid
- DI manual sin frameworks. Interfaces definidas en el consumidor (no en el productor).
- DTOs centralizados en internal/dto/. Domain entities en internal/domain/.

## Patrones confirmados en el codebase
- Módulo `product` es el módulo de referencia: service define interface Repository, usecase define interface Service.
- Repository usa *sql.DB directamente (no ORM). Queries parameterizadas siempre.
- Filtrar isDeleted = 0 en todas las queries de lectura.
- Tipos de error custom en internal/errors/: ValidationError, InternalError.

## Decisiones arquitectónicas importantes (spec-2 reservation)
- Tipos de resultado (ReservationResult, ItemSuccess, ItemFailure) van en internal/dto/, NO en paquete service. De lo contrario Controller dependería de service directamente.
- Service NO debe recibir *sql.Tx. Debe recibir una interfaz de repositorio. UseCase construye TxRepository wrapeando *sql.Tx y lo inyecta al Service.
- Deteccion de deadlock MySQL: usar errors.As con *mysql.MySQLError, Number == 1213. NO usar strings.Contains.
- IsolationLevel recomendado para reservas concurrentes: sql.LevelReadCommitted (menos gap locks que REPEATABLE READ default de MySQL).
- Retry debe verificar ctx.Done() en el sleep entre intentos para respetar cancelaciones HTTP.
- defer tx.Rollback() debe loggear si err != nil && !errors.Is(err, sql.ErrTxDone).

## Detalles de archivos clave
- internal/domain/product.go: entidad Product con método AvailableStock()
- internal/errors/errors.go: ValidationError, InternalError con Unwrap()
- internal/product/service/products_service.go: patrón de referencia para interfaces en consumidor
- internal/product/usecase/search_products_use_case.go: patrón de referencia para mapeo domain->DTO
