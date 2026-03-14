// Package client provides the gRPC client for connecting to the
// Uptimy control plane.
package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/uptimy/uptimy-agent/internal/telemetry"
	"github.com/uptimy/uptimy-agent/internal/version"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	pb "github.com/uptimy/uptimy-agent/pkg/proto"
)

// IncidentReport is an incident snapshot for streaming to the control plane.
type IncidentReport struct {
	ID             string
	CheckName      string
	Service        string
	Status         string
	FailureCount   int
	EventCreatedAt time.Time
}

// RepairReport is a repair result for streaming to the control plane.
type RepairReport struct {
	IncidentID     string
	Rule           string
	Recipe         string
	Status         string
	EventCreatedAt time.Time
	Duration       time.Duration
}

// ActiveIncidentCounter returns the current number of active incidents.
type ActiveIncidentCounter interface {
	ActiveCount() int
}

// ConfigUpdateHandler is called when the server pushes a configuration update.
type ConfigUpdateHandler func(version int32, bundleURL, signature string)

// ControlPlaneClient manages the gRPC connection and bidirectional stream
// to the control plane.
type ControlPlaneClient struct {
	endpoint  string
	agentUUID string
	apiKey    string
	useTLS    bool
	logger    *zap.SugaredLogger

	heartbeatInterval time.Duration
	telemetryInterval time.Duration

	telemetryDrainer *telemetry.RingBuffer
	incidentCounter  ActiveIncidentCounter
	onConfigUpdate   ConfigUpdateHandler

	incidents chan IncidentReport
	repairs   chan RepairReport

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

// WithHeartbeatInterval sets the heartbeat send interval (default 30s).
func WithHeartbeatInterval(d time.Duration) Option {
	return func(c *ControlPlaneClient) { c.heartbeatInterval = d }
}

// WithTelemetryInterval sets the telemetry flush interval (default 10s).
func WithTelemetryInterval(d time.Duration) Option {
	return func(c *ControlPlaneClient) { c.telemetryInterval = d }
}

// WithTelemetryDrainer sets the source for buffered telemetry events.
func WithTelemetryDrainer(d *telemetry.RingBuffer) Option {
	return func(c *ControlPlaneClient) { c.telemetryDrainer = d }
}

// WithActiveIncidentCounter provides the active incident count for heartbeats.
func WithActiveIncidentCounter(counter ActiveIncidentCounter) Option {
	return func(c *ControlPlaneClient) { c.incidentCounter = counter }
}

// WithConfigUpdateHandler sets the callback for config update messages.
func WithConfigUpdateHandler(h ConfigUpdateHandler) Option {
	return func(c *ControlPlaneClient) { c.onConfigUpdate = h }
}

// NewControlPlaneClient creates a new control plane client.
func NewControlPlaneClient(endpoint, agentUUID, apiKey string, logger *zap.SugaredLogger, opts ...Option) *ControlPlaneClient {
	c := &ControlPlaneClient{
		endpoint:          endpoint,
		agentUUID:         agentUUID,
		apiKey:            apiKey,
		logger:            logger,
		heartbeatInterval: 30 * time.Second,
		telemetryInterval: 10 * time.Second,
		incidents:         make(chan IncidentReport, 128),
		repairs:           make(chan RepairReport, 128),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// ReportIncident queues an incident report for sending on the stream.
func (c *ControlPlaneClient) ReportIncident(id, checkName, service, status string, failureCount int, eventCreatedAt time.Time) {
	select {
	case c.incidents <- IncidentReport{
		ID:             id,
		CheckName:      checkName,
		Service:        service,
		Status:         status,
		FailureCount:   failureCount,
		EventCreatedAt: eventCreatedAt,
	}:
	default:
		c.logger.Warnw("incident report channel full, dropping", "id", id)
	}
}

// ReportRepair queues a repair report for sending on the stream.
func (c *ControlPlaneClient) ReportRepair(incidentID, rule, recipe, status string, eventCreatedAt time.Time, duration time.Duration) {
	select {
	case c.repairs <- RepairReport{
		IncidentID:     incidentID,
		Rule:           rule,
		Recipe:         recipe,
		Status:         status,
		EventCreatedAt: eventCreatedAt,
		Duration:       duration,
	}:
	default:
		c.logger.Warnw("repair report channel full, dropping", "incidentID", incidentID)
	}
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

// authContext returns a context with the bearer token attached as gRPC metadata.
func (c *ControlPlaneClient) authContext(ctx context.Context) context.Context {
	if c.agentUUID == "" || c.apiKey == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+c.agentUUID+":"+c.apiKey)
}

// RunWithReconnect connects and runs the bidirectional stream, retrying
// with exponential backoff on failure. It blocks until ctx is canceled.
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

		// Connection established — run the bidirectional stream.
		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()

		authCtx := c.authContext(ctx)
		err := c.runStream(ctx, authCtx, conn)
		if ctx.Err() != nil {
			return
		}
		c.logger.Warnw("stream disconnected, will reconnect",
			"error", err,
			"backoff", backoff,
		)

		// Mark as disconnected so Connect() re-establishes.
		c.mu.Lock()
		c.connected = false
		if c.conn != nil {
			_ = c.conn.Close()
			c.conn = nil
		}
		c.mu.Unlock()

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// runStream opens the bidirectional stream and runs send/receive loops
// until ctx is canceled or the stream breaks.
func (c *ControlPlaneClient) runStream(ctx, authCtx context.Context, conn grpc.ClientConnInterface) error {
	rpcClient := pb.NewAgentControlPlaneClient(conn)

	stream, err := rpcClient.Connect(authCtx)
	if err != nil {
		return fmt.Errorf("opening stream: %w", err)
	}

	c.logger.Infow("gRPC stream established", "agent_uuid", c.agentUUID)

	// Create a cancellable context so both loops exit when one fails.
	loopCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 2)

	go func() {
		err := c.recvLoop(loopCtx, stream)
		cancel() // Signal the other loop to exit.
		errCh <- err
	}()
	go func() {
		err := c.sendLoop(loopCtx, stream)
		cancel() // Signal the other loop to exit.
		errCh <- err
	}()

	// Wait for both goroutines to exit.
	var firstErr error
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil && firstErr == nil && err != context.Canceled {
			firstErr = err
		}
	}

	_ = stream.CloseSend()

	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}

func (c *ControlPlaneClient) sendLoop(ctx context.Context, stream grpc.BidiStreamingClient[pb.AgentMessage, pb.ServerMessage]) error {
	heartbeatTicker := time.NewTicker(c.heartbeatInterval)
	defer heartbeatTicker.Stop()

	telemetryTicker := time.NewTicker(c.telemetryInterval)
	defer telemetryTicker.Stop()

	// Send an initial heartbeat immediately.
	if err := c.sendHeartbeat(stream); err != nil {
		return fmt.Errorf("sending initial heartbeat: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-heartbeatTicker.C:
			if err := c.sendHeartbeat(stream); err != nil {
				return fmt.Errorf("sending heartbeat: %w", err)
			}

		case <-telemetryTicker.C:
			if err := c.sendTelemetry(stream); err != nil {
				return fmt.Errorf("sending telemetry: %w", err)
			}

		case report := <-c.incidents:
			if err := c.sendIncidentReport(stream, &report); err != nil {
				return fmt.Errorf("sending incident report: %w", err)
			}

		case report := <-c.repairs:
			if err := c.sendRepairReport(stream, &report); err != nil {
				return fmt.Errorf("sending repair report: %w", err)
			}
		}
	}
}

func (c *ControlPlaneClient) recvLoop(ctx context.Context, stream grpc.BidiStreamingClient[pb.AgentMessage, pb.ServerMessage]) error {
	for {
		msg, err := stream.Recv()
		if err != nil {
			return fmt.Errorf("receiving message: %w", err)
		}

		switch payload := msg.Payload.(type) {
		case *pb.ServerMessage_ConfigUpdate:
			c.handleConfigUpdate(payload.ConfigUpdate)
		case *pb.ServerMessage_CommandRequest:
			c.handleCommandRequest(payload.CommandRequest)
		default:
			c.logger.Warnw("unknown server message type", "payload", msg.Payload)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}

func (c *ControlPlaneClient) sendHeartbeat(stream grpc.BidiStreamingClient[pb.AgentMessage, pb.ServerMessage]) error {
	activeIncidents := int32(0)
	if c.incidentCounter != nil {
		activeIncidents = int32(c.incidentCounter.ActiveCount())
	}

	hostname, _ := os.Hostname()

	msg := &pb.AgentMessage{
		AgentId: c.agentUUID,
		Payload: &pb.AgentMessage_Heartbeat{
			Heartbeat: &pb.Heartbeat{
				Timestamp:       time.Now().Unix(),
				Status:          "healthy",
				ActiveIncidents: activeIncidents,
				Hostname:        hostname,
				Os:              runtime.GOOS,
				Architecture:    runtime.GOARCH,
				Version:         version.Version,
			},
		},
	}

	if err := stream.Send(msg); err != nil {
		return err
	}
	c.logger.Infow("heartbeat sent", "active_incidents", activeIncidents)
	return nil
}

func (c *ControlPlaneClient) sendTelemetry(stream grpc.BidiStreamingClient[pb.AgentMessage, pb.ServerMessage]) error {
	if c.telemetryDrainer == nil {
		return nil
	}

	events := c.telemetryDrainer.Drain()
	if len(events) == 0 {
		return nil
	}

	pbEvents := make([]*pb.TelemetryEvent, 0, len(events))
	for _, e := range events {
		pbEvents = append(pbEvents, &pb.TelemetryEvent{
			Type:      e.Type,
			Timestamp: e.Timestamp.Unix(),
			Data:      e.Data,
		})
	}

	msg := &pb.AgentMessage{
		AgentId: c.agentUUID,
		Payload: &pb.AgentMessage_TelemetryBatch{
			TelemetryBatch: &pb.TelemetryBatch{
				Events: pbEvents,
			},
		},
	}

	if err := stream.Send(msg); err != nil {
		return err
	}
	c.logger.Infow("telemetry batch sent", "events", len(pbEvents))
	return nil
}

func (c *ControlPlaneClient) sendIncidentReport(stream grpc.BidiStreamingClient[pb.AgentMessage, pb.ServerMessage], r *IncidentReport) error {
	msg := &pb.AgentMessage{
		AgentId: c.agentUUID,
		Payload: &pb.AgentMessage_IncidentReport{
			IncidentReport: &pb.IncidentReport{
				IncidentId:     r.ID,
				CheckName:      r.CheckName,
				Service:        r.Service,
				Status:         r.Status,
				FailureCount:   int32(r.FailureCount),
				EventCreatedAt: r.EventCreatedAt.Unix(),
			},
		},
	}

	if err := stream.Send(msg); err != nil {
		return err
	}
	c.logger.Infow("incident report sent", "id", r.ID, "check", r.CheckName, "status", r.Status)
	return nil
}

func (c *ControlPlaneClient) sendRepairReport(stream grpc.BidiStreamingClient[pb.AgentMessage, pb.ServerMessage], r *RepairReport) error {
	msg := &pb.AgentMessage{
		AgentId: c.agentUUID,
		Payload: &pb.AgentMessage_RepairReport{
			RepairReport: &pb.RepairReport{
				IncidentId:     r.IncidentID,
				Rule:           r.Rule,
				Recipe:         r.Recipe,
				Status:         r.Status,
				EventCreatedAt: r.EventCreatedAt.Unix(),
				Duration:       int64(r.Duration.Seconds()),
			},
		},
	}

	if err := stream.Send(msg); err != nil {
		return err
	}
	c.logger.Infow("repair report sent", "incident_id", r.IncidentID, "rule", r.Rule, "recipe", r.Recipe, "status", r.Status)
	return nil
}

func (c *ControlPlaneClient) handleConfigUpdate(update *pb.ConfigUpdate) {
	c.logger.Infow("config update received (stubbed)",
		"version", update.ConfigVersion,
		"bundle_url", update.BundleUrl,
	)

	if c.onConfigUpdate != nil {
		c.onConfigUpdate(update.ConfigVersion, update.BundleUrl, update.Signature)
	}
}

func (c *ControlPlaneClient) handleCommandRequest(cmd *pb.CommandRequest) {
	c.logger.Infow("command request received (not yet implemented)",
		"command", cmd.Command,
		"params", cmd.Params,
	)
}
