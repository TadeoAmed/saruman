# Microservicio Saruman - Contexto Completo del Proyecto

**Documento de Referencia Integral**
Última actualización: 2025-02-12
Autor: Claude Code

---

## 📋 Tabla de Contenidos

1. [Contexto General](#contexto-general)
2. [Problemática y Solución](#problemática-y-solución)
3. [Objetivos del Proyecto](#objetivos-del-proyecto)
4. [Alcance del Microservicio](#alcance-del-microservicio)
5. [Esquema de Base de Datos](#esquema-de-base-de-datos)
6. [Flujo de n8n a Migrar](#flujo-de-n8n-a-migrar)
7. [Especificaciones Técnicas](#especificaciones-técnicas)
8. [Requerimientos Funcionales](#requerimientos-funcionales)
9. [Requerimientos No-Funcionales](#requerimientos-no-funcionales)
10. [API Propuesta](#api-propuesta)
11. [Modelo de Stock y Semántica](#modelo-de-stock-y-semántica)
12. [Manejo de Concurrencia](#manejo-de-concurrencia)
13. [Validaciones y Reglas de Negocio](#validaciones-y-reglas-de-negocio)
14. [Observabilidad](#observabilidad)
15. [Arquitectura Propuesta](#arquitectura-propuesta)
16. [Stack Tecnológico](#stack-tecnológico)

---

## 1. Contexto General

### Empresa y Caso de Uso

El proyecto opera en el contexto de **Vincula Latam**, una plataforma de distribución que gestiona órdenes y catálogos de productos para múltiples empresas (tenants).

**Plataforma actual:** n8n (orquestación de workflows)
**Necesidad:** Extraer lógica robusta de gestión de stock hacia un microservicio independiente.

---

## 2. Problemática y Solución

### Problemática

n8n tiene limitaciones críticas para manejo robusto de:

- **Iteración compleja de productos** con lógica condicional
- **Reserva segura de stock** bajo concurrencia simultánea
- **Creación atómica de order_items** con validación transaccional
- **Confirmación/cancelación de órdenes** con rollback garantizado
- **Manejo de deadlocks** y race conditions en BD
- **Idempotencia** ante reintentos del cliente

### Solución Propuesta

**Microservicio independiente en Go** que:

1. Expone un único endpoint HTTP que orquesta toda la lógica
2. Realiza todo en una transacción atómica (BEGIN...COMMIT)
3. Valida y reserva stock en paralelo para múltiples productos
4. Retorna successes + failures para casos parciales (206 Partial Content)
5. Implementa retry automático ante deadlocks
6. Proporciona observabilidad integrada (logs, métricas, trazas)

### Beneficios

| Beneficio | Impacto |
|-----------|--------|
| Lógica robusta | No more "all-or-nothing" en n8n |
| Órdenes parciales | Maximizar fulfillment de items disponibles |
| Latencia predecible | Go + transacciones cortas |
| Escalabilidad | Pool de conexiones optimizado |
| Observabilidad | Logs + métricas + trazas distribuidas |
| Mantenibilidad | Código testeable, versionable, CI/CD |

---

## 3. Objetivos del Proyecto

### Objetivo Principal

Permitir armar **órdenes parciales** (no all-or-nothing) reservando stock por ítem, reportar cuáles ítems fallan y por qué, y confirmar o cancelar la orden de forma **consistente y segura bajo concurrencia**.

### Objetivos Secundarios

1. ✅ Validar stock disponible por ítem con clave `productId` (no por nombre)
2. ✅ Crear `order_items` dentro del servicio en una transacción atómica
3. ✅ Confirmar (descontar stock real) o cancelar (liberar reservas) la orden
4. ✅ Proveer API REST para integración con n8n y otros clientes
5. ✅ Implementar reintentos ante deadlocks sin intervención manual
6. ✅ Registrar trazabilidad completa de operaciones (audit logs)
7. ✅ Escalar a bajo volumen inicial, preparado para crecimiento

---

## 4. Alcance del Microservicio

### In Scope (Incluido)

- ✅ Validar disponibilidad de productos por compañía
- ✅ Reservar stock de manera atómica
- ✅ Crear `order_items` en estado reservado
- ✅ Confirmar órdenes (descontar stock real)
- ✅ Cancelar órdenes (liberar reservas)
- ✅ Manejar casos parciales (algunos items ok, otros fallan)
- ✅ Retry ante deadlocks con backoff exponencial
- ✅ Autenticación por API Key / Bearer Token
- ✅ Validación de permisos por tenant (companyId)
- ✅ Logs estructurados + Prometheus + OpenTelemetry

### Out of Scope (No Incluido)

- ❌ Gestión de pagos / facturas
- ❌ Soporte para descuentos / promociones complejas
- ❌ Gestión de devoluciones / cambios
- ❌ Forecasting o demand planning
- ❌ Integración con sistemas ERP complejos (solo lectura básica de stock)
- ❌ Multi-warehouse distribution (un único warehouse por empresa inicialmente)

---

## 5. Esquema de Base de Datos

### 5.1 Tabla: `Product`

```sql
CREATE TABLE `Product` (
  `id` int NOT NULL AUTO_INCREMENT,
  `external_id` int NOT NULL,
  `name` varchar(255) NOT NULL,
  `description` text NOT NULL,
  `price` decimal(10,2) NOT NULL,
  `isActive` tinyint(1) DEFAULT '1',
  `isDeleted` tinyint(1) DEFAULT '0',
  `createdAt` datetime DEFAULT CURRENT_TIMESTAMP,
  `updatedAt` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `companyId` int NOT NULL,
  `typeId` int NOT NULL,
  `hasStock` tinyint(1) DEFAULT '0',
  `stock` int DEFAULT NULL,
  `category` varchar(100) NOT NULL DEFAULT 'general',
  `reserved_stock` int DEFAULT NULL,
  `Stockeable` tinyint(1) NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_id_name` (`id`,`name`),
  UNIQUE KEY `uq_product_external` (`external_id`),
  KEY `companyId` (`companyId`),
  KEY `typeId` (`typeId`),
  CONSTRAINT `Product_ibfk_1` FOREIGN KEY (`companyId`) REFERENCES `Company` (`id`),
  CONSTRAINT `Product_ibfk_2` FOREIGN KEY (`typeId`) REFERENCES `ProductType` (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=246 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

**Campos críticos para stock:**
- `stock`: Cantidad real disponible
- `reserved_stock`: Cantidad en proceso (reservada pero no confirmada)
- `hasStock`: Flag si el producto participa en control de stock (0/1)
- `Stockeable`: Flag si el producto puede estar en stock (0/1)

---

### 5.2 Tabla: `Orders`

```sql
CREATE TABLE `Orders` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `companyId` int NOT NULL DEFAULT '1',
  `firstName` varchar(100) NOT NULL,
  `lastName` varchar(100) NOT NULL,
  `email` varchar(150) NOT NULL,
  `phone` varchar(30) DEFAULT NULL,
  `address` varchar(255) DEFAULT NULL,
  `status` varchar(50) DEFAULT 'pending',
  `totalPrice` decimal(10,2) DEFAULT '0.00',
  `createdAt` datetime DEFAULT CURRENT_TIMESTAMP,
  `updatedAt` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_orders_companyId` (`companyId`),
  CONSTRAINT `fk_orders_company` FOREIGN KEY (`companyId`) REFERENCES `Company` (`id`) ON DELETE RESTRICT ON UPDATE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=230 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

**Estados soportados:**
- `PENDING`: Orden inicial, sin procesar aún
- `CREATED`: Orden confirmada con items reservados/confirmados
- `CANCELED`: Orden cancelada, stock liberado

---

### 5.3 Tabla: `OrderItems`

```sql
CREATE TABLE `OrderItems` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `orderId` int unsigned NOT NULL,
  `productId` int NOT NULL,
  `quantity` int DEFAULT '1',
  `price` decimal(10,2) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `fk_order_items_order` (`orderId`),
  KEY `fk_order_items_product` (`productId`),
  CONSTRAINT `fk_order_items_order` FOREIGN KEY (`orderId`) REFERENCES `Orders` (`id`) ON DELETE CASCADE ON UPDATE CASCADE,
  CONSTRAINT `fk_order_items_product` FOREIGN KEY (`productId`) REFERENCES `Product` (`id`) ON DELETE RESTRICT ON UPDATE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=37 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

**Notas:**
- No tiene columna `status` (se maneja a nivel Order)
- `price` guarda el precio al momento de la orden (referencia histórica)

---

### 5.4 Tabla: `Company`

```sql
CREATE TABLE `Company` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(255) NOT NULL,
  `email` varchar(255) NOT NULL,
  `country` varchar(255) DEFAULT NULL,
  `document` json DEFAULT NULL,
  `areaCode` varchar(10) DEFAULT NULL,
  `phoneNumber` varchar(20) DEFAULT NULL,
  `createdAt` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updatedAt` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `statusId` int NOT NULL,
  `suscriptionId` int NOT NULL,
  PRIMARY KEY (`id`),
  KEY `statusId` (`statusId`),
  KEY `suscriptionId` (`suscriptionId`),
  CONSTRAINT `Company_ibfk_1` FOREIGN KEY (`statusId`) REFERENCES `CompanyStatus` (`id`) ON DELETE CASCADE,
  CONSTRAINT `Company_ibfk_2` FOREIGN KEY (`suscriptionId`) REFERENCES `CompanySuscription` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=4 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

---

### 5.5 Tabla: `CompanyConfig`

```sql
CREATE TABLE `CompanyConfig` (
  `id` int NOT NULL AUTO_INCREMENT,
  `companyId` int NOT NULL,
  `fieldsOrderConfig` json NOT NULL,
  `hasStock` tinyint(1) NOT NULL DEFAULT '0',
  `createdAt` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updatedAt` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_company_config_company` (`companyId`),
  CONSTRAINT `CompanyConfig_ibfk_1` FOREIGN KEY (`companyId`) REFERENCES `Company` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

**Campos críticos:**
- `hasStock`: Flag global de control de stock por compañía (0/1)
- `fieldsOrderConfig`: JSON con campos requeridos para crear orden

---

### 5.6 Tablas de Soporte

#### `ProductType`

```sql
CREATE TABLE `ProductType` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(255) NOT NULL,
  `createdAt` datetime DEFAULT CURRENT_TIMESTAMP,
  `updatedAt` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name` (`name`)
) ENGINE=InnoDB AUTO_INCREMENT=6 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

#### `CompanyStatus`

```sql
CREATE TABLE `CompanyStatus` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(255) NOT NULL,
  `createdAt` datetime DEFAULT CURRENT_TIMESTAMP,
  `updatedAt` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `isActive` tinyint(1) DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `name` (`name`)
) ENGINE=InnoDB AUTO_INCREMENT=5 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

---

## 6. Flujo de n8n a Migrar

### 6.1 Descripción General del Workflow n8n V4

El workflow actual en n8n (`V4 WSP Distribuidora Ascenso.json`) implementa:

1. **Agente 1 (IA)**: Orquesta la conversación con cliente, maneja carrito, recopila datos
2. **Agente 2 (IA)**: Motor de búsqueda de productos en catálogo
3. **Nodo de Loop**: Itera sobre `order.orderItems` para:
   - Reservar stock por producto
   - Crear `OrderItems` en BD
   - Actualizar `reserved_stock`
4. **Nodo de Confirmación**: Descuenta stock real y confirma orden
5. **Nodo de Cancelación**: Libera reservas si falla algo

### 6.2 Nodo Crítico a Migrar: "Loop Product Items"

**Ubicación en JSON:** Línea ~1194
**Tipo:** `splitInBatches` (itera con batch size 10)

**Lógica actual:**

```
Para cada item en order.orderItems:
  1. Validar que product existe (Query SELECT)
  2. UPDATE reserved_stock += quantity WHERE productId, companyId
  3. INSERT INTO OrderItems (orderId, productId, quantity, price)
  4. Si falla → rastrear error
  5. Si éxito → marcar como procesado

Post-processing:
  - Si TODOS exitosos → UPDATE Orders.status = 'CREATED'
  - Si ALGUNOS fallan → UPDATE Orders.status = 'PENDING'
  - Si TODOS fallan → UPDATE Orders.status = 'CANCELLED'
```

### 6.3 Problemas Actuales en n8n

| Problema | Impacto | Solución |
|----------|--------|----------|
| Iteración secuencial | Latencia O(n) | Transacción única en BD |
| Sin transacción atómica | Inconsistencia si falla a mitad | ACID transaction |
| Sin manejo de deadlock | Reintentos manuales | Backoff exponencial automático |
| Búsqueda por nombre | Ambigüedad si hay duplicados | Búsqueda por `productId` |
| Lógica dispersa en nodos | Difícil mantener/testear | Centralizado en servicio |
| Sin idempotencia | Dobles reservas en reintentos | UNIQUE constraints + transacción |

---

## 7. Especificaciones Técnicas

### 7.1 Endpoint Único (MVP)

**POST /orders/{orderId}/reserve-and-add**

#### Request

```json
{
  "companyId": 12,
  "items": [
    {
      "productId": 101,
      "quantity": 5,
      "price": "10.50"
    },
    {
      "productId": 202,
      "quantity": 2,
      "price": "25.00"
    }
  ]
}
```

#### Response (Success - 200 OK)

```json
{
  "traceId": "550e8400-e29b-41d4-a716-446655440000",
  "orderId": "ORD-123",
  "status": "CONFIRMED",
  "totalPrice": "102.50",
  "addedItems": [101, 202],
  "successes": [
    {
      "productId": 101,
      "quantity": 5
    },
    {
      "productId": 202,
      "quantity": 2
    }
  ],
  "failures": [],
  "timestamp": "2025-02-12T10:30:45Z"
}
```

#### Response (Partial - 206 Partial Content)

```json
{
  "traceId": "550e8400-e29b-41d4-a716-446655440000",
  "orderId": "ORD-123",
  "status": "PENDING",
  "totalPrice": "52.50",
  "addedItems": [101],
  "successes": [
    {
      "productId": 101,
      "quantity": 5
    }
  ],
  "failures": [
    {
      "productId": 202,
      "quantity": 2,
      "reason": "INSUFFICIENT_AVAILABLE"
    }
  ],
  "timestamp": "2025-02-12T10:30:45Z"
}
```

#### Response (Failure - 422 Unprocessable Entity)

```json
{
  "traceId": "550e8400-e29b-41d4-a716-446655440000",
  "status": 422,
  "message": "No items could be reserved",
  "code": "NO_STOCK_AVAILABLE",
  "orderId": "ORD-123",
  "details": {
    "failures": [
      {
        "productId": 101,
        "quantity": 5,
        "reason": "INSUFFICIENT_AVAILABLE"
      },
      {
        "productId": 202,
        "quantity": 2,
        "reason": "OUT_OF_STOCK"
      }
    ]
  },
  "timestamp": "2025-02-12T10:30:45Z"
}
```

### 7.2 Códigos de Razón de Fallo

| Código | Descripción | HTTP Status |
|--------|-------------|------------|
| `NOT_FOUND` | Producto no existe | 404 |
| `OUT_OF_STOCK` | Stock en 0 | 422 |
| `INSUFFICIENT_AVAILABLE` | (stock - reserved_stock) < qty solicitada | 422 |
| `INVALID_QUANTITY` | qty <= 0 o > límite | 400 |
| `WRONG_COMPANY` | productId no pertenece a companyId | 403 |
| `PRODUCT_INACTIVE` | Producto desactivado | 422 |
| `COMPANY_INVALID` | companyId no existe | 401 |

---

## 8. Requerimientos Funcionales

### 8.1 Parcialidad de Órdenes

**RF-1: Reserva Selectiva**
- Dada una lista de N ítems, reservar solo aquellos con disponibilidad
- Retornar `successes[]` y `failures[]` con códigos de razón específicos
- NO cancelar toda la orden si algunos ítems fallan

**RF-2: Estados de Orden**
- `PENDING`: Creada para procesar, sin items aún
- `CREATED`: Completada totalmente o parcialmente, lista para confirmar
- `CANCELED`: Cancelada, stock reservado liberado

### 8.2 Claves y Validaciones

**RF-3: Scoping por companyId**
- validar `productId` pertenece a `companyId` solicitado
- Rechazar con `WRONG_COMPANY` si no coincide
- Validar que `companyId` tiene habilitado control de stock

**RF-4: Validaciones Básicas**
- `quantity > 0` y `<= 10000` (límite anti-abuso)
- `productId` requerido (entero positivo)
- `companyId` obligatorio (entero positivo)
- `price >= 0` (validación de coherencia)

### 8.3 Flujo de Vida del Stock

**RF-5: Paso 1 - Reserve-and-Add**
```
Para cada item exitoso:
  1. Incrementar reserved_stock += qty
  2. Crear OrderItem(orderId, productId, qty, price)
  3. Actualizar orden.status = CREATED o PENDING
```

**RF-6: Paso 2 - Confirm (Futuro)**
```
Descontar stock real y confirmar:
  1. Decrementar stock -= qty
  2. Decrementar reserved_stock -= qty
  3. Marcar OrderItems como confirmed
  4. Actualizar orden.status = CONFIRMED
```

**RF-7: Paso 3 - Cancel (Futuro)**
```
Liberar reservas:
  1. Decrementar reserved_stock -= qty
  2. Marcar OrderItems como canceled
  3. Actualizar orden.status = CANCELED
```

### 8.4 Idempotencia

**RF-8: Evitar Dobles Reservas**
- Usar `UNIQUE(orderId, productId)` en `OrderItems`
- Si reintentan con mismo `orderId + productId`:
  - Si `qty` idéntica → retornar 200 (ya reservado)
  - Si `qty` diferente → 409 Conflict (no permitir cambio)

### 8.5 Trazabilidad

**RF-9: Audit Log**
- Registrar en logs: `orderId`, `productId`, `companyId`, `action`, `result`
- Incluir `traceId` en cada operación para correlacionar
- Permitir auditar qué se reservó/confirmó/canceló por orden

---

## 9. Requerimientos No-Funcionales

### 9.1 Concurrencia

**RNF-1: Sin Overselling**
- Múltiples órdenes compitiendo por mismo producto → ninguna le "roba" stock a otra
- Implementar con `SELECT FOR UPDATE` sobre `Product`
- Garantía: `(stock - reserved_stock)` nunca negativo

**RNF-2: Transacciones Atómicas**
- BEGIN...COMMIT en BD = todo o nada por item
- REPEATABLE READ aislamiento para evitar phantom reads
- Timeout de transacción: 5 segundos máximo

### 9.2 Latencia

**RNF-3: Baja Latencia**
- p50: < 100ms
- p95: < 500ms
- p99: < 2s
- Implementar con: transacciones cortas, prepared statements, connection pooling

**RNF-4: Optimización BD**
- máximo 1 query SELECT por producto (con FOR UPDATE)
- máximo 1 query INSERT por OrderItem
- máximo 1 UPDATE de Order (status)
- conexiones preparadas para queries frecuentes

### 9.3 Robustez

**RNF-5: Manejo de Deadlocks**
- Detección: `ER_LOCK_DEADLOCK (1213)` o `ER_LOCK_WAIT_TIMEOUT (1205)`
- Reintento: backoff exponencial con jitter
  - Intento 1: inmediato
  - Intento 2: 100ms + jitter (±20%)
  - Intento 3: 200ms + jitter
  - Máx 3 intentos total
- Logging: cada reintento documentado

**RNF-6: Reintentos Ante Fallos**
- Network errors (timeouts) → automático hasta 3 veces
- DB connection failure → circuit breaker después de N fallos
- No reintentar errores de validación (400, 403, 404, 422)

### 9.4 Observabilidad

**RNF-7: Logs Estructurados**
- Formato JSON en producción
- Campos obligatorios: `timestamp`, `level`, `traceId`, `orderId`, `companyId`, `message`
- Niveles: DEBUG, INFO, WARN, ERROR

**RNF-8: Métricas Prometheus**
- `order_service_transactions_total{status="success|partial|failure"}`
- `order_service_transaction_duration_seconds` (histogram)
- `order_service_stock_reserved_total{productId, companyId}`
- `order_service_deadlock_retries_total`
- `db_connection_pool_open_connections`, `idle_connections`

**RNF-9: Trazas Distribuidas**
- OpenTelemetry traces con span hierarchy
- Instrumentar: validación, transacción DB, commit/rollback
- Exportar a gestor de trazas (ej: Jaeger, Datadog)

### 9.5 Seguridad

**RNF-10: Autenticación**
- Validar `Authorization: Bearer <token>` O `X-API-Key: <key>`
- Extraer `companyId` del token, validar que coincida con request
- Rechazar 401 Unauthorized si token inválido/expirado

**RNF-11: Autorización**
- Validar que `companyId` en token puede operar sobre orden solicitada
- Evitar que cliente A vea/modifique órdenes de cliente B

**RNF-12: Rate Limiting**
- Opcional en MVP, preparado para implementar
- Límite por API Key: ej. 100 requests/minuto

### 9.6 Escalabilidad

**RNF-13: Pool de Conexiones BD**
- `MaxOpenConns = 25` (ajustable)
- `MaxIdleConns = 5`
- `ConnMaxLifetime = 5 minutos`
- `ConnMaxIdleTime = 2 minutos`

**RNF-14: Preparación para Crecimiento**
- Código lista para migrarse a serverless (Cloud Run, Lambda) sin reescrituras
- Stateless = múltiples instancias paralelas
- Contenedor Docker minimal (~50MB)

---

## 10. API Propuesta

### 10.1 POST /orders/{orderId}/reserve-and-add

**Path Parameters:**
- `orderId` (string, requerido): ID de orden pre-generado

**Query Parameters:**
- Ninguno requerido

**Request Headers:**
- `Authorization: Bearer <token>` o `X-API-Key: <key>` (requerido)
- `Content-Type: application/json`
- `Idempotency-Key: <uuid>` (opcional, para idempotencia exacta)

**Request Body:**
```json
{
  "companyId": 12,
  "items": [
    {
      "productId": 101,
      "quantity": 5,
      "price": "10.50"
    }
  ]
}
```

**Response Codes:**
- `200 OK`: Todas las items procesadas exitosamente
- `206 Partial Content`: Algunas items fallaron, otras OK
- `400 Bad Request`: Validación de input fallida
- `401 Unauthorized`: Token inválido
- `403 Forbidden`: Sin permiso en company
- `404 Not Found`: Orden no existe
- `409 Conflict`: Deadlock / Race condition, reintentar
- `422 Unprocessable Entity`: Ningún item se pudo procesar
- `500 Internal Server Error`: Error en servidor
- `503 Service Unavailable`: BD no disponible

### 10.2 Autenticación

**Opción A: Bearer Token (JWT)**
```
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
// Payload: { "sub": "user-123", "companyId": 12, "iat": ..., "exp": ... }
```

**Opción B: API Key**
```
X-API-Key: sk_live_1234567890abcdef
// Validar contra tabla de keys registradas, obtener companyId asociada
```

### 10.3 Error Model

```json
{
  "traceId": "550e8400-e29b-41d4-a716-446655440000",
  "status": 422,
  "message": "No items could be reserved",
  "code": "NO_STOCK_AVAILABLE",
  "orderId": "ORD-123",
  "details": {
    "failures": [
      {
        "productId": 101,
        "quantity": 5,
        "reason": "INSUFFICIENT_AVAILABLE"
      }
    ]
  },
  "timestamp": "2025-02-12T10:30:45Z",
  "retryable": true
}
```

---

## 11. Modelo de Stock y Semántica

### 11.1 Dos Columnas: stock vs reserved_stock

```
Product:
  stock = 100           # Cantidad real disponible en almacén
  reserved_stock = 15   # Cantidad reservada (en órdenes pendientes)

Disponibilidad = stock - reserved_stock = 85
```

### 11.2 Operaciones sobre Stock

#### 11.2.1 Reserva (Reserve-and-Add)

```sql
UPDATE Product
SET reserved_stock = reserved_stock + :qty
WHERE id = :productId
  AND companyId = :companyId
  AND (stock - reserved_stock) >= :qty  -- Validación
  AND isActive = 1
  AND isDeleted = 0;
```

**Efecto:**
- `reserved_stock` aumenta
- `stock` NO cambia (aún)
- Disponibilidad disminuye

**Ejemplos:**
```
Antes:  stock=100, reserved_stock=15  (disponible=85)
Reservar 10 unidades:
Después: stock=100, reserved_stock=25  (disponible=75)
```

#### 11.2.2 Confirmación (Confirm - Futuro)

```sql
UPDATE Product
SET stock = stock - :qty,
    reserved_stock = reserved_stock - :qty
WHERE id = :productId
  AND companyId = :companyId;
```

**Efecto:**
- `stock` disminuye (sale físicamente del almacén)
- `reserved_stock` disminuye (se "consume" la reserva)
- Disponibilidad disminuye en 2X

**Ejemplos:**
```
Antes:  stock=100, reserved_stock=25  (disponible=75)
Confirmar orden de 10 (que estaban reservadas):
Después: stock=90, reserved_stock=15   (disponible=75) ✓ Consistente
```

#### 11.2.3 Cancelación (Cancel - Futuro)

```sql
UPDATE Product
SET reserved_stock = reserved_stock - :qty
WHERE id = :productId
  AND companyId = :companyId;
```

**Efecto:**
- `reserved_stock` disminuye
- `stock` NO cambia
- Disponibilidad aumenta

**Ejemplos:**
```
Antes:  stock=100, reserved_stock=25  (disponible=75)
Cancelar orden de 10 (que estaban reservadas):
Después: stock=100, reserved_stock=15  (disponible=85) ✓ Stock recuperado
```

### 11.3 Por Qué Mantener reserved_stock

1. ✅ **Evita overselling**: Marca clara de stock "prometido pero no consumido"
2. ✅ **Visibilidad**: Distingue entre "tengo" y "tengo disponible"
3. ✅ **Orquestación segura**: Permite pasos 1 (reservar) → 2 (confirmar) sin ambigüedad
4. ✅ **Recuperación ante fallos**: Cancelar libera sin afectar stock real
5. ✅ **Auditoría**: Deja rastro claro de qué fue reservado vs confirmado

---

## 12. Manejo de Concurrencia

### 12.1 Escenario de Concurrencia

```
Producto: id=101, stock=100, reserved_stock=0 (disponible=100)

Orden A: Solicita reservar 40 unidades
Orden B: Solicita reservar 40 unidades
Orden C: Solicita reservar 40 unidades              ✓ Simultáneamente
(Ejecutadas aproximadamente al mismo tiempo)

Sin control:
  A: UPDATE ... reserved_stock = 40
  B: UPDATE ... reserved_stock = 80
  C: UPDATE ... reserved_stock = 120  ← OVERSELLING! (120 > 100)

Con SELECT FOR UPDATE + WHERE condicional:
  A: SELECT stock, reserved_stock FOR UPDATE → (100, 0)
     WHERE (100-0) >= 40 ✓ → UPDATE reserved_stock = 40

  B: SELECT FOR UPDATE (espera a A) → (100, 40)
     WHERE (100-40) >= 40 ✓ → UPDATE reserved_stock = 80

  C: SELECT FOR UPDATE (espera a B) → (100, 80)
     WHERE (100-80) >= 40 ✗ → ROLLBACK, fail con INSUFFICIENT_AVAILABLE
```

### 12.2 Estrategia Anti-Deadlock

**Problema:** Si dos órdenes tocan productos en orden diferente → deadlock

```
Orden 1: UPDATE Product WHERE id=A, entonces id=B
Orden 2: UPDATE Product WHERE id=B, entonces id=A
→ DEADLOCK
```

**Solución:** Siempre ordenar items por `productId ASC`

```
Orden 1: UPDATE Product WHERE id=1, luego id=2
Orden 2: UPDATE Product WHERE id=1, luego id=2
→ NO DEADLOCK (mismo orden)
```

### 12.3 Transacción Atómica

```
BEGIN TRANSACTION
  SET TRANSACTION ISOLATION LEVEL REPEATABLE READ

  FOR cada item en items (ordenado por productId ASC):
    SELECT stock, reserved_stock
    FROM Product
    WHERE id = :productId FOR UPDATE

    IF (stock - reserved_stock) >= qty THEN
      UPDATE Product SET reserved_stock = reserved_stock + qty
      INSERT INTO OrderItem (...)
      ADD to successes[]
    ELSE
      ADD to failures[]
    ENDIF

  IF successes.count > 0 THEN
    UPDATE Orders SET status = CREATED
    COMMIT
  ELSE
    ROLLBACK
    Return 422 error
  ENDIF
```

### 12.4 Retry Strategy

```
max_attempts = 3
backoff_ms = [0, 100, 200]

for attempt in 1..max_attempts:
  try:
    execute_transaction()
    return success

  catch ER_LOCK_DEADLOCK(1213) or ER_LOCK_WAIT_TIMEOUT(1205):
    if attempt < max_attempts:
      jitter = random(-20%, +20%) of backoff_ms[attempt]
      sleep(backoff_ms[attempt] + jitter)
      continue
    else:
      return 409 Conflict

  catch other_error:
    return appropriate_error_code
```

---

## 13. Validaciones y Reglas de Negocio

### 13.1 Validaciones de Input

| Campo | Regla | Error |
|-------|-------|-------|
| `orderId` | UUID válido, existe en BD | 404 Not Found |
| `companyId` | Entero > 0, existe en BD | 400 Bad Request |
| `items` | Array no vacío, max 100 elementos | 400 Bad Request |
| `productId` (item) | Entero > 0, existe y activo | 404 Not Found |
| `quantity` (item) | Entero, 1 <= qty <= 10000 | 400 Bad Request |
| `price` (item) | Decimal >= 0 | 400 Bad Request |

### 13.2 Validaciones de Negocio

| Regla | Condición | Acción |
|-------|-----------|--------|
| Scoping por company | `Product.companyId != companyId_request` | WRONG_COMPANY (403) |
| Stock actual | `(stock - reserved_stock) < qty` | INSUFFICIENT_AVAILABLE (422) |
| Control de stock | `CompanyConfig.hasStock = 0` | Permitir (sin validar stock) |
| Producto activo | `Product.isActive != 1` | PRODUCT_INACTIVE (422) |
| Producto no borrado | `Product.isDeleted = 1` | PRODUCT_INACTIVE (422) |
| Orden existe | `Order.id != orderId` | 404 Not Found |
| Orden status | `Order.status != PENDING` | 409 Conflict (no se puede procesar) |
| Sin duplicados | `items` con mismo `productId` 2X | Agrupar cantidades o rechazar 400 |

### 13.3 Limitaciones

- **Cantidad máxima por ítem**: 10,000 unidades (anti-abuso)
- **Máximo items por orden**: 100 productos distintos (anti-abuso)
- **Timeout transacción**: 5 segundos
- **Timeout conexión BD**: 3 segundos
- **TTL de reserva**: Sin expiración (opcional para futuro)

---

## 14. Observabilidad

### 14.1 Logs Estructurados (Zap)

**Formato JSON en producción:**
```json
{
  "timestamp": "2025-02-12T10:30:45.123Z",
  "level": "INFO",
  "logger": "order-service",
  "traceId": "550e8400-e29b-41d4-a716-446655440000",
  "orderId": "ORD-123",
  "companyId": 12,
  "action": "reserve_stock",
  "productId": 101,
  "quantity": 5,
  "result": "success",
  "duration_ms": 45,
  "message": "Stock reserved successfully"
}
```

**Niveles y contextos:**

| Nivel | Contexto | Ejemplo |
|-------|----------|---------|
| DEBUG | Desarrollo, detalles internos | "Validating input", "DB connection acquired" |
| INFO | Eventos importantes normales | "Order processing started", "Stock reserved" |
| WARN | Situaciones atípicas, recuperables | "Deadlock detected, retrying", "Partial failure" |
| ERROR | Fallos que requieren atención | "Transaction failed", "DB connection error" |

### 14.2 Métricas Prometheus

```
# Total de transacciones por estado
order_service_transactions_total{status="success|partial|failure|error"}

# Duración de transacciones (segundos)
order_service_transaction_duration_seconds{quantile="0.5|0.95|0.99"}

# Items procesados (exitosos + fallidos)
order_service_items_processed_total{result="success|failure", reason="..."}

# Stock reservado por producto
order_service_stock_reserved_total{productId, companyId}

# Deadlocks y reintentos
order_service_deadlock_retries_total{attempt="1|2|3"}

# Estado de conexión BD
db_connection_pool_open_connections
db_connection_pool_idle_connections
db_connection_pool_checkout_failures_total
```

### 14.3 Trazas Distribuidas (OpenTelemetry)

**Span hierarchy:**

```
ProcessOrder (root)
  ├─ ValidateInputs
  │   └─ CheckOrderExists
  ├─ LoadOrderData
  │   ├─ FetchCompanyConfig
  │   └─ FetchProductData
  ├─ BeginTransaction
  ├─ ProcessItem[0]
  │   ├─ SelectForUpdate (productId=101)
  │   ├─ ValidateStock
  │   ├─ UpdateReservedStock
  │   └─ InsertOrderItem
  ├─ ProcessItem[1]
  │   ├─ SelectForUpdate (productId=202)
  │   ├─ ValidateStock
  │   ├─ UpdateReservedStock
  │   └─ InsertOrderItem
  ├─ CommitTransaction
  ├─ UpdateOrderStatus
  └─ BuildResponse
```

---

## 15. Arquitectura Propuesta

### 15.1 Capas

```
┌─────────────────────────────┐
│   HTTP Handler (chi router) │  ← Enrutamiento, middleware
└──────────────┬──────────────┘
               │
┌──────────────▼──────────────┐
│   Order Service             │  ← Orquestación de transacción
│   (Lógica de negocio)       │
└──────────────┬──────────────┘
               │
┌──────────────▼──────────────┐
│   Repository Layer          │  ← Queries a BD
│   (Product, Order, Item)    │
└──────────────┬──────────────┘
               │
┌──────────────▼──────────────┐
│   Database (MySQL)          │  ← ACID transactions
└─────────────────────────────┘
```

### 15.2 Flujo de Requisición

```
1. Cliente HTTP POST /orders/{orderId}/reserve-and-add
            ↓
2. Middleware
   ├─ Extract traceId (injectiontar en contexto)
   ├─ Validate Authorization
   └─ Log request start
            ↓
3. Handler: reserve_and_add()
   ├─ Parse request body
   ├─ Validate basic schema
   └─ Call Service
            ↓
4. Service: ReserveAndAdd()
   ├─ Validate inputs (qty ranges, duplicates, etc)
   ├─ Load order + company config
   ├─ Order items by productId ASC
   ├─ Initiate transaction → OrderService.ExecuteTransaction()
   │  ├─ FOR each item:
   │  │  ├─ SELECT FOR UPDATE
   │  │  ├─ Validate stock available
   │  │  ├─ UPDATE reserved_stock
   │  │  ├─ INSERT OrderItem
   │  └─ COMMIT or ROLLBACK
   ├─ Update order status
   ├─ Collect response data
   └─ Return result
            ↓
5. Handler: Build JSON response
   ├─ HTTP 200 (all success)
   ├─ HTTP 206 (partial)
   └─ HTTP 422 (none)
            ↓
6. Middleware
   ├─ Log response + duration
   └─ Inject traceId header
            ↓
7. Client receives response
```

### 15.3 Estructura de Carpetas

```
saruman/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── config/
│   ├── database/
│   ├── domain/
│   ├── dto/
│   ├── handler/
│   ├── repository/
│   ├── service/
│   ├── observability/
│   └── common/
├── test/
├── docker-compose.yml
├── Dockerfile
├── go.mod
├── .env.example
└── README.md
```

---

## 16. Stack Tecnológico

### 16.1 Runtime

- **Go**: 1.22+
- **MySQL**: 8.0+
- **Docker**: Multistage build, tamaño ~50MB

### 16.2 Librerías Core

| Librería | Versión | Propósito |
|----------|---------|----------|
| `go-chi/chi` | v5.0.11+ | Router HTTP |
| `go-sql-driver/mysql` | v1.7.1+ | Driver MySQL |
| `zap` | v1.26.0+ | Logging estructurado |
| `viper` | v1.17.0+ | Configuration |
| `google/uuid` | v1.5.0+ | UUID generation |

### 16.3 Librerías Observabilidad

| Librería | Propósito |
|----------|----------|
| `go.opentelemetry.io/otel` | Trazas distribuidas |
| `go.opentelemetry.io/otel/exporters/otlp/otlptracehttp` | Exporter OTEL |
| `prometheus/client_golang` | Métricas Prometheus |

### 16.4 Librerías Testing

| Librería | Propósito |
|----------|----------|
| `testify` | Assertions + mocks |
| `testcontainers-go` | Containers para tests |

### 16.5 Tooling

| Tool | Propósito |
|------|----------|
| `golang-migrate/migrate` | Migraciones BD (futuro) |
| `sqlc` | SQL generation tipado (futuro) |
| `golangci-lint` | Linting |
| `make` | Build automation |

---

## 📝 Resumen Ejecutivo

### Problema
n8n no soporta lógica robusta de gestión de stock bajo concurrencia.

### Solución
Microservicio independiente en Go con transacciones atómicas, manejo de deadlocks, y API REST.

### Beneficios
- ✅ Órdenes parciales (no all-or-nothing)
- ✅ Stock seguro bajo concurrencia
- ✅ Latencia predecible
- ✅ Observabilidad integrada
- ✅ Testeable y mantenible

### Timeline Propuesto
1. **MVP (POC)**: GET /products endpoint simple
2. **Spec-driven development**: Iterar sobre módulos core
3. **Implementación**: Service → Repository → Handler
4. **Testing**: Unit + Integration
5. **Deploy**: Docker + CI/CD

### Contacto y Preguntas
Documento versionado. Cambios requieren revisión y aprobación.

---

**Última versión:** 2025-02-12
**Estado:** En Planificación → MVP → Desarrollo
