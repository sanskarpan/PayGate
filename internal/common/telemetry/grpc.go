package telemetry

import (
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

func GRPCServerOptions() []grpc.ServerOption {
	return []grpc.ServerOption{grpc.StatsHandler(otelgrpc.NewServerHandler())}
}
