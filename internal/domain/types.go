// Package domain holds lantern's data and the pure functions that transform it. It performs no I/O.
package domain

import (
	"errors"
	"net/netip"
	"time"
)

const (
	MACLength = 6
	ouiLength = 3
)

var (
	// ErrInsufficientPrivileges means a source needs raw socket access (root or CAP_NET_RAW).
	ErrInsufficientPrivileges = errors.New("insufficient privileges")
	// ErrSourceUnavailable means a source cannot run on this system (e.g. no neighbor table).
	ErrSourceUnavailable = errors.New("source unavailable")
	ErrInvalidMAC        = errors.New("invalid MAC address")
)

// MAC is a comparable EUI-48 address, usable as a map key.
type MAC [MACLength]byte

type OUI [ouiLength]byte

// SourceKind is a bit set of the discovery sources that saw a device.
type SourceKind uint8

const (
	SourceARP SourceKind = 1 << iota
	SourceNeighbor
	SourceMDNS
)

// Interface is a local network interface eligible for scanning.
type Interface struct {
	Name    string
	MAC     MAC
	Addr    netip.Addr // our own address on the network
	Network netip.Prefix
}

// Observation is one fact a source learned about a device. MAC is zero when the source doesn't know it.
type Observation struct {
	Source   SourceKind
	IP       netip.Addr
	MAC      MAC
	Hostname string
	Services []string
	SeenAt   time.Time
}

// Device is everything known about one host. MAC is zero for hosts only seen over mDNS.
type Device struct {
	MAC       MAC
	IPs       []netip.Addr
	Hostname  string
	Vendor    string
	Services  []string
	Sources   SourceKind
	FirstSeen time.Time
	LastSeen  time.Time
}

// Inventory is the merged result of every scan so far.
type Inventory struct {
	Devices map[MAC]Device
	// Unresolved holds hosts seen without a MAC (mDNS only), keyed by IP until a MAC source claims the IP.
	Unresolved map[netip.Addr]Device
}

// ScanResult is the outcome of one scan. Warnings are sources that could not run; the inventory is still valid.
type ScanResult struct {
	Inventory Inventory
	Warnings  []error
}

type VendorTable struct {
	ByOUI map[OUI]string
}
