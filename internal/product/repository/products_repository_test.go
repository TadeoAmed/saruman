package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"saruman/internal/testutil"
)

// Unit Tests

func TestNewMySQLRepository(t *testing.T) {
	db := &sql.DB{}
	repo := NewMySQLRepository(db)

	assert.NotNil(t, repo)
	assert.Equal(t, db, repo.db)
}

// Integration Tests

func TestRepository_FindByIDsAndCompany_Success(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	repo := NewMySQLRepository(db)

	// Insert test products
	_, err := db.Exec(`
		INSERT INTO Product (external_id, name, description, price, stock, reserved_stock,
		                       companyId, typeId, category, isActive, isDeleted, hasStock, Stockeable)
		VALUES (1, 'Product 1', 'Desc 1', 10.00, 100, 0, 1, 1, 'cat1', 1, 0, 1, 1),
		       (2, 'Product 2', 'Desc 2', 20.00, 50, 10, 1, 2, 'cat2', 1, 0, 1, 1),
		       (3, 'Product 3', 'Desc 3', 30.00, 25, 5, 1, 3, 'cat3', 1, 0, 1, 1)
	`)
	require.NoError(t, err)

	// Find products
	products, err := repo.FindByIDsAndCompany(context.Background(), []int{1, 2, 3}, 1)
	require.NoError(t, err)
	assert.Len(t, products, 3)
	assert.Equal(t, 1, products[0].ID)
	assert.Equal(t, 2, products[1].ID)
	assert.Equal(t, 3, products[2].ID)
}

func TestRepository_FindByIDsAndCompany_EmptyList(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	repo := NewMySQLRepository(db)

	// Find with empty IDs list
	products, err := repo.FindByIDsAndCompany(context.Background(), []int{}, 1)
	require.NoError(t, err)
	assert.Nil(t, products)
}

func TestRepository_FindByIDForUpdate_Success(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	repo := NewMySQLRepository(db)

	// Insert test product
	stock := 100
	reserved := 10
	_, err := db.Exec(`
		INSERT INTO Product (external_id, name, description, price, stock, reserved_stock,
		                       companyId, typeId, category, isActive, isDeleted, hasStock, Stockeable)
		VALUES (1, 'Product 1', 'Desc 1', 10.00, ?, ?, 1, 1, 'cat1', 1, 0, 1, 1)
	`, stock, reserved)
	require.NoError(t, err)

	// Get product in transaction
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer tx.Rollback()

	product, err := repo.FindByIDForUpdate(context.Background(), tx, 1, 1)
	require.NoError(t, err)
	assert.NotNil(t, product)
	assert.Equal(t, 1, product.ID)
	assert.Equal(t, 1, product.CompanyID)
	assert.Equal(t, "Product 1", product.Name)
	assert.NotNil(t, product.Stock)
	assert.Equal(t, stock, *product.Stock)
	assert.NotNil(t, product.ReservedStock)
	assert.Equal(t, reserved, *product.ReservedStock)
}

func TestRepository_FindByIDForUpdate_NotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	repo := NewMySQLRepository(db)

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer tx.Rollback()

	// Get non-existent product
	product, err := repo.FindByIDForUpdate(context.Background(), tx, 9999, 1)
	assert.Error(t, err)
	assert.Nil(t, product)
}

func TestRepository_FindByIDForUpdate_DifferentCompany(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	repo := NewMySQLRepository(db)

	// Insert product for company 1
	_, err := db.Exec(`
		INSERT INTO Product (external_id, name, description, price, stock, reserved_stock,
		                       companyId, typeId, category, isActive, isDeleted, hasStock, Stockeable)
		VALUES (1, 'Product 1', 'Desc 1', 10.00, 100, 0, 1, 1, 'cat1', 1, 0, 1, 1)
	`)
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer tx.Rollback()

	// Try to get product for company 2
	product, err := repo.FindByIDForUpdate(context.Background(), tx, 1, 2)
	assert.Error(t, err)
	assert.Nil(t, product)
}

func TestRepository_FindByIDForUpdate_DeletedProduct(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	repo := NewMySQLRepository(db)

	// Insert deleted product
	_, err := db.Exec(`
		INSERT INTO Product (external_id, name, description, price, stock, reserved_stock,
		                       companyId, typeId, category, isActive, isDeleted, hasStock, Stockeable)
		VALUES (1, 'Product 1', 'Desc 1', 10.00, 100, 0, 1, 1, 'cat1', 1, 1, 1, 1)
	`)
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer tx.Rollback()

	// Try to get deleted product
	product, err := repo.FindByIDForUpdate(context.Background(), tx, 1, 1)
	assert.Error(t, err)
	assert.Nil(t, product)
}

func TestRepository_FindByIDForUpdate_AcquiresLock(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	repo := NewMySQLRepository(db)

	// Insert product
	_, err := db.Exec(`
		INSERT INTO Product (external_id, name, description, price, stock, reserved_stock,
		                       companyId, typeId, category, isActive, isDeleted, hasStock, Stockeable)
		VALUES (1, 'Product 1', 'Desc 1', 10.00, 100, 0, 1, 1, 'cat1', 1, 0, 1, 1)
	`)
	require.NoError(t, err)

	// Start first transaction and acquire lock
	tx1, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	product, err := repo.FindByIDForUpdate(context.Background(), tx1, 1, 1)
	require.NoError(t, err)
	assert.NotNil(t, product)

	// Start second transaction and try to update (should wait for lock)
	tx2, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	// Create a channel to track update completion
	done := make(chan error, 1)
	go func() {
		_, err := tx2.ExecContext(context.Background(), `UPDATE Product SET name = ? WHERE id = 1`, "Updated")
		done <- err
	}()

	// Sleep a bit, then rollback first transaction to release lock
	time.Sleep(100 * time.Millisecond)
	tx1.Rollback()

	// Second update should now complete
	err = <-done
	require.NoError(t, err)
	tx2.Commit()
}

