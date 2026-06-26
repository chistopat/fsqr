package beers

import "fmt"

type ErrorCode string

const (
	InvalidRequest ErrorCode = "invalid_request"
	InternalError  ErrorCode = "internal_error"
)

type Error struct {
	Code    ErrorCode
	Message string
	Err     error
}

func (err *Error) Error() string {
	if err.Err == nil {
		return err.Message
	}

	return fmt.Sprintf("%s: %v", err.Message, err.Err)
}

func (err *Error) Unwrap() error {
	return err.Err
}

func newError(code ErrorCode, message string, err error) *Error {
	return &Error{Code: code, Message: message, Err: err}
}
