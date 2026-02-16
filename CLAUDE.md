# Saruman - Way of Work (WoW)

## Proyecto
Microservicio Go para Vincula Latam. Gestión de stock y órdenes, reemplazando lógica de n8n.

## Stack (no agregar libs sin justificación explícita)
- Router: go-chi/chi/v5
- DB: database/sql + go-sql-driver/mysql
- Config: go.yaml.in/yaml/v3 (cargado desde internal/config/config.yaml)
- Logging: uber-go/zap (JSON en producción)
- UUID: google/uuid

## Arquitectura: Hexagonal de 4 capas

Flujo de dependencias (NUNCA invertir):
```
Controller → UseCase → Service → Repository → MySQL
```

### Responsabilidades por capa

| Capa | Ubicación | Hace | NO hace |
|------|-----------|------|---------|
| Controller | `internal/{modulo}/controller/` | Parsear HTTP request/response, validar schema, HTTP status codes | Lógica de negocio, acceso a DB |
| UseCase | `internal/{modulo}/usecase/` | Orquestar flujo, mapear domain ↔ DTO, pre-validaciones | Lógica de negocio, acceso a DB, transacciones, retry de infraestructura |
| Service | `internal/{modulo}/service/` | Lógica de dominio pura, manejo de transacciones | Tocar HTTP, importar DTOs de request/response |
| Repository | `internal/{modulo}/repository/` | SQL puro, mapear rows → domain entities | Lógica de negocio, devolver DTOs |

**Regla de UseCase:** El use case es orquestador de services, NO ejecuta lógica de negocio ni infraestructura. Solo: pre-validaciones, ordenamiento de datos, mapeo de DTOs, y orquestación de llamadas a services. El retry ante deadlock y manejo de transacciones ocurren en el service, NO en el use case.

## Estructura de carpetas para nuevos módulos

Cuando crees un nuevo módulo (ej: `order`), seguir EXACTAMENTE este patrón:

```
internal/{modulo}/
├── controller/
│   └── {action}_controller.go    # Un archivo por endpoint/action
├── usecase/
│   └── {action}_use_case.go      # Un archivo por caso de uso
├── service/
│   └── {modulo}_service.go       # Lógica de dominio del módulo
├── repository/
│   └── {modulo}_repository.go    # Acceso a datos del módulo
└── wire.go                       # DI manual: repo → service → usecase → controller
```

## Convenciones de código

### DI (Inyección de Dependencias)
- DI manual sin frameworks. Cada módulo tiene `wire.go`
- `wire.go` exporta `NewModule(db *sql.DB, logger *zap.Logger) *controller.Controller`
- `main.go` conecta: config → logger → DB → {modulo}.NewModule → router → server

### Interfaces
- Cada capa depende de INTERFACES, nunca de implementaciones concretas
- Las interfaces se definen en el paquete que las CONSUME, no en el que las implementa

### Domain entities
- Van en `internal/domain/`. Un archivo por entidad
- Métodos de cálculo (ej: AvailableStock()) van en la entidad

### DTOs
- Centralizados en `internal/dto/`
- Request/Response DTOs para la API HTTP
- Mapeo domain ↔ DTO ocurre en el UseCase

### Errors
- Tipos de error custom en `internal/errors/`
- ValidationError para errores de schema/input
- InternalError para errores internos

### Infraestructura compartida
- `internal/infrastructure/logger/` → Factory de Zap logger
- `internal/infrastructure/mysql/` → Pool de conexiones MySQL
- `internal/commons/` → Utilidades de configuración
- `internal/config/` → Structs de config + config.yaml
- `internal/server/` → Router Chi + HTTP server lifecycle

## Reglas de SQL (repositorios)
- Queries raw SQL con database/sql (no ORM)
- Parameterized queries SIEMPRE (prevenir SQL injection)
- Nombres de columnas del schema existente en MySQL son camelCase mixto (companyId, isDeleted, createdAt)
- Filtrar siempre `isDeleted = 0` en queries de lectura
- Campos nullable → mapear a punteros en Go (*int, *string)

## Reglas de routing
- Rutas se registran en `internal/server/router.go`
- Middleware: Recoverer → RequestID → Logging (en ese orden)
- Cada módulo controller se monta como sub-router o grupo

## Reglas para commits y PRs
- Idioma: español para documentación, inglés para código (nombres de funciones, variables, comments)
- No crear archivos de documentación salvo que se pida explícitamente

## Referencias
- Arquitectura detallada: `docs/architecture.md`
- Contexto de negocio completo: `PROJECT_CONTEXT.md`
- Spec del MVP: `specs/mvp-product-search.md`


## AI Software Architect Framework

This project uses the AI Software Architect framework for structured architecture management.

### Framework Usage
- **Architecture Reviews**: "Start architecture review for version X.Y.Z" or "Review architecture for 'component'"
- **Specialized Reviews**: "Ask Security Architect to review these code changes"
- **ADR Creation**: "Create an ADR for 'topic'"
- **Recalibration**: "Start architecture recalibration for 'feature name'"

### Framework Structure
- `.architecture/decisions/` - Architectural Decision Records and principles
- `.architecture/reviews/` - Architecture review documents
- `.architecture/recalibration/` - Implementation plans from reviews
- `.architecture/members.yml` - Architecture team member definitions

Refer to `.architecture/decisions/principles.md` for architectural guidance.
