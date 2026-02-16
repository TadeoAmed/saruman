package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"saruman/internal/domain"
	"saruman/internal/testutil"
)

// Unit Tests

func TestNewMySQLOrderItemRepository(t *testing.T) {
	db := &sql.DB{}
	repo := NewMySQLOrderItemRepository(db)

	assert.NotNil(t, repo)
	assert.Equal(t, db, repo.db)
}

// Integration Tests

func TestOrderItemRepository_Insert_Success(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	itemRepo := NewMySQLOrderItemRepository(db)

	// Insert test order first
	orderResult, err := db.Exec(`
		INSERT INTO Orders (companyId, firstName, lastName, email, status, totalPrice)
		VALUES (1, 'John', 'Doe', 'john@example.com', 'PENDING', 99.99)
	`)
	require.NoError(t, err)

	orderID, err := orderResult.LastInsertId()
	require.NoError(t, err)

	// Begin transaction
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	// Insert order item
	item := domain.OrderItem{
		OrderID:   uint(orderID),
		ProductID: 5,
		Quantity:  3,
		Price:     29.99,
	}

	itemID, err := itemRepo.Insert(context.Background(), tx, item)
	require.NoError(t, err)
	assert.Greater(t, itemID, uint(0))

	err = tx.Commit()
	require.NoError(t, err)

	// Verify item was inserted by querying directly
	var retrievedItemID uint
	var qty int
	var price float64
	err = db.QueryRow(`
		SELECT id, quantity, price FROM OrderItems WHERE id = ? AND orderId = ?
	`, itemID, orderID).Scan(&retrievedItemID, &qty, &price)
	require.NoError(t, err)
	assert.Equal(t, itemID, retrievedItemID)
	assert.Equal(t, 3, qty)
	assert.Equal(t, 29.99, price)
}

func TestOrderItemRepository_Insert_MultipleItems(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	itemRepo := NewMySQLOrderItemRepository(db)

	// Insert test order
	orderResult, err := db.Exec(`
		INSERT INTO Orders (companyId, firstName, lastName, email, status, totalPrice)
		VALUES (1, 'John', 'Doe', 'john@example.com', 'PENDING', 99.99)
	`)
	require.NoError(t, err)

	orderID, err := orderResult.LastInsertId()
	require.NoError(t, err)

	// Begin transaction
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	// Insert multiple items
	items := []domain.OrderItem{
		{OrderID: uint(orderID), ProductID: 5, Quantity: 2, Price: 50.00},
		{OrderID: uint(orderID), ProductID: 10, Quantity: 1, Price: 75.50},
		{OrderID: uint(orderID), ProductID: 15, Quantity: 3, Price: 25.25},
	}

	var itemIDs []uint
	for _, item := range items {
		itemID, err := itemRepo.Insert(context.Background(), tx, item)
		require.NoError(t, err)
		itemIDs = append(itemIDs, itemID)
	}

	err = tx.Commit()
	require.NoError(t, err)

	// Verify all items were inserted
	assert.Len(t, itemIDs, 3)

	var count int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM OrderItems WHERE orderId = ?
	`, orderID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestOrderItemRepository_Insert_WithTransaction_Rollback(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	itemRepo := NewMySQLOrderItemRepository(db)

	// Insert test order
	orderResult, err := db.Exec(`
		INSERT INTO Orders (companyId, firstName, lastName, email, status, totalPrice)
		VALUES (1, 'John', 'Doe', 'john@example.com', 'PENDING', 99.99)
	`)
	require.NoError(t, err)

	orderID, err := orderResult.LastInsertId()
	require.NoError(t, err)

	// Begin transaction
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	// Insert item
	item := domain.OrderItem{
		OrderID:   uint(orderID),
		ProductID: 5,
		Quantity:  3,
		Price:     29.99,
	}

	itemID, err := itemRepo.Insert(context.Background(), tx, item)
	require.NoError(t, err)
	assert.Greater(t, itemID, uint(0))

	// Rollback transaction
	err = tx.Rollback()
	require.NoError(t, err)

	// Verify item was not persisted
	var count int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM OrderItems WHERE id = ?
	`, itemID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestOrderItemRepository_Insert_ReturnID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	itemRepo := NewMySQLOrderItemRepository(db)

	// Insert test order
	orderResult, err := db.Exec(`
		INSERT INTO Orders (companyId, firstName, lastName, email, status, totalPrice)
		VALUES (1, 'John', 'Doe', 'john@example.com', 'PENDING', 99.99)
	`)
	require.NoError(t, err)

	orderID, err := orderResult.LastInsertId()
	require.NoError(t, err)

	// Insert multiple items in sequence and verify IDs increment
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	item1 := domain.OrderItem{OrderID: uint(orderID), ProductID: 5, Quantity: 1, Price: 10.00}
	id1, err := itemRepo.Insert(context.Background(), tx, item1)
	require.NoError(t, err)

	item2 := domain.OrderItem{OrderID: uint(orderID), ProductID: 10, Quantity: 1, Price: 20.00}
	id2, err := itemRepo.Insert(context.Background(), tx, item2)
	require.NoError(t, err)

	err = tx.Commit()
	require.NoError(t, err)

	// Verify IDs are sequential
	assert.NotEqual(t, id1, id2)
	assert.Greater(t, id2, id1)
}
