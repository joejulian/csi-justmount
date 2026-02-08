package node

import (
	"context"
	"os"
	"strings"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type ctxLoggerKey struct{}

var (
	baseLogger *zap.Logger
	hostName   = "unknown"
)

func init() {
	logger, err := zap.NewProduction()
	if err != nil {
		logger = zap.NewNop()
	}
	baseLogger = logger
	if h, err := os.Hostname(); err == nil && h != "" {
		hostName = h
	}
}

func loggerFromContext(ctx context.Context) *zap.Logger {
	if ctx == nil {
		return baseLogger
	}
	if l, ok := ctx.Value(ctxLoggerKey{}).(*zap.Logger); ok && l != nil {
		return l
	}
	return baseLogger
}

func withLogger(ctx context.Context, l *zap.Logger) context.Context {
	return context.WithValue(ctx, ctxLoggerKey{}, l)
}

func Logger(ctx context.Context) *zap.Logger {
	return loggerFromContext(ctx)
}

func BaseLogger() *zap.Logger {
	return baseLogger
}

func unaryLoggingInterceptor(nodeID string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		requestID := requestIDFromMetadata(ctx)
		fields := []zap.Field{
			zap.String("request_id", requestID),
			zap.String("method", info.FullMethod),
			zap.String("node_id", nodeID),
			zap.String("host", hostName),
		}
		fields = append(fields, requestFields(req)...)
		l := baseLogger.With(fields...)
		ctx = withLogger(ctx, l)
		resp, err := handler(ctx, req)
		if err != nil {
			l.Error("request failed", zap.Error(err))
		} else {
			l.Info("request completed")
		}
		return resp, err
	}
}

func requestIDFromMetadata(ctx context.Context) string {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		for _, key := range []string{"x-request-id", "x-correlation-id", "request-id"} {
			if vals := md.Get(key); len(vals) > 0 && strings.TrimSpace(vals[0]) != "" {
				return strings.TrimSpace(vals[0])
			}
		}
	}
	return uuid.NewString()
}

func requestFields(req any) []zap.Field {
	switch r := req.(type) {
	case *csi.NodeStageVolumeRequest:
		return []zap.Field{
			zap.String("volume_id", r.VolumeId),
			zap.String("staging_target_path", r.StagingTargetPath),
		}
	case *csi.NodePublishVolumeRequest:
		return []zap.Field{
			zap.String("volume_id", r.VolumeId),
			zap.String("target_path", r.TargetPath),
			zap.String("staging_target_path", r.StagingTargetPath),
		}
	case *csi.NodeUnstageVolumeRequest:
		return []zap.Field{
			zap.String("volume_id", r.VolumeId),
			zap.String("staging_target_path", r.StagingTargetPath),
		}
	case *csi.NodeUnpublishVolumeRequest:
		return []zap.Field{
			zap.String("volume_id", r.VolumeId),
			zap.String("target_path", r.TargetPath),
		}
	default:
		return nil
	}
}
