# SQL Repository Expert — Project Memory

## Project: Saruman (Vincula Latam)
- Go microservice, MySQL, `database/sql` puro (no ORM)
- Driver: `github.com/go-sql-driver/mysql`
- Connection pool configured in `internal/infrastructure/mysql/connection.go`

## Schema conventions
- Column names are camelCase mixed in MySQL (companyId, isDeleted, createdAt, hasStock, Stockeable)
- Soft deletes via `isDeleted = 0` filter — applies to `Product` table, NOT to `Orders` (no such column)
- Nullable columns map to Go pointers (*int, *string, etc.)
- `Product.stock` and `Product.reserved_stock` are nullable (*int in Go)

## Key tables identified
- `Product`: has `id` (PK), `companyId`, `isDeleted`, `isActive`, `hasStock`, `Stockeable`, `stock`, `reserved_stock`
- `Orders`: has `id` (PK), `companyId`, `status` ('PENDING'/'CREATED'/'CANCELED'), `totalPrice` — no `isDeleted`
- `OrderItems`: has FK to Orders(id) with ON DELETE CASCADE
- `CompanyConfig`: has `companyId` (UNIQUE), `hasStock` (tinyint bool)

## Repository patterns
- Not-found: repository wraps `sql.ErrNoRows` into `errors.NotFoundError` (not returning nil, nil)
- Transactional methods receive `*sql.Tx` as parameter
- Non-transactional reads use `*sql.DB` directly
- Always check `rows.Err()` after iterating result sets
- Parameterized queries with `?` placeholders always

## Transaction design (reserve-and-add spec)
- Isolation: `sql.LevelRepeatableRead` — correct for SELECT FOR UPDATE pattern
- Lock acquisition order: products sorted by productId ASC (anti-deadlock)
- Pattern: `defer tx.Rollback()` + explicit `tx.Commit()` — idiomatic Go
- Retry: max 3 attempts, backoffs [0ms, 100ms, 200ms] + ±20% jitter
- Deadlock detection: MySQL error 1213 (ER_LOCK_DEADLOCK) and 1205 (ER_LOCK_WAIT_TIMEOUT)
- Transaction timeout: 5 seconds via context.WithTimeout

## Hardening rules confirmed
- Always add `companyId` to UPDATE WHERE clauses as defensive filter (even when PK is sufficient)
- Always check `RowsAffected() == 0` on critical UPDATE statements
- `FOR UPDATE` reads ignore MVCC snapshot — always see current committed data (correct for stock check)
- `isActive` validated in service layer, not in SQL — allows precise failure reason reporting

## Architecture (relevant to repository layer)
- Interfaces defined in the consuming package (CLAUDE.md rule)
- `*sql.DB` lives in the use case (transaction owner), service receives `*sql.Tx`
- Repos: `internal/product/repository/`, `internal/order/repository/`, `internal/company/repository/`

## See also
- `specs/spec-1-order-domain-data-access.md` — SQL queries and schema mapping
- `specs/spec-2-stock-reservation-service.md` — transaction design and business rules
