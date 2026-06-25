package crops

import "fmt"

type ErrorCode string

const (
	InvalidRequest       ErrorCode = "invalid_request"
	UnsupportedMediaType ErrorCode = "unsupported_media_type"
	StorageError         ErrorCode = "storage_error"
	NotFound             ErrorCode = "not_found"
	InferenceError       ErrorCode = "inference_error"
	InternalError        ErrorCode = "internal_error"
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
