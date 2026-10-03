// Package arpscan actively sweeps local IPv4 networks with ARP requests. It needs root or CAP_NET_RAW.
package arpscan

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"sync"
	"time"

	"github.com/mdlayher/arp"

	"github.com/davidbz/lantern/internal/config"
	"github.com/davidbz/lantern/internal/domain"
)

// Conn sends ARP requests and reads ARP packets. Read returns os.ErrDeadlineExceeded once ctx's deadline passes.
type Conn interface {
	Request(ctx context.Context, ip netip.Addr) error
	Read(ctx context.Context) (*arp.Packet, error)
}

type Scanner struct {
	cfg *config.ScanConfig
}

func NewScanner(_ context.Context, cfg *config.ScanConfig) *Scanner {
	return &Scanner{cfg: cfg}
}

func (s *Scanner) Discover(ctx context.Context, ifaces []domain.Interface) ([]domain.Observation, error) {
	results := make([][]domain.Observation, len(ifaces))
	failures := make([]error, len(ifaces))
	var wg sync.WaitGroup
	for i, iface := range ifaces {
		wg.Go(func() {
			results[i], failures[i] = s.sweepInterface(ctx, iface)
		})
	}
	wg.Wait()

	if err := errors.Join(failures...); err != nil {
		return nil, err
	}

	var observations []domain.Observation
	for _, result := range results {
		observations = append(observations, result...)
	}

	return observations, nil
}

func (s *Scanner) sweepInterface(ctx context.Context, iface domain.Interface) ([]domain.Observation, error) {
	systemIface, err := net.InterfaceByName(iface.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to look up interface %s: %w", iface.Name, err)
	}

	client, err := arp.Dial(systemIface)
	if errors.Is(err, os.ErrPermission) {
		return nil, fmt.Errorf("arp sweep on %s: %w: %w", iface.Name, domain.ErrInsufficientPrivileges, err)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to open arp socket on %s: %w", iface.Name, err)
	}
	defer client.Close()

	return Sweep(ctx, clientConn{client: client}, iface, s.cfg)
}

// Sweep sends a request to every target on the interface, then collects replies until the reply wait elapses.
func Sweep(
	ctx context.Context,
	conn Conn,
	iface domain.Interface,
	cfg *config.ScanConfig,
) ([]domain.Observation, error) {
	targets := domain.SweepTargets(ctx, iface, cfg.ARPMinPrefixBits)
	for _, target := range targets {
		if err := conn.Request(ctx, target); err != nil {
			return nil, fmt.Errorf("failed to send arp request to %s: %w", target, err)
		}
		if err := pause(ctx, cfg.ARPSendInterval); err != nil {
			return nil, err
		}
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.ARPReplyWait)
	defer cancel()

	return collectReplies(ctx, conn, iface)
}

func collectReplies(ctx context.Context, conn Conn, iface domain.Interface) ([]domain.Observation, error) {
	var observations []domain.Observation
	for {
		packet, err := conn.Read(ctx)
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return observations, nil
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read arp reply on %s: %w", iface.Name, err)
		}

		obs, ok := toObservation(ctx, packet, iface)
		if !ok {
			continue
		}
		observations = append(observations, obs)
	}
}

// toObservation keeps replies from hosts on the swept network; the socket also sees unrelated ARP traffic.
func toObservation(ctx context.Context, packet *arp.Packet, iface domain.Interface) (domain.Observation, bool) {
	if packet.Operation != arp.OperationReply || !iface.Network.Contains(packet.SenderIP) {
		return domain.Observation{}, false
	}

	mac, err := domain.MACFromHardwareAddr(ctx, packet.SenderHardwareAddr)
	if err != nil {
		return domain.Observation{}, false
	}

	return domain.Observation{
		Source:   domain.SourceARP,
		IP:       packet.SenderIP,
		MAC:      mac,
		Hostname: "",
		Services: nil,
		SeenAt:   time.Now(),
	}, true
}

func pause(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return nil
	}

	timer := time.NewTimer(interval)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return fmt.Errorf("arp sweep interrupted: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

// clientConn adapts *arp.Client to Conn, turning ctx deadlines into socket read deadlines.
type clientConn struct {
	client *arp.Client
}

func (c clientConn) Request(ctx context.Context, ip netip.Addr) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("arp request cancelled: %w", err)
	}
	if err := c.client.Request(ip); err != nil {
		return fmt.Errorf("arp client: %w", err)
	}

	return nil
}

func (c clientConn) Read(ctx context.Context) (*arp.Packet, error) {
	deadline, _ := ctx.Deadline() // the zero time clears the deadline
	if err := c.client.SetReadDeadline(deadline); err != nil {
		return nil, fmt.Errorf("failed to set arp read deadline: %w", err)
	}

	packet, _, err := c.client.Read()
	if err != nil {
		return nil, fmt.Errorf("arp client: %w", err)
	}

	return packet, nil
}
