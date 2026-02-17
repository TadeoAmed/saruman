# Price Handling Fix — Reserve-and-Add Flow — Spec

## Contexto de Negocio

The reserve-and-add endpoint allows a client to reserve stock and add items to an existing
order in a single atomic operation. During the reservation, each product's price must be
authoritative: the canonical price is the one stored in the product catalog in the database,
not anything the client sends.

The current implementation trusts the client entirely for per-item pricing. A caller can
send any numeric value as `price` and that value is stored verbatim in the order item
record, and it is also used to compute the order's `totalPrice`. This means:

- A client can manipulate the price of any order item.
- The stored `totalPrice` on the order does not reflect real catalog prices.
- The `product.Price` already fetched from the database (under a `FOR UPDATE` lock) is
  silently discarded.

Fixing this removes the price attack surface and ensures order financials are always derived
from catalog data.

Stakeholders affected: any system that reads `OrderItems.price` or `Orders.totalPrice` to
make financial decisions (billing, reporting, downstream ERP).

---

## Requisitos Funcionales

**RF-01 — Remove per-item price from request.**
The request body must no longer accept a `price` field at the item level
(`ReserveAndAddItem`). Sending a `price` field in an item must have no effect on the stored
price; the field must be ignored entirely or rejected.

**RF-02 — Add client-provided totalPrice at the order level.**
The request body must accept an optional `totalPrice` field at the order level
(`ReserveAndAddRequest`). This field represents what the client believes the order total
should be and is used only for an integrity check (RF-07); it does not override the
recalculated total.

**RF-03 — Use the database product price as the unit price for each line item.**
When inserting an `OrderItem` record, the `price` column must be set to
`product.Price * quantity` (the line-item total). `product.Price` is the value returned by
`FindByIDForUpdate`, which is already executed inside the transaction with a `FOR UPDATE`
lock. No additional database query is required.

**RF-04 — Accumulate order totalPrice exclusively from DB-derived line-item prices.**
The service must compute `totalPrice` by summing `product.Price * quantity` for every item
that was successfully reserved. Items that failed reservation must not contribute to the
total.

**RF-05 — Write the recalculated totalPrice to the order record.**
When at least one item is reserved successfully, the service must call
`UpdateTotalPrice` with the recalculated total, exactly as today, but the value passed must
be derived from DB prices (RF-04), never from client input.

**RF-06 — Return the recalculated totalPrice in the response.**
The `ReservationResult.TotalPrice` returned by the service and surfaced in the HTTP response
must reflect the recalculated DB-derived total.

**RF-07 — Validate client totalPrice against recalculated totalPrice (optional integrity check).**
If the client supplies `totalPrice` in the request, the system must compare it against the
recalculated total. If the values differ by more than an acceptable floating-point tolerance
(0.01 units), the system must return a validation error and reject the request before any
reservation is committed. If the client omits `totalPrice`, the check is skipped and the
reservation proceeds normally.

**RF-08 — Preserve all existing item-level validations.**
All current validations that do not concern price (`productId`, `quantity`, `companyId`,
`orderId`, duplicate detection, items count) must remain unchanged.

**RF-09 — All-failed result carries no totalPrice.**
When every item fails reservation, `Orders.totalPrice` must not be updated and the response
`totalPrice` must be zero (or absent), exactly as today.

---

## Comportamiento Esperado

### Flujo principal (happy path — todos los items reservados)

Given a valid `POST /orders/{orderId}/reserve-and-add` request where:
- `companyId` matches the order's company
- the order is in `PENDING` status
- all requested items exist, are active, and have sufficient stock
- the client omits `totalPrice` or provides a value within tolerance of the recalculated total

When the request is processed:
- Each item is fetched from the database under a `FOR UPDATE` lock.
- `OrderItems.price` is set to `product.Price * quantity` for each item.
- The service accumulates `totalPrice = sum(product.Price * quantity)` for successful items.
- `Orders.status` is updated to `CREATED`.
- `Orders.totalPrice` is updated to the recalculated total.
- The transaction is committed.

Then the response is HTTP 200 with `status = ALL_SUCCESS`, `totalPrice` equal to the
recalculated total, all items in `successes`, and `failures` empty.

### Flujo parcial (algunos items fallan)

Given the same preconditions but one or more items are out of stock, inactive, or not found:

When the request is processed:
- Successfully reserved items get `OrderItems.price = product.Price * quantity`.
- `totalPrice` is recalculated from the successfully reserved items only.
- `Orders.status` is updated to `CREATED`, `Orders.totalPrice` is updated.
- The transaction is committed.

