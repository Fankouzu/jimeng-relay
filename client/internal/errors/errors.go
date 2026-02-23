package errors

import (
	"fmt"
)

type Code string

const (
	ErrAuthFailed       Code = "AUTH_FAILED"
	ErrRateLimited      Code = "RATE_LIMITED"
	ErrTimeout          Code = "TIMEOUT"
	ErrBusinessFailed   Code = "BUSINESS_FAILED"
	ErrDecodeFailed     Code = "DECODE_FAILED"
	ErrValidationFailed Code = "VALIDATION_FAILED"
	ErrUnknown          Code = "UNKNOWN"
)

type Error struct {
	Code    Code
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error {
	return e.Err
}

func New(code Code, message string, err error) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

func GetCode(err error) Code {
	if err == nil {
		return ""
	}
	if e, ok := err.(*Error); ok {
		return e.Code
	}
	return ErrUnknown
}
