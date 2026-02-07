// Package grpc provides gRPC server implementation for IAM service.
package grpc

import (
	"errors"
	"regexp"
	"strings"

	"buf.build/go/protovalidate"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// ValidationHelper provides validation utilities for handlers.
type ValidationHelper struct {
	validator protovalidate.Validator
}

// NewValidationHelper creates a new validation helper.
func NewValidationHelper() (*ValidationHelper, error) {
	validator, err := protovalidate.New()
	if err != nil {
		return nil, err
	}
	return &ValidationHelper{validator: validator}, nil
}

// ValidateRequest validates a proto message and returns structured validation errors.
// Returns nil if validation passes, otherwise returns BaseResponse with validation errors.
func (h *ValidationHelper) ValidateRequest(msg proto.Message) *commonv1.BaseResponse {
	if err := h.validator.Validate(msg); err != nil {
		// Parse protovalidate error to extract field-level errors
		validationErrors := parseValidationErrors(err.Error())

		return &commonv1.BaseResponse{
			IsSuccess:        false,
			StatusCode:       "400",
			Message:          "Validation failed",
			ValidationErrors: validationErrors,
		}
	}
	return nil
}

// parseValidationErrors parses protovalidate error string into ValidationError slice.
func parseValidationErrors(errStr string) []*commonv1.ValidationError {
	var errs []*commonv1.ValidationError

	// Split by " - " which is the separator for each error
	lines := strings.Split(errStr, " - ")

	// Field name pattern (captures field name before colon)
	fieldPattern := regexp.MustCompile(`^([a-z_]+):\s*(.+)$`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "validation errors:" {
			continue
		}

		match := fieldPattern.FindStringSubmatch(line)
		if len(match) == 3 {
			errs = append(errs, &commonv1.ValidationError{
				Field:   match[1],
				Message: strings.TrimSpace(match[2]),
			})
		} else if line != "" {
			// If pattern doesn't match, add as generic error
			errs = append(errs, &commonv1.ValidationError{
				Field:   "unknown",
				Message: line,
			})
		}
	}

	return errs
}

// ErrorResponse creates a BaseResponse for error cases.
func ErrorResponse(statusCode string, message string) *commonv1.BaseResponse {
	return &commonv1.BaseResponse{
		IsSuccess:  false,
		StatusCode: statusCode,
		Message:    message,
	}
}

// NotFoundResponse creates a 404 response.
func NotFoundResponse(message string) *commonv1.BaseResponse {
	return ErrorResponse("404", message)
}

// InternalErrorResponse creates a 500 response.
func InternalErrorResponse(message string) *commonv1.BaseResponse {
	return ErrorResponse("500", message)
}

// ConflictResponse creates a 409 response.
func ConflictResponse(message string) *commonv1.BaseResponse {
	return ErrorResponse("409", message)
}

// UnauthorizedResponse creates a 401 response.
func UnauthorizedResponse(message string) *commonv1.BaseResponse {
	return ErrorResponse("401", message)
}

// ForbiddenResponse creates a 403 response.
func ForbiddenResponse(message string) *commonv1.BaseResponse {
	return ErrorResponse("403", message)
}

// SuccessResponse creates a successful BaseResponse.
func SuccessResponse(message string) *commonv1.BaseResponse {
	return &commonv1.BaseResponse{
		IsSuccess:  true,
		StatusCode: "200",
		Message:    message,
	}
}

// parseOptionalUUID parses a proto optional string field into a *uuid.UUID.
// Returns nil if the input is nil or not a valid UUID.
func parseOptionalUUID(s *string) *uuid.UUID {
	if s == nil {
		return nil
	}
	id, err := uuid.Parse(*s)
	if err != nil {
		return nil
	}
	return &id
}

// domainErrorToBaseResponse maps domain errors to BaseResponse.
func domainErrorToBaseResponse(err error) *commonv1.BaseResponse {
	if err == nil {
		return SuccessResponse("Success")
	}

	switch {
	case errors.Is(err, shared.ErrNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, shared.ErrAlreadyExists):
		return ConflictResponse(err.Error())
	case errors.Is(err, shared.ErrAlreadyDeleted):
		return ConflictResponse(err.Error())
	case errors.Is(err, shared.ErrNotActive):
		return ErrorResponse("422", err.Error())
	case errors.Is(err, shared.ErrUnauthorized),
		errors.Is(err, shared.ErrInvalidCredentials),
		errors.Is(err, shared.ErrInvalidToken),
		errors.Is(err, shared.ErrTokenRevoked):
		return UnauthorizedResponse(err.Error())
	case errors.Is(err, shared.ErrPermissionDenied):
		return ForbiddenResponse(err.Error())
	case errors.Is(err, shared.ErrAccountLocked):
		return ForbiddenResponse(err.Error())
	case errors.Is(err, shared.ErrTOTPRequired),
		errors.Is(err, shared.ErrTwoFARequired):
		return ErrorResponse("428", "2FA required")
	case errors.Is(err, shared.ErrTOTPInvalid),
		errors.Is(err, shared.ErrInvalid2FACode),
		errors.Is(err, shared.ErrTwoFAAlreadyEnabled):
		return ErrorResponse("422", err.Error())
	case errors.Is(err, shared.ErrInvalidOTP):
		return ErrorResponse("422", err.Error())
	default:
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "invalid"):
			return ErrorResponse("400", errMsg)
		case strings.Contains(errMsg, "not found"):
			return NotFoundResponse(errMsg)
		case strings.Contains(errMsg, "already exists"):
			return ConflictResponse(errMsg)
		default:
			return InternalErrorResponse("internal server error")
		}
	}
}
