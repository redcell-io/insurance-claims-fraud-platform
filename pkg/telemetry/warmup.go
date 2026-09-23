package telemetry

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

// WarmUp kicks off connecting conn immediately — grpc.NewClient dials
// lazily by default, only on the first real RPC — and blocks up to
// timeout for it to reach Ready, logging a warning rather than failing if
// it doesn't. A downstream service simply not being up yet at startup is
// normal in this repo (RUNBOOK.md: "a couple seconds' head start is
// enough, it doesn't have to be exact"), so this never returns an error.
//
// Call this right after dialing, before the service starts serving real
// traffic: without it, a cold TCP/HTTP2 handshake on the first real
// downstream call competes with that call's own timeout_ms — a real
// regression this pass's DAG-timeout enforcement surfaced (see
// DECISIONS.md #17).
func WarmUp(conn *grpc.ClientConn, logger *slog.Logger, target string, timeout time.Duration) {
	conn.Connect()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			return
		}
		if !conn.WaitForStateChange(ctx, state) {
			logger.Warn("connection not ready within warm-up timeout", "target", target, "state", conn.GetState().String())
			return
		}
	}
}
