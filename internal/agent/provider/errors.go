package provider

import (
	"context"
	"errors"
	"net"
	"net/url"
)

type ErrorCategory string

const (
	ErrorUnconfigured    ErrorCategory = "unconfigured"
	ErrorAuthentication  ErrorCategory = "authentication_failed"
	ErrorRateLimited     ErrorCategory = "rate_limited"
	ErrorTimeout         ErrorCategory = "timeout"
	ErrorNetwork         ErrorCategory = "network_error"
	ErrorInvalidRequest  ErrorCategory = "invalid_request"
	ErrorInvalidResponse ErrorCategory = "invalid_response"
	ErrorService         ErrorCategory = "service_error"
)

var (
	ErrConfiguration   = errors.New("model provider configuration is incomplete")
	ErrInvalidRequest  = errors.New("model request is invalid")
	ErrInvalidResponse = errors.New("model provider returned an invalid response")
)

func ClassifyError(err error) (ErrorCategory, bool) {
	if err == nil {
		return "", false
	}
	switch {
	case errors.Is(err, ErrConfiguration):
		return ErrorUnconfigured, true
	case errors.Is(err, ErrInvalidRequest):
		return ErrorInvalidRequest, true
	case errors.Is(err, ErrInvalidResponse):
		return ErrorInvalidResponse, true
	case errors.Is(err, context.DeadlineExceeded):
		return ErrorTimeout, true
	case errors.Is(err, context.Canceled):
		return ErrorNetwork, true
	}
	var statusError *HTTPStatusError
	if errors.As(err, &statusError) {
		switch statusError.StatusCode {
		case 401, 403:
			return ErrorAuthentication, true
		case 429:
			return ErrorRateLimited, true
		case 400, 404, 422:
			return ErrorInvalidRequest, true
		default:
			return ErrorService, true
		}
	}
	var urlError *url.Error
	if errors.As(err, &urlError) {
		var networkError net.Error
		if errors.As(urlError.Err, &networkError) && networkError.Timeout() {
			return ErrorTimeout, true
		}
		return ErrorNetwork, true
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		if networkError.Timeout() {
			return ErrorTimeout, true
		}
		return ErrorNetwork, true
	}
	return "", false
}