Then the response is HTTP 206 with `status = PARTIAL`, `totalPrice` reflecting only the
items that succeeded, failed items in `failures`.

### Flujo de fallo total (ningún item reservado)

Given all items fail validation (not found, inactive, insufficient stock):

When the request is processed:
- No `OrderItems` are inserted.
- `Orders.totalPrice` is NOT updated.
- The transaction is rolled back.

Then the response is HTTP 422 with `status = ALL_FAILED` and `totalPrice = 0`.

### Verificación de totalPrice del cliente (RF-07)

Given a request where the client provides `totalPrice`:

When the service has recalculated the actual total from DB prices:
- If `|client.totalPrice - recalculated| <= 0.01`: proceed normally, commit.
- If `|client.totalPrice - recalculated| > 0.01`: roll back, return HTTP 422 with error code
  `PRICE_MISMATCH` and a message indicating the discrepancy.

The integrity check must be performed after all individual item reservations are attempted
but before the transaction is committed.

### Campos price en el request de items

Given a request where an item contains a `price` field in the JSON body:

The system must ignore the field entirely. It must not be parsed into any internal struct
that propagates to the service or repository. No validation error is returned for its
presence; the field is simply not consumed.

---

## Criterios de Aceptación

**CA-01** — A request with items that include a `price` field results in the same stored
`OrderItems.price` as the same request without a `price` field. The client-supplied price
has zero effect on the stored value.

**CA-02** — `OrderItems.price` stored in the database equals `product.Price * quantity`
for each successfully reserved item, where `product.Price` is the value in the products
table at the time of reservation.

**CA-03** — `Orders.totalPrice` stored in the database equals the sum of all
`OrderItems.price` values for the successfully reserved items of that order.

**CA-04** — The `totalPrice` field in the HTTP response equals the value written to
`Orders.totalPrice`.

**CA-05** — When only a subset of items succeeds (PARTIAL), `Orders.totalPrice` and the
response `totalPrice` reflect only the successful items.

**CA-06** — When all items fail (ALL_FAILED), `Orders.totalPrice` is not modified and the
response `totalPrice` is 0.

**CA-07** — A request omitting `totalPrice` at the order level is accepted and processed
normally without any price mismatch check.

**CA-08** — A request providing a `totalPrice` at the order level that matches the
recalculated total within 0.01 is accepted and the order is committed.

**CA-09** — A request providing a `totalPrice` at the order level that differs from the
recalculated total by more than 0.01 is rejected with HTTP 422 and error code
`PRICE_MISMATCH`. No `OrderItems` are persisted and `Orders.totalPrice` is not changed.

**CA-10** — All existing validations for `orderId`, `companyId`, `items[*].productId`,
`items[*].quantity`, duplicate product IDs, and items count continue to behave exactly as
before.

---

## Entidades y Datos Involucrados

### Product (domain entity, read-only in this flow)
- `ID` — identifies the product.
- `Price` — the authoritative unit price sourced from the products table. This is the sole
  source of truth for pricing in the reservation flow.
- `IsActive`, `HasStock`, `Stockeable`, `Stock`, `ReservedStock` — used for eligibility and
  stock checks (unchanged).

### OrderItem (domain entity, written in this flow)
- `OrderID` — foreign key to the order.
- `ProductID` — foreign key to the product.
- `Quantity` — number of units reserved.
- `Price` — line-item total stored as `product.Price * quantity`. Previously this field
  stored the client-supplied per-unit price; after this fix it stores the DB-derived
  line-item total.

### Order (domain entity, updated in this flow)
- `TotalPrice` — recalculated as the sum of all successfully reserved `OrderItem.Price`
  values. Client input for `totalPrice` must never overwrite this recalculated value
  directly.

### ReserveAndAddRequest (request contract)
- `companyId` (int, required) — unchanged.
- `items` (array, required) — unchanged except that each item no longer carries `price`.
- `totalPrice` (float64, optional) — new field. If present, used only for integrity check
  (RF-07).

### ReserveAndAddItem (item in the request)
- `productId` (int, required) — unchanged.
- `quantity` (int, required) — unchanged.
- `price` — REMOVED. Must not be present in the internal contract or flow.

### ReservationItem (internal DTO passed from controller → use case → service)
- `ProductID` (int) — unchanged.
- `Quantity` (int) — unchanged.
- `Price` — REMOVED. The internal DTO must not carry a client-supplied price.

