package telemetry

import (
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

// KeepaliveDialOption pings a downstream service's connection periodically
// so grpc-go doesn't drop it back to IDLE after a period of no RPCs
// (default grpc-go behavior, default idle_timeout 30m) and so a peer that
// silently disappeared — killed mid-connection, or a connection an
// intermediate LB/NAT evicted without a clean close — is detected and
// reconnected before a real request hits it, rather than that request
// paying for the reconnect itself and racing its own timeout_ms (see
// WarmUp's doc comment and DECISIONS.md #17/#19 for the startup-side
// version of this same race). PermitWithoutStream is required here:
// without it, grpc-go only sends keepalive pings while an RPC is in
// flight, which never happens during the long idle gaps this is meant to
// cover.
//
// Time must stay comfortably above every server's keepalive enforcement
// policy (grpc-go's and grpc-java's shared default MinTime is 5 minutes)
// — a tighter interval gets the connection killed with GOAWAY
// ENHANCE_YOUR_CALM/too_many_pings instead of kept healthy, which is
// worse than no keepalive at all. 10 minutes clears that with margin
// while still resetting the idle timer well before the 30-minute default
// idle_timeout.
func KeepaliveDialOption() grpc.DialOption {
	return grpc.WithKeepaliveParams(keepalive.ClientParameters{
		Time:                10 * time.Minute,
		Timeout:             20 * time.Second,
		PermitWithoutStream: true,
	})
}
