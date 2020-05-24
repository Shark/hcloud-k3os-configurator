package errorx

import "fmt"

// RetryableError represents a temporary error which tells the client that the operation may succeed when retried.
type RetryableError struct {
	Message string
	Err     error
}

// Error is the error message
func (e *RetryableError) Error() string {
	return fmt.Sprintf("retryable error: %s: %s", e.Message, e.Err)
}

// Unwrap makes this error conformant with Go 1.13 errors
func (e *RetryableError) Unwrap() error {
	return e.Err
}
