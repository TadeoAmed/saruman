# Saruman 🔮

Microservicio Go que reemplaza lógica de n8n para Vincula Latam. Gestión robusta de stock y órdenes con transacciones atómicas, manejo de concurrencia y API REST.



<details>
<summary><strong>📋 Contexto de la Aplicación</strong></summary>

### Resumen del Proyecto

**Saruman** es un microservicio independiente en Go desarrollado para Vincula Latam que centraliza la lógica de gestión de stock y órdenes, reemplazando implementaciones frágiles en n8n.

#### 🎯 Problema que Resuelve

La plataforma de distribución de Vincula Latam necesitaba:
- ✅ **Reserva segura de stock** bajo concurrencia simultánea (múltiples órdenes compitiendo)
- ✅ **Órdenes parciales** (no all-or-nothing): si un producto no tiene stock, los otros se reservan igual
- ✅ **Transacciones atómicas** garantizadas en base de datos (ACID)
- ✅ **Manejo automático de deadlocks** sin intervención manual
- ✅ **Trazabilidad completa** para auditoría y debugging
- ✅ **Latencia predecible** (<500ms p95) para buena UX

#### 🏗️ Modelos de Negocio

El servicio maneja tres entidades principales:

1. **Productos** (`Product`)
   - Stock real disponible
   - Stock reservado (en órdenes pendientes)
   - Disponibilidad = stock - reserved_stock

2. **Órdenes** (`Orders`)
   - Estados: `PENDING` (creada), `CREATED` (items procesados), `CANCELED`
   - Múltiples items, cada uno con cantidad y precio

3. **Items de Orden** (`OrderItems`)
   - Registro de qué producto, cantidad y precio en una orden
   - Creado de forma atómica junto con la reserva de stock

#### 🔄 Flujos Principales

**Reserve-and-Add**: Reserva stock y crea order items en una transacción atómica
- Cliente envía: companyId + items (productId, qty, price)
- Servicio: valida disponibilidad, incrementa `reserved_stock`, crea `OrderItems`
- Retorna: successes (items procesados) + failures (items sin stock con razón)

**Confirm** (futuro): Descuenta stock real y confirma la orden
- Decrementa `stock` y `reserved_stock` simultáneamente
- Marca orden como confirmada

**Cancel** (futuro): Libera reservas sin consumir stock
- Decrementa solo `reserved_stock`
- Recupera disponibilidad para otras órdenes

#### 📊 Caso de Uso Real

```
Producto A: stock=100, reserved_stock=0 (disponible=100)

Orden 1 → Reserva 40 unidades
Orden 2 → Reserva 40 unidades
Orden 3 → Solicita 40 unidades    ← Ejecutadas ~simultáneamente

Con Saruman (transacciones + SELECT FOR UPDATE):
  ✓ Orden 1: Reserva 40 → reserved_stock=40
  ✓ Orden 2: Reserva 40 → reserved_stock=80
  ✗ Orden 3: Rechazada (disponible=20 < 40 solicitados)

Sin Saruman (n8n secuencial):
  ✗ Overselling: Las tres órdenes se procesan sin bloqueos
    → reserved_stock termina en 120 (¡mayor que stock real!)
```

</details>

---

<details>
<summary><strong>🚀 Requisitos, Dependencias y Setup Local</strong></summary>

### Para Personas Sin Go Instalado

Todos los requisitos se pueden usar vía Docker. Si prefieres no instalar nada localmente:

#### ✅ Opción 1: Con Docker (Recomendado para Principiantes)

