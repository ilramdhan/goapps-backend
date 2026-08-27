// Package grpc provides gRPC server implementation.
package grpc

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
)

// StructuredErrorInterceptor is the outermost interceptor that catches all
// gRPC errors and wraps them into a typed response with BaseResponse.
// This ensures consistent response format for both gRPC and HTTP clients.
func StructuredErrorInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)
		if err == nil {
			return resp, nil
		}

		// Try to wrap the gRPC error in a typed response
		if structured := wrapErrorInResponse(info.FullMethod, err); structured != nil {
			return structured, nil
		}

		// Fallback: return raw gRPC error
		return nil, err
	}
}

// wrapErrorInResponse dynamically creates the correct response message for
// the given gRPC method and sets its "base" field from the error.
func wrapErrorInResponse(fullMethod string, grpcErr error) proto.Message {
	// Parse "/package.Service/Method" → ["package.Service", "Method"]
	parts := strings.Split(strings.TrimPrefix(fullMethod, "/"), "/")
	if len(parts) != 2 {
		return nil
	}

	// Look up the service descriptor from the global proto registry
	desc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(parts[0]))
	if err != nil {
		return nil
	}

	svc, ok := desc.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil
	}

	methodDesc := svc.Methods().ByName(protoreflect.Name(parts[1]))
	if methodDesc == nil {
		return nil
	}

	// Find the Go type for the response message
	outputType, err := protoregistry.GlobalTypes.FindMessageByName(methodDesc.Output().FullName())
	if err != nil {
		return nil
	}

	resp := outputType.New().Interface()

	// Find and set the "base" field using protobuf reflection
	baseFieldDesc := resp.ProtoReflect().Descriptor().Fields().ByName("base")
	if baseFieldDesc == nil {
		return nil
	}

	st := status.Convert(grpcErr)
	base := &commonv1.BaseResponse{
		IsSuccess:  false,
		StatusCode: fmt.Sprintf("%d", grpcCodeToHTTPStatus(st.Code())),
		Message:    st.Message(),
	}

	resp.ProtoReflect().Set(baseFieldDesc, protoreflect.ValueOfMessage(base.ProtoReflect()))

	return resp
}

// invalidIDResponse returns a BaseResponse for an invalid UUID field.
func invalidIDResponse(fieldName string) *commonv1.BaseResponse {
	return &commonv1.BaseResponse{
		IsSuccess:  false,
		StatusCode: "400",
		Message:    fmt.Sprintf("invalid %s: must be a valid UUID", fieldName),
		ValidationErrors: []*commonv1.ValidationError{
			{Field: fieldName, Message: "must be a valid UUID"},
		},
	}
}

// frozenCheckStatusResponse returns a BaseResponse rejecting a write to the
// FROZEN mbh_check_status column (plan §11 item 106, user decision = option 2:
// REJECT LOUDLY, never ignore silently).
//
// 🔴 The rejection fires whenever the field is PRESENT, not merely when it is
// non-empty. mbh_check_status is a proto3 `optional string`, so presence IS
// tracked: nil = absent, non-nil = the caller deliberately sent something. An
// explicitly sent "" is the MOST destructive case, not the most harmless one —
// letting it through would overwrite a stored Oracle trace with an empty string.
// Accepting "" while rejecting "Current" would therefore seal the safe write and
// wave the dangerous one through. Callers that used to send this field must drop
// it from the payload entirely.
//
// The message explains WHY so an operator learns the rule instead of guessing.
func frozenCheckStatusResponse() *commonv1.BaseResponse {
	const msg = "mbh_check_status is frozen: it is the Oracle import trace and is " +
		"read-only. Remove the field from the request; the derived value is " +
		"maintained by the server in mbh_check_status_calc."
	return &commonv1.BaseResponse{
		IsSuccess:  false,
		StatusCode: "400",
		Message:    msg,
		ValidationErrors: []*commonv1.ValidationError{
			{Field: "mbh_check_status", Message: msg},
		},
	}
}

// Messages returned for MB Recipe workflow actions that were removed by the
// USER DECISION of 2026-08-26 (see mb_head_handler.go). The workflow is now
// DRAFT (editable) → SUBMITTED (not editable) → APPROVED (locked); from SUBMITTED the
// only actions are Approve and Reject, and a locked recipe offers only Request Unlock.
//
// Each message says WHAT was removed and WHAT to do instead, so an operator or an
// out-of-date client learns the new rule rather than seeing a bare failure.
const (
	unApproveRemovedMessage = "un-approve has been removed from the MB recipe workflow. " +
		"An approved recipe is locked; use Request Unlock to reopen it for editing."
	revokeRemovedMessage = "revoke has been removed from the MB recipe workflow. " +
		"Deactivate the recipe with its active flag instead of revoking it."
)

// featureRemovedResponse returns a BaseResponse rejecting an RPC whose feature has been
// removed from the product while the RPC itself stays in the proto contract.
//
// 🔴 410 Gone, deliberately — ⛔ not 400 and ⛔ not 501. The request is well formed and
// the caller is not at fault for having been built against it; the capability simply no
// longer exists at this endpoint. 410 is the one status that says exactly that, and it
// keeps these refusals distinguishable from ordinary validation failures in logs.
//
// The BaseResponse pattern is used (⛔ never a raw gRPC error) so REST clients through
// the gateway see the same envelope as every other MB Head response.
func featureRemovedResponse(message string) *commonv1.BaseResponse {
	return &commonv1.BaseResponse{
		IsSuccess:  false,
		StatusCode: "410",
		Message:    message,
	}
}

// grpcCodeToHTTPStatus maps gRPC status codes to HTTP status codes.
func grpcCodeToHTTPStatus(code codes.Code) int {
	switch code {
	case codes.OK:
		return 200
	case codes.InvalidArgument:
		return 400
	case codes.Unauthenticated:
		return 401
	case codes.PermissionDenied:
		return 403
	case codes.NotFound:
		return 404
	case codes.AlreadyExists:
		return 409
	case codes.ResourceExhausted:
		return 429
	case codes.FailedPrecondition:
		return 412
	case codes.Unimplemented:
		return 501
	case codes.Unavailable:
		return 503
	case codes.DeadlineExceeded:
		return 504
	default:
		return 500
	}
}
