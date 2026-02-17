package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"saruman/internal/domain"
	"saruman/internal/errors"
	"saruman/internal/testutil"
)

// Unit Tests

func TestNewMySQLOrderRepository(t *testing.T) {
	db := &sql.DB{}
	repo := NewMySQLOrderRepository(db)

	assert.NotNil(t, repo)
	assert.Equal(t, db, repo.db)
}

// Integration Tests

func TestOrderRepository_FindByID_Success(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	repo := NewMySQLOrderRepository(db)

	// Insert test data
	result, err := db.Exec(`
		INSERT INTO Orders (companyId, firstName, lastName, email, phone, address, status, totalPrice)
		VALUES (1, 'John', 'Doe', 'john@example.com', '1234567890', '123 Main St', 'PENDING', 99.99)
	`)
	require.NoError(t, err)

	id, err := result.LastInsertId()
	require.NoError(t, err)

	// Test FindByID
	order, err := repo.FindByID(context.Background(), uint(id))
	require.NoError(t, err)
	assert.NotNil(t, order)
	assert.Equal(t, uint(id), order.ID)
	assert.Equal(t, 1, order.CompanyID)
	assert.Equal(t, "John", order.FirstName)
	assert.Equal(t, "Doe", order.LastName)
	assert.Equal(t, "john@example.com", order.Email)
	assert.Equal(t, "PENDING", order.Status)
	assert.Equal(t, 99.99, order.TotalPrice)
	assert.NotNil(t, order.Phone)
	assert.Equal(t, "1234567890", *order.Phone)
	assert.NotNil(t, order.Address)
	assert.Equal(t, "123 Main St", *order.Address)
}

func TestOrderRepository_FindByID_NotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	repo := NewMySQLOrderRepository(db)

	// Test FindByID with non-existent ID
	order, err := repo.FindByID(context.Background(), uint(9999))
	assert.Error(t, err)
	assert.Nil(t, order)

	nfe, ok := errors.IsNotFoundError(err)
	assert.True(t, ok)
	assert.NotNil(t, nfe)
}

func TestOrderRepository_FindByID_NullableFields(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	repo := NewMySQLOrderRepository(db)

	// Insert order without phone and address (NULL)
	result, err := db.Exec(`
		INSERT INTO Orders (companyId, firstName, lastName, email, status, totalPrice)
		VALUES (1, 'Jane', 'Smith', 'jane@example.com', 'CREATED', 150.50)
	`)
	require.NoError(t, err)

	id, err := result.LastInsertId()
	require.NoError(t, err)

	order, err := repo.FindByID(context.Background(), uint(id))
	require.NoError(t, err)
	assert.NotNil(t, order)
	assert.Nil(t, order.Phone)
	assert.Nil(t, order.Address)
}

func TestOrderRepository_UpdateStatus_Success(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	repo := NewMySQLOrderRepository(db)

	// Insert test order
	result, err := db.Exec(`
		INSERT INTO Orders (companyId, firstName, lastName, email, status, totalPrice)
		VALUES (1, 'John', 'Doe', 'john@example.com', 'PENDING', 99.99)
	`)
	require.NoError(t, err)

	id, err := result.LastInsertId()
	require.NoError(t, err)

	// Begin transaction
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	// Test UpdateStatus
	err = repo.UpdateStatus(context.Background(), tx, uint(id), domain.OrderStatusCreated)
	require.NoError(t, err)

	err = tx.Commit()
	require.NoError(t, err)

	// Verify update
	order, err := repo.FindByID(context.Background(), uint(id))
	require.NoError(t, err)
	assert.Equal(t, domain.OrderStatusCreated, order.Status)
}

func TestOrderRepository_UpdateStatus_NotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	repo := NewMySQLOrderRepository(db)

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer tx.Rollback()

	// Test UpdateStatus with non-existent ID
	err = repo.UpdateStatus(context.Background(), tx, uint(9999), domain.OrderStatusCreated)
	assert.Error(t, err)

	nfe, ok := errors.IsNotFoundError(err)
	assert.True(t, ok)
	assert.NotNil(t, nfe)
}

func TestOrderRepository_UpdateTotalPrice_Success(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	repo := NewMySQLOrderRepository(db)

	// Insert test order
	result, err := db.Exec(`
		INSERT INTO Orders (companyId, firstName, lastName, email, status, totalPrice)
		VALUES (1, 'John', 'Doe', 'john@example.com', 'PENDING', 50.00)
	`)
	require.NoError(t, err)

	id, err := result.LastInsertId()
	require.NoError(t, err)

	// Begin transaction
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	// Test UpdateTotalPrice
	newPrice := 150.50
	err = repo.UpdateTotalPrice(context.Background(), tx, uint(id), newPrice)
	require.NoError(t, err)

	err = tx.Commit()
	require.NoError(t, err)

	// Verify update
	order, err := repo.FindByID(context.Background(), uint(id))
	require.NoError(t, err)
	assert.Equal(t, newPrice, order.TotalPrice)
}

func TestOrderRepository_UpdateTotalPrice_NotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	repo := NewMySQLOrderRepository(db)

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer tx.Rollback()

	// Test UpdateTotalPrice with non-existent ID
	err = repo.UpdateTotalPrice(context.Background(), tx, uint(9999), 200.00)
	assert.Error(t, err)

	nfe, ok := errors.IsNotFoundError(err)
	assert.True(t, ok)
	assert.NotNil(t, nfe)
}

func TestOrderRepository_TransactionIsolation(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	repo := NewMySQLOrderRepository(db)

	// Insert test order
	result, err := db.Exec(`
		INSERT INTO Orders (companyId, firstName, lastName, email, status, totalPrice)
		VALUES (1, 'John', 'Doe', 'john@example.com', 'PENDING', 100.00)
	`)
	require.NoError(t, err)

	id, err := result.LastInsertId()
	require.NoError(t, err)

	// Begin transaction and update, but rollback
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	err = repo.UpdateTotalPrice(context.Background(), tx, uint(id), 500.00)
	require.NoError(t, err)

	err = tx.Rollback()
	require.NoError(t, err)

	// Verify update was rolled back
	order, err := repo.FindByID(context.Background(), uint(id))
	require.NoError(t, err)
	assert.Equal(t, 100.00, order.TotalPrice)
}
