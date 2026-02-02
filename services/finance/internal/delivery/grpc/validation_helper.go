// Package grpc provides gRPC server implementation for finance service.
package grpc

import (
	"regexp"
	"strings"

	"buf.build/go/protovalidate"
	"google.golang.org/protobuf/proto"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
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
// Example input: "validation errors:\n - field1: error message\n - field2: error message"
func parseValidationErrors(errStr string) []*commonv1.ValidationError {
	var errors []*commonv1.ValidationError

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
			errors = append(errors, &commonv1.ValidationError{
				Field:   match[1],
				Message: strings.TrimSpace(match[2]),
			})
		} else if line != "" {
			// If pattern doesn't match, add as generic error
			errors = append(errors, &commonv1.ValidationError{
				Field:   "unknown",
				Message: line,
			})
		}
	}

	return errors
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
