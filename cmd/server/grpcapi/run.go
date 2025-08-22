package grpcapi

import (
	"context"
	"log"
	"net"
	"net/http"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"

	codespacev1 "github.com/codespace-operator/codespace-operator/pkg/gen/codespace/v1"
	gw "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
)

// Start launches gRPC on grpcAddr and a grpc-gateway mux on the provided HTTP mux.
func Start(ctx context.Context, grpcAddr string, registerOn *http.ServeMux, svr codespacev1.SessionServiceServer) error {
	// gRPC server (h2c OK inside cluster; add TLS for prod)
	gs := grpc.NewServer()
	codespacev1.RegisterSessionServiceServer(gs, svr)
	reflection.Register(gs)
	l, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return err
	}
	go func() { log.Printf("gRPC listening on %s", grpcAddr); _ = gs.Serve(l) }()

	// grpc‑gateway mux mounted under the existing http mux
	mux := gw.NewServeMux(gw.WithMetadata(func(ctx context.Context, req *http.Request) metadata.MD {
		// forward auth header if you add OIDC later
		if v := req.Header.Get("Authorization"); v != "" {
			return metadata.Pairs("authorization", v)
		}
		return nil
	}))
	conn, err := grpc.DialContext(ctx, grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	if err := codespacev1.RegisterSessionServiceHandler(ctx, mux, conn); err != nil {
		return err
	}

	registerOn.Handle("/api/v1/", mux)
	return nil
}
