package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/common/config"
	"github.com/sanskarpan/PayGate/internal/common/telemetry"
	"github.com/sanskarpan/PayGate/internal/ledger"
	ledgerpb "github.com/sanskarpan/PayGate/internal/ledger/pb"
	"google.golang.org/grpc"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := telemetry.Init(ctx, "ledger-service")
	if err != nil {
		return err
	}
	defer func() { _ = shutdownTelemetry(context.Background()) }()

	cfg := config.FromEnv()

	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	repo := ledger.NewRepository(db)
	svc := ledger.NewService(repo)
	grpcServer := grpc.NewServer(telemetry.GRPCServerOptions()...)
	ledgerpb.RegisterLedgerServiceServer(grpcServer, ledger.NewGRPCServer(svc))

	addr := "127.0.0.1:9090"
	if v := os.Getenv("LEDGER_GRPC_ADDR"); v != "" {
		addr = v
	}
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		grpcServer.Stop()
	}
	return nil
}
