package domain

import "context"

// VendorPrivate labels randomized/locally administered MACs, which have no registered vendor.
const VendorPrivate = "Private (randomized MAC)"

// LookupVendor returns the vendor for a MAC, or "" when unknown.
func LookupVendor(ctx context.Context, table VendorTable, mac MAC) string {
	if IsZeroMAC(ctx, mac) {
		return ""
	}
	if IsLocallyAdministered(ctx, mac) {
		return VendorPrivate
	}

	return table.ByOUI[OUIOf(ctx, mac)]
}

// ApplyVendors returns a copy of the inventory with every device's vendor resolved.
func ApplyVendors(ctx context.Context, inv Inventory, table VendorTable) Inventory {
	devices := make(map[MAC]Device, len(inv.Devices))
	for mac := range inv.Devices {
		device := inv.Devices[mac]
		device.Vendor = LookupVendor(ctx, table, mac)
		devices[mac] = device
	}

	return Inventory{Devices: devices, Unresolved: inv.Unresolved}
}
