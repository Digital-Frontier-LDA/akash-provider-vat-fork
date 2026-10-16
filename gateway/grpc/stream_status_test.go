package grpc

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	tpubsub "github.com/troian/pubsub"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	providerv1 "pkg.akt.dev/go/provider/v1"

	"github.com/akash-network/provider/tools/fromctx"
	ptypes "github.com/akash-network/provider/types"
)

type fakeStatusStream struct {
	grpc.ServerStream
	ctx  context.Context
	sent chan *providerv1.Status
}

func (s *fakeStatusStream) Context() context.Context { return s.ctx }

func (s *fakeStatusStream) Send(m *providerv1.Status) error {
	s.sent <- m
	return nil
}

// TestStreamStatusReleasesSubscriptionWhenClientLeaves pins the fix for the provider's
// inventory-snapshot memory leak (akash-providers-IaC#202, round 2).
//
// StreamStatus subscribes to provider-status on the process-wide pubsub bus. troian/pubsub
// gives every subscription its own goroutine, which queues each message in an unbounded
// slice until the subscriber reads it. StreamStatus never unsubscribed, so when a client
// went away its subscription stayed, and from then on every provider status was queued for
// a reader that no longer existed. Each status carries a full inventory snapshot (one per
// inventory change, about 1,000 an hour on oni_dc), so one dropped stream grew the heap by
// ~100 MB/h for the life of the process.
func TestStreamStatusReleasesSubscriptionWhenClientLeaves(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The capacity the provider runs with (cmd/provider-services/cmd/run.go).
	bus := tpubsub.New(ctx, 1000)
	defer bus.Shutdown()

	topics := []string{ptypes.PubSubTopicProviderStatus}
	gm := &grpcProviderV1{ctx: context.WithValue(ctx, fromctx.CtxKeyPubSub, bus)}

	// A retained status reaches the stream as soon as it subscribes, which shows the
	// subscription is live, so the check below cannot pass for want of one.
	bus.Pub(providerv1.Status{}, topics, tpubsub.WithRetain())

	streamCtx, leave := context.WithCancel(ctx)
	stream := &fakeStatusStream{ctx: streamCtx, sent: make(chan *providerv1.Status, 1)}

	done := make(chan error, 1)
	go func() { done <- gm.StreamStatus(&emptypb.Empty{}, stream) }()

	select {
	case <-stream.sent:
	case <-time.After(5 * time.Second):
		t.Fatal("the stream never received the retained provider status")
	}

	// The client goes away.
	leave()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("StreamStatus did not return after its client left")
	}

	// Clear the retained status, then publish one the way statusRun does, with a
	// finalizer on the cluster status it points at (in production that holds the
	// inventory snapshot).
	bus.Pub(nil, topics)

	finalized := make(chan struct{})
	cluster := &providerv1.ClusterStatus{}
	runtime.SetFinalizer(cluster, func(*providerv1.ClusterStatus) { close(finalized) })
	bus.Pub(providerv1.Status{Cluster: cluster}, topics)
	cluster = nil //nolint: ineffassign,wastedassign

	deadline := time.After(5 * time.Second)
	for {
		runtime.GC()
		select {
		case <-finalized:
			return
		case <-deadline:
			t.Fatal("a provider status published after the client left is still held: " +
				"StreamStatus left its subscription on the bus")
		case <-time.After(20 * time.Millisecond):
		}
	}
}
