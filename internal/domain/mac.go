package domain

import (
	"context"
	"fmt"
	"net"
)

// locallyAdministeredBit marks MACs not assigned by the IEEE: randomized/private addresses and VMs.
const locallyAdministeredBit = 0x02

func ParseMAC(ctx context.Context, text string) (MAC, error) {
	hw, err := net.ParseMAC(text)
	if err != nil {
		return MAC{}, fmt.Errorf("%w %q: %w", ErrInvalidMAC, text, err)
	}

	return MACFromHardwareAddr(ctx, hw)
}

func MACFromHardwareAddr(_ context.Context, hw net.HardwareAddr) (MAC, error) {
	if len(hw) != MACLength {
		return MAC{}, fmt.Errorf("%w: %d bytes, want %d", ErrInvalidMAC, len(hw), MACLength)
	}

	return MAC(hw), nil
}

func FormatMAC(ctx context.Context, mac MAC) string {
	if IsZeroMAC(ctx, mac) {
		return ""
	}

	return net.HardwareAddr(mac[:]).String()
}

func IsZeroMAC(_ context.Context, mac MAC) bool {
	return mac == MAC{}
}

func OUIOf(_ context.Context, mac MAC) OUI {
	return OUI(mac[:ouiLength])
}

func IsLocallyAdministered(_ context.Context, mac MAC) bool {
	return mac[0]&locallyAdministeredBit != 0
}
