// Package client provides the gRPC client for connecting to the
// Uptimy control plane.
package client

import (
	"context"
	"crypto/tls"
	"sync"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// ControlPlaneClient manages the gRPC connection to the control plane.
type ControlPlaneClient struct {
	endpoint  string
	token     string
	useTLS    bool
	logger    *zap.SugaredLogger
	conn      *grpc.ClientConn
	mu        sync.Mutex
	connected bool
}

// Option configures a ControlPlaneClient.
type Option func(*ControlPlaneClient)

// WithTLS enables TLS transport credentials.
func WithTLS() Option {
	return func(c *ControlPlaneClient) { c.useTLS = true }
}

// NewControlPlaneClient creates a new control plane client.
func NewControlPlaneClient(endpoint, token string, logger *zap.SugaredLogger, opts ...Option) *ControlPlaneClient {
	c := &ControlPlaneClient{
		endpoint: endpoint,
		token:    token,
		logger:   logger,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Connect establishes the gRPC connection.
func (c *ControlPlaneClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	var transportCred grpc.DialOption
	if c.useTLS {
		transportCred = grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			MinVersion: tls.VersionTLS12,
		}))
	} else {
		transportCred = grpc.WithTransportCredentials(insecure.NewCredentials())
	}

	conn, err := grpc.NewClient(
		c.endpoint,
		transportCred,
	)
	if err != nil {
		return err
	}

	c.conn = conn
	c.connected = true
	c.logger.Infow("connected to control plane", "endpoint", c.endpoint, "tls", c.useTLS)
	return nil
}

// Disconnect closes the gRPC connection.
func (c *ControlPlaneClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected || c.conn == nil {
		return nil
	}

	err := c.conn.Close()
	c.connected = false
	c.conn = nil
	return err
}

// IsConnected returns true if currently connected.
func (c *ControlPlaneClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// AuthContext returns a context with the bearer token attached as gRPC
// metadata. All RPCs should use this context.
func (c *ControlPlaneClient) AuthContext(ctx context.Context) context.Context {
	if c.token == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+c.token)
}

// RunWithReconnect connects and stays connected, retrying with exponential
// backoff. It blocks until ctx is canceled.
func (c *ControlPlaneClient) RunWithReconnect(ctx context.Context) {
	backoff := 1 * time.Second
	maxBackoff := 60 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := c.Connect(ctx); err != nil {
			c.logger.Warnw("control plane connection failed, retrying",
				"error", err,
				"backoff", backoff,
			)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		// Connected - wait for context cancellation
		// or connection error (in future: run bidirectional stream here).
		c.logger.Infow("control plane connection established, waiting for events")

		// Block until context is canceled. In a full implementation,
		// this would run the bidirectional streaming RPC.
		<-ctx.Done()
		return
	}
}
