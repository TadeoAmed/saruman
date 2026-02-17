package testutil

import (
	"database/sql"
	"fmt"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

// SetupTestDB configura una base de datos de prueba
// Espera que exista una BD MySQL en localhost:3306 llamada 'saruman_test'
func SetupTestDB(t *testing.T) *sql.DB {
	dsn := "root:@tcp(localhost:3306)/saruman_test"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Verify connection
	err = db.Ping()
	if err != nil {
		t.Skipf("test database not available: %v", err)
	}

	return db
}

// CleanupTestDB limpia la BD de prueba
func CleanupTestDB(t *testing.T, db *sql.DB) {
	if db == nil {
		return
	}

	tables := []string{"OrderItems", "Orders", "CompanyConfig", "Product"}
	for _, table := range tables {
		_, err := db.Exec(fmt.Sprintf("DELETE FROM %s", table))
		if err != nil {
			t.Logf("failed to clean table %s: %v", table, err)
		}
	}

	db.Close()
}

// SetupTestTables crea las tablas necesarias para los tests
func SetupTestTables(t *testing.T, db *sql.DB) {
	createCompanyConfigTable := `
	CREATE TABLE IF NOT EXISTS CompanyConfig (
		id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
		companyId INT NOT NULL UNIQUE,
		fieldsOrderConfig JSON NOT NULL,
		hasStock TINYINT(1) NOT NULL DEFAULT 0,
		createdAt DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updatedAt DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	)`

	createProductTable := `
	CREATE TABLE IF NOT EXISTS Product (
		id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
		external_id INT,
		name VARCHAR(255),
		description TEXT,
		price DECIMAL(10,2),
		stock INT,
		reserved_stock INT,
		companyId INT NOT NULL,
		typeId INT,
		category VARCHAR(100),
		isActive TINYINT(1) DEFAULT 1,
		isDeleted TINYINT(1) DEFAULT 0,
		hasStock TINYINT(1) DEFAULT 0,
		Stockeable TINYINT(1) DEFAULT 0,
		createdAt DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updatedAt DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		INDEX idx_company (companyId),
		INDEX idx_deleted (isDeleted)
	)`

	createOrdersTable := `
	CREATE TABLE IF NOT EXISTS Orders (
		id INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
		companyId INT NOT NULL DEFAULT 1,
		firstName VARCHAR(100) NOT NULL,
		lastName VARCHAR(100) NOT NULL,
		email VARCHAR(150) NOT NULL,
		phone VARCHAR(30),
		address VARCHAR(255),
		status VARCHAR(50) DEFAULT 'PENDING',
		totalPrice DECIMAL(10,2) DEFAULT 0.00,
		createdAt DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updatedAt DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		INDEX idx_company (companyId)
	)`

	createOrderItemsTable := `
	CREATE TABLE IF NOT EXISTS OrderItems (
		id INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
		orderId INT UNSIGNED NOT NULL,
		productId INT NOT NULL,
		quantity INT DEFAULT 1,
		price DECIMAL(10,2) NOT NULL,
		FOREIGN KEY (orderId) REFERENCES Orders(id) ON DELETE CASCADE,
		INDEX idx_order (orderId),
		INDEX idx_product (productId)
	)`

	tables := []struct {
		name  string
		query string
	}{
		{"CompanyConfig", createCompanyConfigTable},
		{"Product", createProductTable},
		{"Orders", createOrdersTable},
		{"OrderItems", createOrderItemsTable},
	}

	for _, tbl := range tables {
		_, err := db.Exec(tbl.query)
		if err != nil {
			t.Logf("failed to create table %s: %v", tbl.name, err)
		}
	}
}
