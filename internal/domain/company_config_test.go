package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCompanyConfig_Creation(t *testing.T) {
	createdAt := time.Now()
	updatedAt := time.Now()
	jsonConfig := `{"fields": ["field1", "field2"]}`

	config := CompanyConfig{
		ID:                1,
		CompanyID:         10,
		FieldsOrderConfig: jsonConfig,
		HasStock:          true,
		CreatedAt:         createdAt,
		UpdatedAt:         updatedAt,
	}

	assert.Equal(t, 1, config.ID)
	assert.Equal(t, 10, config.CompanyID)
	assert.Equal(t, jsonConfig, config.FieldsOrderConfig)
	assert.True(t, config.HasStock)
	assert.Equal(t, createdAt, config.CreatedAt)
	assert.Equal(t, updatedAt, config.UpdatedAt)
}

func TestCompanyConfig_WithoutStock(t *testing.T) {
	config := CompanyConfig{
		ID:                2,
		CompanyID:         20,
		FieldsOrderConfig: `{"fields": []}`,
		HasStock:          false,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	assert.False(t, config.HasStock)
}

func TestCompanyConfig_JSONFieldHandling(t *testing.T) {
	tests := []struct {
		name   string
		json   string
		valid  bool
	}{
		{
			name:  "empty JSON object",
			json:  "{}",
			valid: true,
		},
		{
			name:  "JSON with fields array",
			json:  `{"fields": ["field1"]}`,
			valid: true,
		},
		{
			name:  "complex JSON",
			json:  `{"fields": ["f1", "f2"], "config": {"nested": true}}`,
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := CompanyConfig{
				ID:                1,
				CompanyID:         10,
				FieldsOrderConfig: tt.json,
				HasStock:          true,
				CreatedAt:         time.Now(),
				UpdatedAt:         time.Now(),
			}

			assert.NotEmpty(t, config.FieldsOrderConfig)
			assert.Equal(t, tt.json, config.FieldsOrderConfig)
		})
	}
}
