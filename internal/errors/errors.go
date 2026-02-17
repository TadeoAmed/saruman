package errors

import "fmt"

type ValidationDetail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationError struct {
	Message string
	Details []ValidationDetail
}

func (e *ValidationError) Error() string {
	return e.Message
}

func NewValidationError(message string, details ...ValidationDetail) *ValidationError {
	return &ValidationError{
		Message: message,
		Details: details,
	}
}

func IsValidationError(err error) (*ValidationError, bool) {
	if ve, ok := err.(*ValidationError); ok {
		return ve, true
	}
	return nil, false
}

type InternalError struct {
	Message string
	Cause   error
}

func (e *InternalError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *InternalError) Unwrap() error {
	return e.Cause
}

func NewInternalError(message string, cause error) *InternalError {
	return &InternalError{
		Message: message,
		Cause:   cause,
	}
}

type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return e.Message
}

func NewNotFoundError(message string) *NotFoundError {
	return &NotFoundError{
		Message: message,
	}
}

func IsNotFoundError(err error) (*NotFoundError, bool) {
	if nfe, ok := err.(*NotFoundError); ok {
		return nfe, true
	}
	return nil, false
}

type ConflictError struct {
	Message string
}

func (e *ConflictError) Error() string {
	return e.Message
}

func NewConflictError(message string) *ConflictError {
	return &ConflictError{
		Message: message,
	}
}

func IsConflictError(err error) (*ConflictError, bool) {
	if ce, ok := err.(*ConflictError); ok {
		return ce, true
	}
	return nil, false
}

type ForbiddenError struct {
	Message string
}

func (e *ForbiddenError) Error() string {
	return e.Message
}

func NewForbiddenError(message string) *ForbiddenError {
	return &ForbiddenError{
		Message: message,
	}
}

func IsForbiddenError(err error) (*ForbiddenError, bool) {
	if fe, ok := err.(*ForbiddenError); ok {
		return fe, true
	}
	return nil, false
}

type DeadlockError struct {
	Message string
}

func (e *DeadlockError) Error() string {
	return e.Message
}

func NewDeadlockError(message string) *DeadlockError {
	return &DeadlockError{
		Message: message,
	}
}

func IsDeadlockError(err error) (*DeadlockError, bool) {
	if de, ok := err.(*DeadlockError); ok {
		return de, true
	}
	return nil, false
}