### ReservationResult (internal DTO returned from service → use case → controller)
- `Status` — unchanged.
- `OrderID` — unchanged.
- `TotalPrice` — now always DB-derived; unchanged in type and position.
- `Successes`, `Failures` — unchanged.

---

## Fuera de Alcance

- Changing how `product.Price` is fetched from the database. The existing
  `FindByIDForUpdate` query already returns `product.Price`; no new query or repository
  method is needed.
- Retroactively correcting `OrderItems.price` or `Orders.totalPrice` for orders created
  before this fix.
- Currency handling, rounding rules beyond the 0.01 tolerance for the integrity check, or
  multi-currency support.
- Exposing a separate endpoint to recalculate prices on existing orders.
- Changing the `ItemSuccessDTO` or `ItemFailureDTO` response shapes (no per-item price is
  added to the response).
- Any change to the stock reservation logic, deadlock retry, or transaction isolation level.
- Authentication or authorization changes.

---

## Dudas que debe responder Tadeo

A continuación se detalla información que requiere aprobación explícita antes de proceder a la implementación:

### P1: Tolerancia para la validación de totalPrice

**Pregunta:** ¿Cuál debe ser la tolerancia aceptable al validar el `totalPrice` enviado por el cliente
contra el `totalPrice` recalculado del servidor?

**Opciones:**
1. **Tolerancia de 0.01** (recomendado): Acepta diferencias hasta 0.01 unidades. Útil para
   manejar errores de redondeo en aritmética de punto flotante en sistemas clientes. Los precios
   `DECIMAL(10,2)` pueden generar pequeñas diferencias acumuladas.
2. **Tolerancia de 0.00 (exacta)**: Solo acepta match exacto (`client.totalPrice == recalculated`).
   Más estricto, adecuado si el cliente siempre calcula del mismo modo que el servidor.

**Impacto:**
- Tolerancia 0.01: Menos falsos positivos (PRICE_MISMATCH), más resiliente a arredondeos menores.
- Tolerancia 0.00: Más restrictivo, detecta cualquier desajuste por pequeño que sea.

**Decision Tadeo:** _______________________

---

### P2: Estructura del precio por item en el contrato

**Pregunta:** ¿Cómo el cliente debe comunicar el precio de cada item?

**Opciones:**

**Opción A — Eliminar completamente el precio del contrato (actual en el spec):**
```json
{
  "companyId": 1,
  "totalPrice": 100.00,
  "items": [
    { "productId": 5, "quantity": 2 },
    { "productId": 10, "quantity": 1 }
  ]
}
```
- **Ventaja:** Máxima seguridad. El cliente no puede manipular precios unitarios ni por línea.
  El servidor obtiene todos los precios de la BD.
- **Desventaja:** El cliente no envía información sobre precios. Si el cliente quiere validar
  precios antes de confirmar, no puede hacerlo en el mismo request.

**Opción B — Recibir y validar un `itemTotalPrice` por item:**
```json
{
  "companyId": 1,
  "totalPrice": 100.00,
  "items": [
    { "productId": 5, "quantity": 2, "itemTotalPrice": 40.00 },
    { "productId": 10, "quantity": 1, "itemTotalPrice": 60.00 }
  ]
}
```
- **Ventaja:** El cliente envía su cálculo por item. El servidor valida que cada
  `itemTotalPrice == product.Price * quantity`. Detecta inconsistencias por item.
- **Desventaja:** Más validación, más banda ancha, el cliente debe calcular precios.
  Riesgo si el cliente envía `itemTotalPrice` incorrecto (se rechaza el request completo).

**Flujo esperado si elegimos Opción B:**
1. Cliente calcula para cada item: `itemTotalPrice = expectedUnitPrice * quantity`
2. Cliente suma: `totalPrice = sum(itemTotalPrice)`
3. Servidor recibe y por cada item:
   - Obtiene `product.Price` de la BD
   - Calcula `expectedItemTotal = product.Price * quantity`
   - Valida: `itemTotalPrice == expectedItemTotal` (con tolerancia 0.01)
   - Si no coincide: rechaza el request con error `ITEM_PRICE_MISMATCH`
4. Después de todas las validaciones, suma lo recalculado para confirmar `totalPrice`

**Recomendación:** Opción A (eliminar precio completamente) es más segura. Opción B es más informativa
pero agrega complejidad y puntos de fallo. Elegir según si es crítico que el cliente valide precios
en el mismo request.

**Decision Tadeo:** _______________________

---
