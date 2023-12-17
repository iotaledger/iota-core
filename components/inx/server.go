package inx

import (
	"net"
	"time"

	grpcprometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	inx "github.com/iotaledger/inx/go"
)

const (
	workerCount = 1
)

func newServer() *Server {
	grpcServer := grpc.NewServer(
		grpc.StreamInterceptor(grpcprometheus.StreamServerInterceptor),
		grpc.UnaryInterceptor(grpcprometheus.UnaryServerInterceptor),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    20 * time.Second,
			Timeout: 5 * time.Second,
		}),
		grpc.MaxConcurrentStreams(10),
	)

	s := &Server{grpcServer: grpcServer}
	inx.RegisterINXServer(grpcServer, s)

	return s
}

type Server struct {
	inx.UnimplementedINXServer
	grpcServer *grpc.Server
}

func (s *Server) ConfigurePrometheus() {
	grpcprometheus.Register(s.grpcServer)
}

func (s *Server) Start() {
	go func() {
		listener, err := net.Listen("tcp", ParamsINX.BindAddress)
		if err != nil {
			Component.LogFatalf("failed to listen: %v", err)
		}
		defer listener.Close()

		if err := s.grpcServer.Serve(listener); err != nil {
			Component.LogFatalf("failed to serve: %v", err)
		}
	}()
}

func (s *Server) Stop() {
	s.grpcServer.Stop()
}
