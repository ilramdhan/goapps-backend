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
