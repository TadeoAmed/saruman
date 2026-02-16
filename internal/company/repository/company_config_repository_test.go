package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"saruman/internal/errors"
	"saruman/internal/testutil"
)

// Unit Tests

func TestNewMySQLCompanyConfigRepository(t *testing.T) {
	db := &sql.DB{}
	repo := NewMySQLCompanyConfigRepository(db)

	assert.NotNil(t, repo)
	assert.Equal(t, db, repo.db)
}

// Integration Tests

func TestCompanyConfigRepository_FindByCompanyID_Success(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	repo := NewMySQLCompanyConfigRepository(db)

	// Insert test config
	jsonConfig := `{"fields": ["field1", "field2"]}`
	result, err := db.Exec(`
		INSERT INTO CompanyConfig (companyId, fieldsOrderConfig, hasStock)
		VALUES (1, ?, true)
	`, jsonConfig)
	require.NoError(t, err)

	configID, err := result.LastInsertId()
	require.NoError(t, err)

	// Test FindByCompanyID
	config, err := repo.FindByCompanyID(context.Background(), 1)
	require.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, int(configID), config.ID)
	assert.Equal(t, 1, config.CompanyID)
	assert.Equal(t, jsonConfig, config.FieldsOrderConfig)
	assert.True(t, config.HasStock)
}

func TestCompanyConfigRepository_FindByCompanyID_NotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	repo := NewMySQLCompanyConfigRepository(db)

	// Test FindByCompanyID with non-existent company
	config, err := repo.FindByCompanyID(context.Background(), 9999)
	assert.Error(t, err)
	assert.Nil(t, config)

	nfe, ok := errors.IsNotFoundError(err)
	assert.True(t, ok)
	assert.NotNil(t, nfe)
}

func TestCompanyConfigRepository_FindByCompanyID_WithoutStock(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	repo := NewMySQLCompanyConfigRepository(db)

	// Insert config without stock
	jsonConfig := `{"fields": []}`
	result, err := db.Exec(`
		INSERT INTO CompanyConfig (companyId, fieldsOrderConfig, hasStock)
		VALUES (2, ?, false)
	`, jsonConfig)
	require.NoError(t, err)

	_, err = result.LastInsertId()
	require.NoError(t, err)

	// Test FindByCompanyID
	config, err := repo.FindByCompanyID(context.Background(), 2)
	require.NoError(t, err)
	assert.NotNil(t, config)
	assert.False(t, config.HasStock)
	assert.Equal(t, jsonConfig, config.FieldsOrderConfig)
}

func TestCompanyConfigRepository_FindByCompanyID_JSONFieldHandling(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	repo := NewMySQLCompanyConfigRepository(db)

	// Test various JSON configurations
	tests := []struct {
		name        string
		companyID   int
		jsonConfig  string
		hasStock    bool
	}{
		{
			name:       "simple json",
			companyID:  10,
			jsonConfig: `{"fields": ["name", "email"]}`,
			hasStock:   true,
		},
		{
			name:       "complex json",
			companyID:  20,
			jsonConfig: `{"fields": ["name", "email", "phone"], "config": {"nested": true}}`,
			hasStock:   false,
		},
		{
			name:       "empty json",
			companyID:  30,
			jsonConfig: `{}`,
			hasStock:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := db.Exec(`
				INSERT INTO CompanyConfig (companyId, fieldsOrderConfig, hasStock)
				VALUES (?, ?, ?)
			`, tt.companyID, tt.jsonConfig, tt.hasStock)
			require.NoError(t, err)

			_, err = result.LastInsertId()
			require.NoError(t, err)

			config, err := repo.FindByCompanyID(context.Background(), tt.companyID)
			require.NoError(t, err)
			assert.NotNil(t, config)
			assert.Equal(t, tt.jsonConfig, config.FieldsOrderConfig)
			assert.Equal(t, tt.hasStock, config.HasStock)
			assert.Equal(t, tt.companyID, config.CompanyID)
		})
	}
}

func TestCompanyConfigRepository_FindByCompanyID_TimestampHandling(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	repo := NewMySQLCompanyConfigRepository(db)

	// Insert config
	jsonConfig := `{"fields": []}`
	result, err := db.Exec(`
		INSERT INTO CompanyConfig (companyId, fieldsOrderConfig, hasStock)
		VALUES (40, ?, true)
	`, jsonConfig)
	require.NoError(t, err)

	_, err = result.LastInsertId()
	require.NoError(t, err)

	// Test FindByCompanyID
	config, err := repo.FindByCompanyID(context.Background(), 40)
	require.NoError(t, err)
	assert.NotNil(t, config)
	assert.False(t, config.CreatedAt.IsZero())
	assert.False(t, config.UpdatedAt.IsZero())
	assert.True(t, config.UpdatedAt.Equal(config.CreatedAt) || config.UpdatedAt.After(config.CreatedAt))
}

func TestCompanyConfigRepository_UniqueConstraint(t *testing.T) {
	db := testutil.SetupTestDB(t)
	testutil.SetupTestTables(t, db)
	defer testutil.CleanupTestDB(t, db)

	// Insert first config
	result, err := db.Exec(`
		INSERT INTO CompanyConfig (companyId, fieldsOrderConfig, hasStock)
		VALUES (50, ?, true)
	`, `{"fields": []}`)
	require.NoError(t, err)

	_, err = result.LastInsertId()
	require.NoError(t, err)

	// Try to insert second config with same companyId (should fail due to UNIQUE constraint)
	result, err = db.Exec(`
		INSERT INTO CompanyConfig (companyId, fieldsOrderConfig, hasStock)
		VALUES (50, ?, true)
	`, `{"fields": ["field1"]}`)

	// Note: The actual error depends on MySQL driver behavior, just verify the second insert failed
	require.Error(t, err)
}