**Requisitos mínimos:**
- Docker Desktop instalado ([descargar aquí](https://www.docker.com/products/docker-desktop))

**Comandos:**

```bash
# Clonar el repositorio
git clone <repo-url>
cd saruman

# Construir la imagen Docker
docker build -t saruman:latest .

# Ejecutar el servidor (requiere MySQL corriendo)
docker-compose up -d

# Ver logs
docker-compose logs -f saruman

# Detener
docker-compose down
```

---

#### ✅ Opción 2: Setup Local Completo

**Requisitos a instalar:**

| Requisito | Versión | Instalación | Para qué |
|-----------|---------|-------------|----------|
| **Go** | 1.25+ | [Descargar](https://golang.org/dl) | Runtime del servicio |
| **MySQL** | 8.0+ | [Docker](https://hub.docker.com/_/mysql) o [Installer](https://dev.mysql.com/downloads/mysql/) | Base de datos |
| **Git** | Cualquier versión | [Descargar](https://git-scm.com/) | Control de versiones |
| **Make** | (opcional) | Incluido en macOS/Linux; [MinGW](http://www.mingw.org/) en Windows | Automación de builds |

**Instalación de dependencias con Go:**

```bash
# Instalar dependencias del módulo Go (automático en primer build)
go mod download

# (Opcional) Validar que todo está bien
go mod tidy
```

---

### Comandos para Ejecutar Localmente

**1. Configurar base de datos:**

```bash
# Copiar archivo de ejemplo
cp .env.example .env

# Editar .env con tus credenciales MySQL
# DATABASE_URL=mysql://user:password@localhost:3306/vincula

# Crear la base de datos (si no existe)
mysql -u root -p -e "CREATE DATABASE IF NOT EXISTS vincula;"

# Importar schema (si existe migrations/)
# mysql -u root -p vincula < migrations/schema.sql
```

**2. Cargar configuración:**

```bash
# El archivo config.yaml debe estar en internal/config/config.yaml
# Editar con tu setup local:
cat > internal/config/config.yaml << 'EOF'
server:
  port: 8080
database:
  host: localhost
  port: 3306
  user: root
  password: ""
  name: vincula
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: 5m
log:
  level: info
EOF
```

**3. Ejecutar el servidor:**

```bash
# Build y run
go run ./cmd/server/main.go

# O con Make (si instalaste):
make build
make run

# El servidor escuchará en http://localhost:8080
```

**4. Probar que está vivo:**

```bash
# Health check
curl http://localhost:8080/health

# Respuesta esperada:
# {"status":"ok"}
```

**5. Prueba un endpoint:**

```bash
# Buscar productos
curl -X POST http://localhost:8080/products/search \
  -H "Content-Type: application/json" \
  -d '{
    "companyId": 1,
    "productIds": [1, 2, 3]
  }'
```

---

### Solución de Problemas

| Problema | Solución |
|----------|----------|
| **"command not found: go"** | Go no está instalado. Instala desde https://golang.org/dl |
| **"connection refused" en MySQL** | Asegúrate de que MySQL está corriendo (`docker ps` o check servicio) |
| **"database does not exist"** | Crea la DB: `mysql -u root -p -e "CREATE DATABASE vincula;"` |
| **Puerto 8080 ya en uso** | Cambia en `config.yaml` o mata el proceso: `lsof -i :8080` |
| **Error de dependencias** | Ejecuta `go mod tidy && go mod download` |

</details>

---

<details>
<summary><strong>🏛️ Lógica de Arquitectura y Casos de Uso</strong></summary>

### Arquitectura Hexagonal (4 Capas)

Saruman sigue una arquitectura hexagonal estricta con flujo de dependencias unidireccional:

```
HTTP Request
    │
    ▼
┌─────────────────────────────────────┐
│  🌐 Controller                      │  Parsea request HTTP
│  ├─ Validación de schema            │  Retorna response HTTP
│  └─ HTTP status codes               │  NO lógica de negocio
└────────────────┬────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────┐
│  🔗 UseCase (Orquestador)          │  Orquesta el flujo
│  ├─ Mapeo domain ↔ DTO             │  Pre-validaciones
│  ├─ Llamadas a Services            │  NO lógica de negocio
│  └─ Composición de resultados      │
└────────────────┬────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────┐
│  💼 Service                         │  Lógica de dominio pura
│  ├─ Reglas de negocio              │  Transacciones
│  ├─ Cálculos y validaciones        │  Manejo de deadlocks
│  └─ Orquestación transaccional     │  NO importa DTOs HTTP
└────────────────┬────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────┐
│  🗄️ Repository                     │  SQL puro
│  ├─ Queries parametrizadas         │  Mapeo rows → domain
│  └─ Transacciones BD               │  NO lógica de negocio
└────────────────┬────────────────────┘
                 │
                 ▼
            🗄️ MySQL
```

**Regla clave**: Cada capa solo depende de INTERFACES, nunca de implementaciones.

---

### Casos de Uso Implementados

#### 1️⃣ Search Products (MVP - Implementado)

**Descripción**: Busca productos por IDs dentro de una compañía.

**Endpoint**: `POST /products/search`

**Flujo lógico**:
```
Controller recibe:  { "companyId": 1, "productIds": [101, 202] }
    ↓
UseCase → Service:  "Obtén estos productos"
    ↓
Service → Repository: "Busca en BD"
    ↓
Repository ejecuta: SELECT * FROM Product WHERE id IN (101,202) AND companyId=1
    ↓
Service compara:    IDs solicitados vs IDs encontrados
    ↓
UseCase mapea:      domain.Product → ProductDTO (con availableStock calculado)
    ↓
Controller retorna: { "products": [...], "notFound": [...] }
```

**Validaciones**:
- ✅ companyId requerido, entero > 0
- ✅ productIds no vacío, máximo 100 elementos
- ✅ Cada productId entero > 0

**Respuesta exitosa (200)**:
```json
{
  "products": [
    {
      "id": 101,
      "name": "Laptop",
      "price": 1200.00,
      "stock": 50,
      "reservedStock": 10,
      "availableStock": 40,
      "category": "electronics"
    }
  ],
  "notFound": [999]
}
```

---

#### 2️⃣ Reserve-and-Add (MVP - Implementado)

**Descripción**: Reserva stock y crea items de orden de forma atómica.

**Endpoint**: `POST /orders/{orderId}/reserve-and-add`

**Flujo con transacción (SELECT FOR UPDATE)**:

```
BEGIN TRANSACTION
    ↓
Para cada item (ordenado por productId ASC):
    ├─ SELECT id, stock, reserved_stock, hasStock, stockeable FROM Product WHERE id=X FOR UPDATE
    │   (bloquea filas para evitar race conditions)
    │
    ├─ VALIDAR: Producto activo?
    │
    ├─ VALIDAR: ¿hasStock=true AND stockeable=true? (SIEMPRE, incondicional)
    │   ├─ NO → Agregar a "failures" con razón PRODUCT_NOT_STOCKEABLE
    │   └─ SÍ → Continuar
    │
    ├─ VALIDAR: (stock - reserved_stock) >= cantidad_solicitada? (SIEMPRE, incondicional)
    │   ├─ Disponible = 0 → Agregar a "failures" con razón OUT_OF_STOCK
    │   ├─ Disponible < cantidad → Agregar a "failures" con razón INSUFFICIENT_AVAILABLE
    │   └─ Disponible >= cantidad → Continuar
    │
    ├─ SI ✓ → UPDATE Product SET reserved_stock += cantidad
    │       → INSERT INTO OrderItems (orderId, productId, qty, price)
    │       → Agregar a "successes"
    │
    └─ SI ✗ → Agregar a "failures" con razón específica

Si al menos 1 success:
    ├─ UPDATE Orders SET status = CREATED
    └─ COMMIT → Retorna 200 (all success) o 206 (partial)

Si 0 successes:
    └─ ROLLBACK → Retorna 422 (ninguno procesado)
```

**CAMBIO CRÍTICO (Feb 2026)**: La validación de stock **SIEMPRE ocurre**, independientemente de `companyConfig.HasStock`.
- **Antes**: Se saltaba validación si company tenía `HasStock=false`
- **Ahora**: SIEMPRE se valida que cada producto sea stockeable (HasStock && Stockeable)
- **Razón**: Prevenir overselling - items con `stock=2, reserved=2, available=0` ahora son correctamente rechazados

**Ejemplo de race condition resuelta**:

```
Producto: stock=100, reserved_stock=0

Sin SELECT FOR UPDATE:           Con SELECT FOR UPDATE (Saruman):
Transacción A → lee (100,0)      Transacción A → SELECT FOR UPDATE (BLOQUEA)
Transacción B → lee (100,0)      Transacción B → espera...
Transacción A → UPDATE a 50      Transacción A → UPDATE a 50, COMMIT
Transacción B → UPDATE a 50      Transacción B → ahora lee (100,50)
                                  Transacción B → puede restar más ✓
```

**Validaciones**:
- ✅ **Company-level (UseCase)**: companyConfig.HasStock debe ser `true` (si es false → error CONFLICT inmediato)
- ✅ orderId existe y está en estado PENDING
- ✅ companyId coincide con la orden
- ✅ Cada productId pertenece a la companyId
- ✅ **Product-level (Service, SIEMPRE)**:
  - Producto activo: `IsActive=true`
  - Producto stockeable: `HasStock=true` AND `Stockeable=true`
  - Stock disponible: `(stock - reserved_stock) > 0`
  - Cantidad suficiente: `(stock - reserved_stock) >= cantidad_solicitada`
- ✅ Cantidades entre 1 y 10,000
- ✅ Sin items duplicados en el request

**Respuesta exitosa (200 - Todas OK)**:
```json
{
  "traceId": "550e8400-e29b-41d4-a716-446655440000",
  "orderId": 123,
  "status": "ALL_SUCCESS",
  "totalPrice": 2400.00,
  "addedItems": [101, 202],
  "successes": [
    {"productId": 101, "quantity": 2},
    {"productId": 202, "quantity": 5}
  ],
  "failures": [],
  "timestamp": "2026-02-17T15:30:45Z"
}
```

**Respuesta parcial (206 - Algunas OK)**:
```json
{
  "traceId": "...",
  "orderId": 123,
  "status": "PARTIAL",
  "totalPrice": 2400.00,
  "addedItems": [101],
  "successes": [{"productId": 101, "quantity": 2}],
  "failures": [{"productId": 202, "quantity": 5, "reason": "OUT_OF_STOCK"}],
  "timestamp": "2026-02-17T15:30:45Z"
}
```

**Manejo de Deadlock**:
```
Si BD retorna error 1213 (ER_LOCK_DEADLOCK):
    Intento 1: Reintentar inmediatamente
    Intento 2: Esperar 100ms + jitter, reintentar
    Intento 3: Esperar 200ms + jitter, reintentar
    Intento 4: Fallar con 409 Conflict
```

---

#### 3️⃣ Confirm Order (Futuro)

Descontará stock real y confirmará la orden (no implementado aún).

---

#### 4️⃣ Cancel Order (Futuro)

Liberará reservas sin consumir stock (no implementado aún).

---

### Modelo de Stock Explicado

**Dos columnas en `Product` table**:
- `stock`: Cantidad REAL disponible en almacén
- `reserved_stock`: Cantidad RESERVADA (en órdenes pendientes)
- **Disponible** = `stock - reserved_stock`

**Evolución en un escenario real**:

```
Inicial:  stock=100, reserved_stock=0
         (disponible: 100)

Orden A reserva 30:
         stock=100, reserved_stock=30
         (disponible: 70)

Orden B reserva 40:
         stock=100, reserved_stock=70
         (disponible: 30)

Orden C intenta reservar 40:
         ✗ Rechazada (disponible=30 < 40)

Confirmar Orden A (descontar real):
         stock=70, reserved_stock=40
         (disponible: 30) ✓ Consistente

Cancelar Orden B (liberar):
         stock=70, reserved_stock=0
         (disponible: 70) ✓ Stock recuperado
```

---

### Diagramas de Flujo

Visualiza en `docs/`:
- `sequence-reserve-items.puml`: Flujo secuencial reserve-and-add
- `sequence-deadlock-retry.puml`: Manejo de deadlock
- `state-order-transaction.puml`: Estados de transacción
- `architecture-hexagonal.puml`: Capas y dependencias

</details>

---

<details>
<summary><strong>🛠️ Tecnologías Utilizadas</strong></summary>

### Stack Principal

| Componente | Librería | Versión | Uso |
|-----------|----------|---------|-----|
| **Runtime** | Go | 1.25+ | Lenguaje de programación |
| **Router HTTP** | go-chi/chi/v5 | v5.2.5 | Enrutamiento REST |
| **Driver MySQL** | go-sql-driver/mysql | v1.9.3 | Conexión a BD |
| **Logging** | uber-go/zap | v1.27.1 | Logs estructurados (JSON) |
| **Config YAML** | go.yaml.in/yaml/v3 | v3.0.4 | Archivos de configuración |
| **UUID** | google/uuid | v1.6.0 | Generación de IDs únicos |

### Librerías Testing (Opcionales)

| Librería | Uso |
|----------|-----|
| `testify` | Assertions + mocks para unit tests |

### Infraestructura y Deployment

| Componente | Uso |
|-----------|-----|
| **Docker** | Containerización del servicio |
| **Docker Compose** | Orquestación de MySQL + Saruman |
| **Make** | Automación de builds y comandos |

### Patrones y Principios

| Patrón | Descripción |
|--------|------------|
| **Hexagonal Architecture** | 4 capas bien definidas: Controller → UseCase → Service → Repository |
| **Dependency Injection Manual** | Sin frameworks, cada módulo tiene `wire.go` |
| **Interface Segregation** | Cada capa depende de interfaces pequeñas y específicas |
| **SOLID Principles** | Single Responsibility, Open/Closed, Liskov, Interface Segregation, Dependency Inversion |

### Database

| Aspecto | Especificación |
|--------|----------------|
| **Engine** | MySQL 8.0+ |
| **Pool de Conexiones** | max_open=25, max_idle=5, lifetime=5m |
| **Isolations Level** | REPEATABLE READ (para transacciones) |
| **Queries** | Parametrizadas (prevenir SQL injection) |

### Características Disponibles

- ✅ **Logging estructurado** en JSON (producción)
- ✅ **Request tracing** con traceId único (UUID v4)
- ✅ **Graceful shutdown** en 10 segundos
- ✅ **Middleware recovery** para panics
- ✅ **Transacciones atómicas** con deadlock retry
- ✅ **Health checks** para monitoreo

### Características Futuras

- [ ] Autenticación Bearer Token / API Key
- [ ] Métricas Prometheus
- [ ] Trazas OpenTelemetry (Jaeger)
- [ ] Rate Limiting
- [ ] CORS configurables
- [ ] Migration system (golang-migrate/migrate)

</details>

---

## Contrato API

### Endpoints soportados

#### `POST /products/search`
Búsqueda de productos por compañía y lista de IDs.

#### `POST /orders/{orderId}/reserve-and-add`
Reserva y agrega items a una orden existente.

**Especificación OpenAPI:**

```yaml
POST /orders/{orderId}/reserve-and-add:
  summary: Reserve items for order
  parameters:
    - name: orderId
      in: path
      required: true
      schema:
        type: integer
        minimum: 1
  requestBody:
    required: true
    content:
      application/json:
        schema:
          type: object
          required: [companyId, items]
          properties:
            companyId:
              type: integer
              minimum: 1
            items:
              type: array
              minItems: 1
              maxItems: 100
              items:
                type: object
                required: [productId, quantity, price]
                properties:
                  productId:
                    type: integer
                    minimum: 1
                  quantity:
                    type: integer
                    minimum: 1
                    maximum: 10000
                  price:
                    type: number
                    format: float
                    minimum: 0
  responses:
    '200':
      description: All items reserved successfully
      content:
        application/json:
          schema:
            type: object
            properties:
              traceId: { type: string }
              orderId: { type: integer }
              status: { type: string, enum: [ALL_SUCCESS] }
              totalPrice: { type: number }
              addedItems: { type: array, items: { type: integer } }
              successes: { type: array }
              failures: { type: array }
              timestamp: { type: string, format: date-time }
    '206':
      description: Partial reservation (some items failed)
    '400':
      description: Validation error (invalid orderId, companyId, items structure)
    '404':
      description: Order not found
    '409':
      description: Conflict (order not PENDING) or Deadlock
    '422':
      description: All items failed reservation
    '500':
      description: Internal server error
```

### Códigos de error

| HTTP | Código | Mensaje | Razón |
|------|--------|---------|-------|
| 400 | `VALIDATION_ERROR` | Validation failed | Invalid input (orderId, companyId, items, quantities, prices, duplicates) |
| 404 | `NOT_FOUND` | order not found | Order ID no existe en base de datos |
| 409 | `CONFLICT` | order is not in PENDING status | La orden debe estar en estado PENDING |
| 409 | `CONFLICT` | la compañía solicitada no vende productos stockeables | `companyConfig.HasStock=false` - guard company-level |
| 403 | `FORBIDDEN` | company mismatch | companyId no coincide con la orden |
| 409 | `DEADLOCK` | max retries exceeded | Deadlock en BD, reintentable |
| 500 | `INTERNAL_ERROR` | an unexpected error occurred | Error interno del servidor |

### Razones de fallos en items (dentro de response exitoso)

| Código | Razón | Cuándo ocurre |
|--------|-------|--------------|
| `NOT_FOUND` | Producto no existe | ProductId no pertenece a la compañía |
| `PRODUCT_INACTIVE` | Producto inactivo | `product.IsActive=false` |
| **`PRODUCT_NOT_STOCKEABLE`** | Producto no stockeable | `product.HasStock=false` OR `product.Stockeable=false` (**SIEMPRE validado**) |
| `OUT_OF_STOCK` | Sin stock disponible | `availableStock = 0` (**SIEMPRE validado**) |
| `INSUFFICIENT_AVAILABLE` | Stock insuficiente | `availableStock < cantidad_solicitada` (**SIEMPRE validado**) |

### Respuestas

**Éxito (200 OK) — ALL_SUCCESS:**
```json
{
  "traceId": "uuid",
  "orderId": 123,
  "status": "ALL_SUCCESS",
  "totalPrice": 150.00,
  "addedItems": [101, 102],
  "successes": [{"productId": 101, "quantity": 2}],
  "failures": [],
  "timestamp": "2026-02-17T15:30:45Z"
}
```

**Parcial (206 Partial Content) — PARTIAL:**
```json
{
  "traceId": "uuid",
  "orderId": 123,
  "status": "PARTIAL",
  "totalPrice": 21.00,
  "addedItems": [101],
  "successes": [{"productId": 101, "quantity": 2}],
  "failures": [
    {"productId": 102, "quantity": 5, "reason": "OUT_OF_STOCK"},
    {"productId": 103, "quantity": 3, "reason": "PRODUCT_NOT_STOCKEABLE"}
  ],
  "timestamp": "2026-02-17T15:30:45Z"
}
```

**Validación (400 Bad Request):**
```json
{
  "error": "VALIDATION_ERROR",
  "message": "validation failed",
  "details": [
    {"field": "companyId", "message": "companyId is required"},
    {"field": "items[0].quantity", "message": "quantity must be between 1 and 10000"}
  ]
}
```

**Error (404, 409, 422, 500):**
```json
{
  "traceId": "uuid",
  "status": 404,
  "message": "order not found",
  "code": "NOT_FOUND",
  "orderId": 9999,
  "timestamp": "2026-02-17T15:30:45Z"
}
```
