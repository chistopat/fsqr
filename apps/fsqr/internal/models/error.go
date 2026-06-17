package models

type ErrorCode string

const (
	InvalidRequest ErrorCode = "invalid_request"
	NotFound       ErrorCode = "not_found"
	InternalError  ErrorCode = "internal_error"
)

type ErrorResponse struct {
	Error APIError `json:"error"`
}

type APIError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

type HealthResponse struct {
	Status string `json:"status"`
}
