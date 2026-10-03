package arpscan_test

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/mdlayher/arp"
	"github.com/mdlayher/ethernet"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/davidbz/lantern/internal/arpscan"
	"github.com/davidbz/lantern/internal/config"
	"github.com/davidbz/lantern/internal/domain"
	"github.com/davidbz/lantern/internal/mocks"
)

func TestSweep(t *testing.T) {
	iface := domain.Interface{
		Name:    "eth0",
		Addr:    netip.MustParseAddr("192.168.1.1"),
		Network: netip.MustParsePrefix("192.168.1.0/30"),
	}
	cfg := &config.ScanConfig{ARPMinPrefixBits: 22, ARPReplyWait: time.Second, ARPSendInterval: time.Microsecond}
	target := netip.MustParseAddr("192.168.1.2")
	hw := net.HardwareAddr{0x00, 0x1a, 0x2b, 0x3c, 0x4d, 0x5e}

	t.Run("collects replies from the swept network", func(t *testing.T) {
		conn := mocks.NewMockConn(t)
		conn.EXPECT().Request(mock.Anything, target).Return(nil)
		conn.EXPECT().Read(mock.Anything).Return(&arp.Packet{
			Operation: arp.OperationRequest, SenderIP: target, SenderHardwareAddr: hw,
		}, nil).Once()
		conn.EXPECT().Read(mock.Anything).Return(&arp.Packet{
			Operation: arp.OperationReply, SenderIP: netip.MustParseAddr("10.0.0.9"), SenderHardwareAddr: hw,
		}, nil).Once()
		conn.EXPECT().Read(mock.Anything).Return(&arp.Packet{
			Operation: arp.OperationReply, SenderIP: target, SenderHardwareAddr: net.HardwareAddr{1, 2},
		}, nil).Once()
		conn.EXPECT().Read(mock.Anything).Return(&arp.Packet{
			Operation: arp.OperationReply, SenderIP: target, SenderHardwareAddr: hw,
		}, nil).Once()
		conn.EXPECT().Read(mock.Anything).Return(nil, os.ErrDeadlineExceeded).Once()

		observations, err := arpscan.Sweep(t.Context(), conn, iface, cfg)

		require.NoError(t, err)
		require.Len(t, observations, 1)
		require.Equal(t, domain.SourceARP, observations[0].Source)
		require.Equal(t, target, observations[0].IP)
		require.Equal(t, domain.MAC(hw), observations[0].MAC)
	})

	t.Run("fails when a request can't be sent", func(t *testing.T) {
		conn := mocks.NewMockConn(t)
		boom := errors.New("boom")
		conn.EXPECT().Request(mock.Anything, target).Return(boom)

		_, err := arpscan.Sweep(t.Context(), conn, iface, cfg)

		require.ErrorIs(t, err, boom)
	})

	t.Run("fails on read errors", func(t *testing.T) {
		conn := mocks.NewMockConn(t)
		boom := errors.New("boom")
		conn.EXPECT().Request(mock.Anything, target).Return(nil)
		conn.EXPECT().Read(mock.Anything).Return(nil, boom)

		_, err := arpscan.Sweep(t.Context(), conn, iface, cfg)

		require.ErrorIs(t, err, boom)
	})

	t.Run("stops when the context is cancelled while sending", func(t *testing.T) {
		conn := mocks.NewMockConn(t)
		conn.EXPECT().Request(mock.Anything, target).Return(nil)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := arpscan.Sweep(ctx, conn, iface, &config.ScanConfig{ARPMinPrefixBits: 22, ARPSendInterval: time.Hour})

		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("reads until the reply wait, never past the context deadline", func(t *testing.T) {
		conn := mocks.NewMockConn(t)
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		t.Cleanup(cancel)
		ctxDeadline, _ := ctx.Deadline()
		conn.EXPECT().Request(mock.Anything, target).Return(nil)
		conn.EXPECT().Read(mock.MatchedBy(func(readCtx context.Context) bool {
			deadline, ok := readCtx.Deadline()
			return ok && deadline.Equal(ctxDeadline)
		})).Return(nil, os.ErrDeadlineExceeded)

		_, err := arpscan.Sweep(ctx, conn, iface, &config.ScanConfig{ARPMinPrefixBits: 22, ARPReplyWait: time.Hour})

		require.NoError(t, err)
	})
}

// packetConn is a net.PacketConn that records writes and serves queued frames, then a deadline error.
type packetConn struct {
	net.PacketConn

	frames      [][]byte
	written     int
	deadline    time.Time
	deadlineErr error
	writeErr    error
}

func (p *packetConn) ReadFrom(buf []byte) (int, net.Addr, error) {
	if len(p.frames) == 0 {
		return 0, nil, os.ErrDeadlineExceeded
	}
	frame := p.frames[0]
	p.frames = p.frames[1:]

	return copy(buf, frame), nil, nil
}

func (p *packetConn) WriteTo(buf []byte, _ net.Addr) (int, error) {
	p.written++
	return len(buf), p.writeErr
}

func (p *packetConn) SetReadDeadline(deadline time.Time) error {
	p.deadline = deadline
	return p.deadlineErr
}

func newClientConn(t *testing.T, packets *packetConn) arpscan.ClientConn {
	t.Helper()

	loopback, err := net.InterfaceByName("lo")
	if err != nil {
		t.Skip("no loopback interface to bind an arp client to")
	}
	ifi := *loopback
	ifi.HardwareAddr = net.HardwareAddr{0x00, 0x1a, 0x2b, 0x3c, 0x4d, 0x5e}
	client, err := arp.New(&ifi, packets)
	require.NoError(t, err)

	return arpscan.NewClientConn(client)
}

func replyFrame(t *testing.T, sender netip.Addr, hw net.HardwareAddr) []byte {
	t.Helper()

	packet, err := arp.NewPacket(arp.OperationReply, hw, sender, ethernet.Broadcast, netip.MustParseAddr("127.0.0.1"))
	require.NoError(t, err)
	payload, err := packet.MarshalBinary()
	require.NoError(t, err)
	frame, err := (&ethernet.Frame{
		Destination: ethernet.Broadcast, Source: hw, EtherType: ethernet.EtherTypeARP, Payload: payload,
	}).MarshalBinary()
	require.NoError(t, err)

	return frame
}

func TestClientConn(t *testing.T) {
	target := netip.MustParseAddr("192.168.1.2")
	hw := net.HardwareAddr{0x00, 0x1a, 0x2b, 0x3c, 0x4d, 0x5f}

	t.Run("sends requests", func(t *testing.T) {
		packets := &packetConn{}

		require.NoError(t, newClientConn(t, packets).Request(t.Context(), target))
		require.Equal(t, 1, packets.written)
	})

	t.Run("does not send once the context is done", func(t *testing.T) {
		packets := &packetConn{}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		err := newClientConn(t, packets).Request(ctx, target)

		require.ErrorIs(t, err, context.Canceled)
		require.Zero(t, packets.written)
	})

	t.Run("reports send failures", func(t *testing.T) {
		boom := errors.New("boom")

		err := newClientConn(t, &packetConn{writeErr: boom}).Request(t.Context(), target)

		require.ErrorIs(t, err, boom)
	})

	t.Run("reads packets with the context deadline", func(t *testing.T) {
		packets := &packetConn{frames: [][]byte{replyFrame(t, target, hw)}}
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		t.Cleanup(cancel)
		deadline, _ := ctx.Deadline()
		conn := newClientConn(t, packets)

		packet, err := conn.Read(ctx)
		require.NoError(t, err)
		require.Equal(t, target, packet.SenderIP)
		require.Equal(t, deadline, packets.deadline)

		_, err = conn.Read(ctx)
		require.ErrorIs(t, err, os.ErrDeadlineExceeded)
	})

	t.Run("fails when the deadline can't be set", func(t *testing.T) {
		boom := errors.New("boom")

		_, err := newClientConn(t, &packetConn{deadlineErr: boom}).Read(t.Context())

		require.ErrorIs(t, err, boom)
	})
}

func TestScannerDiscover(t *testing.T) {
	t.Run("an unknown interface is an error", func(t *testing.T) {
		scanner := arpscan.NewScanner(t.Context(), &config.ScanConfig{})

		_, err := scanner.Discover(t.Context(), []domain.Interface{{Name: "lantern-does-not-exist0"}})

		require.Error(t, err)
	})

	t.Run("without raw socket access the sweep needs privileges", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root can open raw sockets")
		}

		_, err := arpscan.NewScanner(t.Context(), &config.ScanConfig{}).
			Discover(t.Context(), []domain.Interface{{Name: "lo"}})

		require.ErrorIs(t, err, domain.ErrInsufficientPrivileges)
	})

	t.Run("no interfaces means no observations", func(t *testing.T) {
		observations, err := arpscan.NewScanner(t.Context(), &config.ScanConfig{}).Discover(t.Context(), nil)

		require.NoError(t, err)
		require.Empty(t, observations)
	})
}
