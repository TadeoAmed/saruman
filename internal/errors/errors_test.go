package errors

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNotFoundError_Creation(t *testing.T) {
	message := "order not found"
	err := NewNotFoundError(message)

	assert.NotNil(t, err)
	assert.Equal(t, message, err.Message)
	assert.Equal(t, message, err.Error())
}

func TestNotFoundError_IsNotFoundError(t *testing.T) {
	err := NewNotFoundError("test not found")

	notFoundErr, ok := IsNotFoundError(err)
	assert.True(t, ok)
	assert.NotNil(t, notFoundErr)
	assert.Equal(t, "test not found", notFoundErr.Message)
}

func TestNotFoundError_IsNotFoundError_WithOtherError(t *testing.T) {
	err := errors.New("some other error")

	notFoundErr, ok := IsNotFoundError(err)
	assert.False(t, ok)
	assert.Nil(t, notFoundErr)
}

func TestNotFoundError_ErrorInterface(t *testing.T) {
	var err error = NewNotFoundError("entity not found")
	assert.NotNil(t, err)
	assert.Equal(t, "entity not found", err.Error())
}

func TestValidationError_Creation(t *testing.T) {
	message := "validation failed"
	details := []ValidationDetail{
		{Field: "email", Message: "invalid email"},
		{Field: "name", Message: "required field"},
	}

	err := NewValidationError(message, details...)

	assert.NotNil(t, err)
	assert.Equal(t, message, err.Message)
	assert.Equal(t, message, err.Error())
	assert.Len(t, err.Details, 2)
}

func TestInternalError_Creation(t *testing.T) {
	cause := errors.New("database error")
	err := NewInternalError("failed to query database", cause)

	assert.NotNil(t, err)
	assert.Equal(t, "failed to query database", err.Message)
	assert.Equal(t, cause, err.Cause)
	assert.Contains(t, err.Error(), "failed to query database")
	assert.Contains(t, err.Error(), "database error")
}

func TestInternalError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := NewInternalError("wrapper", cause)

	assert.Equal(t, cause, err.Unwrap())
	assert.True(t, errors.Is(err, cause))
}

func TestInternalError_NilCause(t *testing.T) {
	err := NewInternalError("no cause", nil)

	assert.Equal(t, "no cause", err.Error())
	assert.Nil(t, err.Unwrap())
}