func TestRepository_IncrementReservedStock_Success(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	repo := NewMySQLRepository(db)

	// Insert product with initial reserved stock
	_, err := db.Exec(`
		INSERT INTO Product (external_id, name, description, price, stock, reserved_stock,
		                       companyId, typeId, category, isActive, isDeleted, hasStock, Stockeable)
		VALUES (1, 'Product 1', 'Desc 1', 10.00, 100, 10, 1, 1, 'cat1', 1, 0, 1, 1)
	`)
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	// Increment reserved stock
	err = repo.IncrementReservedStock(context.Background(), tx, 1, 5)
	require.NoError(t, err)

	err = tx.Commit()
	require.NoError(t, err)

	// Verify increment
	var reserved int
	err = db.QueryRow(`SELECT reserved_stock FROM Product WHERE id = 1`).Scan(&reserved)
	require.NoError(t, err)
	assert.Equal(t, 15, reserved)
}

func TestRepository_IncrementReservedStock_FromNull(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	repo := NewMySQLRepository(db)

	// Insert product with NULL reserved stock
	_, err := db.Exec(`
		INSERT INTO Product (external_id, name, description, price, stock, reserved_stock,
		                       companyId, typeId, category, isActive, isDeleted, hasStock, Stockeable)
		VALUES (2, 'Product 2', 'Desc 2', 20.00, 50, NULL, 1, 2, 'cat2', 1, 0, 1, 1)
	`)
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	// Increment reserved stock (from NULL, should treat as 0)
	err = repo.IncrementReservedStock(context.Background(), tx, 2, 8)
	require.NoError(t, err)

	err = tx.Commit()
	require.NoError(t, err)

	// Verify increment (should be 8, not NULL)
	var reserved *int
	err = db.QueryRow(`SELECT reserved_stock FROM Product WHERE id = 2`).Scan(&reserved)
	require.NoError(t, err)
	assert.NotNil(t, reserved)
	assert.Equal(t, 8, *reserved)
}

func TestRepository_IncrementReservedStock_Multiple(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	repo := NewMySQLRepository(db)

	// Insert product
	_, err := db.Exec(`
		INSERT INTO Product (external_id, name, description, price, stock, reserved_stock,
		                       companyId, typeId, category, isActive, isDeleted, hasStock, Stockeable)
		VALUES (3, 'Product 3', 'Desc 3', 30.00, 100, 0, 1, 3, 'cat3', 1, 0, 1, 1)
	`)
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	// Increment multiple times in same transaction
	err = repo.IncrementReservedStock(context.Background(), tx, 3, 5)
	require.NoError(t, err)

	err = repo.IncrementReservedStock(context.Background(), tx, 3, 3)
	require.NoError(t, err)

	err = tx.Commit()
	require.NoError(t, err)

	// Verify total increment (5 + 3 = 8)
	var reserved int
	err = db.QueryRow(`SELECT reserved_stock FROM Product WHERE id = 3`).Scan(&reserved)
	require.NoError(t, err)
	assert.Equal(t, 8, reserved)
}

func TestRepository_IncrementReservedStock_TransactionRollback(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	repo := NewMySQLRepository(db)

	// Insert product
	_, err := db.Exec(`
		INSERT INTO Product (external_id, name, description, price, stock, reserved_stock,
		                       companyId, typeId, category, isActive, isDeleted, hasStock, Stockeable)
		VALUES (4, 'Product 4', 'Desc 4', 40.00, 100, 5, 1, 4, 'cat4', 1, 0, 1, 1)
	`)
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	// Increment reserved stock
	err = repo.IncrementReservedStock(context.Background(), tx, 4, 10)
	require.NoError(t, err)

	// Rollback
	err = tx.Rollback()
	require.NoError(t, err)

	// Verify increment was rolled back
	var reserved int
	err = db.QueryRow(`SELECT reserved_stock FROM Product WHERE id = 4`).Scan(&reserved)
	require.NoError(t, err)
	assert.Equal(t, 5, reserved) // Should remain 5, not 15
}

func TestRepository_FindByIDForUpdate_AllFields(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	repo := NewMySQLRepository(db)

	// Insert product with all fields
	stock := 100
	reserved := 20
	typeID := 5
	_, err := db.Exec(`
		INSERT INTO Product (external_id, name, description, price, stock, reserved_stock,
		                       companyId, typeId, category, isActive, isDeleted, hasStock, Stockeable)
		VALUES (1, 'Full Product', 'Full Description', 99.99, ?, ?, 1, ?, 'Electronics', 1, 0, 1, 1)
	`, stock, reserved, typeID)
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer tx.Rollback()

	product, err := repo.FindByIDForUpdate(context.Background(), tx, 1, 1)
	require.NoError(t, err)
	assert.NotNil(t, product)
	assert.Equal(t, 1, product.ID)
	assert.Equal(t, 1, product.ExternalID)
	assert.Equal(t, "Full Product", product.Name)
	assert.Equal(t, "Full Description", product.Description)
	assert.Equal(t, 99.99, product.Price)
	assert.Equal(t, stock, *product.Stock)
	assert.Equal(t, reserved, *product.ReservedStock)
	assert.Equal(t, 1, product.CompanyID)
	assert.Equal(t, typeID, product.TypeID)
	assert.Equal(t, "Electronics", product.Category)
	assert.True(t, product.IsActive)
	assert.False(t, product.IsDeleted)
	assert.True(t, product.HasStock)
	assert.True(t, product.Stockeable)
}
