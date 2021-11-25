package service

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	log "github.com/ovirt/go-ovirt-client-log/v2"
	"google.golang.org/grpc"
	"k8s.io/klog"
)

func NewNonBlockingGRPCServer(
	logger log.Logger,
	ids csi.IdentityServer,
	cs csi.ControllerServer,
	ns csi.NodeServer,
	endpoint string,
) (Service, error) {
	u, err := url.Parse(endpoint)

	if err != nil {
		return nil, fmt.Errorf("failed to parse endpoint URL %s (%v)", endpoint, err)
	}

	var addr string
	if u.Scheme == "unix" {
		addr = u.Path
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to remove previous UNIX socket %s (%w)", addr, err)
		}

		listenDir := filepath.Dir(addr)
		if _, err := os.Stat(listenDir); err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("expected Kubelet plugin watcher to create parent dir %s but did not find the specified directory", listenDir)
			} else {
				return nil, fmt.Errorf("failed to stat UNIX socket %s (%w)", listenDir, err)
			}
		}
	} else if u.Scheme == "tcp" {
		addr = u.Host
	} else {
		return nil, fmt.Errorf("%v endpoint scheme not supported", u.Scheme)
	}

	return &nonBlockingGRPCServer{
		ids:    ids,
		cs:     cs,
		ns:     ns,
		logger: logger,
		addr:   addr,
		u:      u,
	}, nil
}

type nonBlockingGRPCServer struct {
	ids    csi.IdentityServer
	cs     csi.ControllerServer
	ns     csi.NodeServer
	u      *url.URL
	addr   string
	logger log.Logger
}

func (s *nonBlockingGRPCServer) run(
	ctx context.Context,
	ready chan struct{},
	stopping chan struct{},
	stopped chan struct{},
) {
	defer close(stopped)

	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(s.log),
	}

	s.logger.Infof("Start listening with scheme %v, addr %v", s.u.Scheme, s.addr)
	listener, err := net.Listen(s.u.Scheme, s.addr)
	if err != nil {
		klog.Fatalf("Failed to listen: %v", err)
	}

	server := grpc.NewServer(opts...)

	if s.ids != nil {
		csi.RegisterIdentityServer(server, s.ids)
	}
	if s.cs != nil {
		csi.RegisterControllerServer(server, s.cs)
	}
	if s.ns != nil {
		csi.RegisterNodeServer(server, s.ns)
	}

	s.logger.Infof("Listening for connections on address: %#v", listener.Addr())

	close(ready)
	go func() {
		select {
		case <-ctx.Done():
			server.Stop()
		case <-stopping:
		}
	}()

	if err := server.Serve(listener); err != nil {
		s.logger.Errorf("Failed to server (%v).", err)
	}

	close(stopping)
}

func (s *nonBlockingGRPCServer) log(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (resp interface{}, err error) {
	s.logger.Infof("%s called with request: %+v", info.FullMethod, protosanitizer.StripSecrets(req))
	resp, err = handler(ctx, req)
	if err != nil {
		s.logger.Errorf("%s returned with error: %v", info.FullMethod, err)
	} else {
		s.logger.Infof("%s returned with response: %+v", info.FullMethod, protosanitizer.StripSecrets(resp))
	}
	return resp, err
}
