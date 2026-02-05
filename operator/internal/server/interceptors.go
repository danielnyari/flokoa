package server

import (
	"context"
	"runtime/debug"
	"time"

	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// LoggingUnaryInterceptor returns a unary server interceptor that logs requests.
func LoggingUnaryInterceptor(log logr.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		code := codes.OK
		if err != nil {
			code = status.Code(err)
		}

		log.V(1).Info("gRPC request",
			"method", info.FullMethod,
			"duration", duration.String(),
			"code", code.String(),
		)

		if err != nil {
			log.Error(err, "gRPC request failed",
				"method", info.FullMethod,
				"code", code.String(),
			)
		}

		return resp, err
	}
}

// LoggingStreamInterceptor returns a stream server interceptor that logs requests.
func LoggingStreamInterceptor(log logr.Logger) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()
		err := handler(srv, ss)
		duration := time.Since(start)

		code := codes.OK
		if err != nil {
			code = status.Code(err)
		}

		log.V(1).Info("gRPC stream",
			"method", info.FullMethod,
			"duration", duration.String(),
			"code", code.String(),
		)

		if err != nil {
			log.Error(err, "gRPC stream failed",
				"method", info.FullMethod,
				"code", code.String(),
			)
		}

		return err
	}
}

// RecoveryUnaryInterceptor returns a unary server interceptor that recovers from panics.
func RecoveryUnaryInterceptor(log logr.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				log.Error(nil, "Recovered from panic",
					"method", info.FullMethod,
					"panic", r,
					"stack", string(debug.Stack()),
				)
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()
		return handler(ctx, req)
	}
}

// RecoveryStreamInterceptor returns a stream server interceptor that recovers from panics.
func RecoveryStreamInterceptor(log logr.Logger) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) (err error) {
		defer func() {
			if r := recover(); r != nil {
				log.Error(nil, "Recovered from panic in stream",
					"method", info.FullMethod,
					"panic", r,
					"stack", string(debug.Stack()),
				)
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()
		return handler(srv, ss)
	}
}
