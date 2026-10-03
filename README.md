# lantern

A terminal UI that finds the devices on your local network and tells you what they are.

lantern combines three sources and merges them into one row per device:

| Source | What it finds | Needs |
| --- | --- | --- |
| Active ARP sweep | Every host that answers on each local IPv4 subnet (MAC + IP) | root or `CAP_NET_RAW` |
| Kernel neighbor cache (`/proc/net/arp`) | Hosts this machine has recently talked to | nothing |
| mDNS / DNS-SD | Hostnames and services (printers, Chromecasts, AirPlay, SMB, SSH, HomeKit, ...) | nothing |

Vendors come from the IEEE OUI registry, which is embedded in the binary. Randomized (locally administered) MACs, such
as phones using private Wi-Fi addresses, are labeled as such rather than shown as unknown.

## Usage

```bash
make build
sudo setcap cap_net_raw+ep bin/lantern   # once, so the ARP sweep works without sudo
bin/lantern
```

Without raw-socket access, lantern still runs: it shows a banner and lists what the neighbor cache and mDNS found.

| Key | Action |
| --- | --- |
| `↑`/`↓` | Move |
| `enter` / `esc` | Show / hide device details |
| `s` | Cycle sort: IP, vendor, hostname, last seen |
| `r` | Rescan now (the inventory also refreshes every 30s) |
| `q` | Quit |

| Flag | Description |
| --- | --- |
| `--update-oui` | Download the latest IEEE registry to the user cache dir; later runs prefer it over the embedded copy |
| `--version` | Print the version |

### Configuration

All settings are environment variables (a `.env` file in the working directory is also read):

| Variable | Default | Description |
| --- | --- | --- |
| `LANTERN_SCAN_TIMEOUT` | `8s` | Upper bound for one scan |
| `LANTERN_ARP_REPLY_WAIT` | `2s` | How long to wait for ARP replies |
| `LANTERN_ARP_SEND_INTERVAL` | `2ms` | Pause between ARP requests |
| `LANTERN_ARP_MIN_PREFIX_BITS` | `22` | Subnets larger than this are narrowed around our own address |
| `LANTERN_NEIGHBOR_TABLE` | `/proc/net/arp` | Kernel neighbor cache |
| `LANTERN_MDNS_WAIT` | `3s` | How long to browse mDNS |
| `LANTERN_MDNS_SERVICES` | common types | Comma-separated DNS-SD service types to browse |
| `LANTERN_REFRESH_INTERVAL` | `30s` | Pause between automatic scans |
| `LANTERN_OUI_CACHE` | `<user cache>/lantern/oui.csv` | Where `--update-oui` stores the registry |
| `LANTERN_OUI_URL` | IEEE MA-L CSV | Registry source |
| `LANTERN_LOG_FILE` | (none) | Log file; logs are discarded when unset, since stdout belongs to the UI |
| `LANTERN_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |

## Design

The code is data-oriented: `internal/domain` holds plain data (`Observation`, `Device`, `Inventory`) and pure
functions over it (`Merge`, `ApplyVendors`, `SortedDevices`, `SweepTargets`). Services hold only interfaces and static
config, and everything is wired with `go.uber.org/dig` in `cmd/lantern/main.go`.

```
netif ─┐                                 ┌─ arpscan  (active ARP sweep)
       ├─> discovery.Service.Scan ──────┼─ neighbor (kernel cache)
oui ───┘    Merge + ApplyVendors         └─ mdns     (DNS-SD browse)
                     │
                     v
                    tui   (Bubble Tea; Update is a reducer over the inventory)
```

Each source implements `discovery.Source` and is registered in the `sources` dig group. To add a source, write an
adapter and add one `Provide` line. Sources that cannot run on the current system (`ErrInsufficientPrivileges`,
`ErrSourceUnavailable`) become warnings in the UI. Any other failure fails the scan.

## Development

Conventions for contributors and AI agents are in [AGENTS.md](AGENTS.md).

| Target | Description |
| --- | --- |
| `build` / `run` | Build or run `cmd/lantern` |
| `test`, `test-coverage`, `test-coverage-html` | Tests with race detector and shuffle; coverage reports |
| `mocks`, `mocks-regen`, `mocks-check` | Generate mocks; fail if committed mocks are stale |
| `fmt`, `lint`, `lint-fix` | golangci-lint formatters and linters |
| `oui` | Refresh the embedded IEEE vendor registry |
| `vuln`, `licenses` | govulncheck; fail on forbidden/restricted dependency licenses |
| `tidy`, `tidy-check` | `go mod tidy`; fail if not tidy |
| `ci` | Everything CI checks |

## License

[MIT](LICENSE)
