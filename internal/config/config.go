// Package config loads lantern's configuration from flags, env files and the environment.
// It is the only package allowed to read the environment; everything else receives config via DI.
package config

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
	"go.uber.org/dig"
)

const (
	appName      = "lantern"
	envFile      = ".env"
	ouiCacheFile = "oui.csv"
)

type Config struct {
	CLI  CLIConfig
	Scan ScanConfig
	OUI  OUIConfig
	UI   UIConfig
	Log  LogConfig
}

type CLIConfig struct {
	UpdateOUI bool
	Version   bool
}

type ScanConfig struct {
	// Timeout bounds one whole scan across every source.
	Timeout time.Duration `env:"LANTERN_SCAN_TIMEOUT" envDefault:"8s"`
	// ARPReplyWait is how long to listen for ARP replies after the last request was sent.
	ARPReplyWait time.Duration `env:"LANTERN_ARP_REPLY_WAIT" envDefault:"2s"`
	// ARPSendInterval paces ARP requests so a sweep does not flood the link.
	ARPSendInterval time.Duration `env:"LANTERN_ARP_SEND_INTERVAL" envDefault:"2ms"`
	// ARPMinPrefixBits caps the sweep size: larger subnets are narrowed around our own address (22 → 1022 hosts).
	ARPMinPrefixBits int `env:"LANTERN_ARP_MIN_PREFIX_BITS" envDefault:"22"`
	// NeighborTablePath is the kernel's neighbor (ARP) cache.
	NeighborTablePath string `env:"LANTERN_NEIGHBOR_TABLE" envDefault:"/proc/net/arp"`
	// MDNSWait is how long to browse each mDNS service type.
	MDNSWait time.Duration `env:"LANTERN_MDNS_WAIT" envDefault:"3s"`
	// MDNSServices are the DNS-SD service types to browse.
	MDNSServices []string `env:"LANTERN_MDNS_SERVICES" envDefault:"_http._tcp,_https._tcp,_ipp._tcp,_ipps._tcp,_printer._tcp,_pdl-datastream._tcp,_scanner._tcp,_uscan._tcp,_airplay._tcp,_raop._tcp,_googlecast._tcp,_spotify-connect._tcp,_sonos._tcp,_smb._tcp,_afpovertcp._tcp,_ssh._tcp,_sftp-ssh._tcp,_workstation._tcp,_device-info._tcp,_companion-link._tcp,_homekit._tcp,_hap._tcp,_matter._tcp,_hue._tcp,_esphomelib._tcp,_home-assistant._tcp,_mqtt._tcp,_amzn-wplay._tcp,_androidtvremote2._tcp,_rdlink._tcp"`
}

type OUIConfig struct {
	// CachePath is where --update-oui stores a fresh vendor database; defaults to the user cache dir.
	CachePath string `env:"LANTERN_OUI_CACHE"`
	// SourceURL is the IEEE MA-L registry in CSV form.
	SourceURL       string        `env:"LANTERN_OUI_URL"              envDefault:"https://standards-oui.ieee.org/oui/oui.csv"`
	DownloadTimeout time.Duration `env:"LANTERN_OUI_DOWNLOAD_TIMEOUT" envDefault:"60s"`
}

type UIConfig struct {
	// RefreshInterval is the pause between the end of one scan and the start of the next.
	RefreshInterval time.Duration `env:"LANTERN_REFRESH_INTERVAL" envDefault:"30s"`
}

type LogConfig struct {
	// Path is the log file; empty discards logs (stdout belongs to the TUI).
	Path  string `env:"LANTERN_LOG_FILE"`
	Level string `env:"LANTERN_LOG_LEVEL" envDefault:"info"`
}

type DepConfig struct {
	dig.Out

	*CLIConfig
	*ScanConfig
	*OUIConfig
	*UIConfig
	*LogConfig
}

// Load parses command-line args, the optional .env file and the environment.
func Load(_ context.Context, args []string, output io.Writer) (*Config, error) {
	cli, flagsErr := parseFlags(args, output)
	if flagsErr != nil {
		return nil, flagsErr
	}

	// The env file is optional, but a file that exists and can't be read is an error.
	if err := godotenv.Load(envFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("failed to load %s: %w", envFile, err)
	}

	var cfg Config
	cfg.CLI = cli
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config from environment: %w", err)
	}

	if cfg.OUI.CachePath != "" {
		return &cfg, nil
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve user cache dir: %w", err)
	}
	cfg.OUI.CachePath = filepath.Join(cacheDir, appName, ouiCacheFile)

	return &cfg, nil
}

func ParseDependenciesConfig(_ context.Context, cfg *Config) DepConfig {
	return DepConfig{
		Out:        dig.Out{},
		CLIConfig:  &cfg.CLI,
		ScanConfig: &cfg.Scan,
		OUIConfig:  &cfg.OUI,
		UIConfig:   &cfg.UI,
		LogConfig:  &cfg.Log,
	}
}

func parseFlags(args []string, output io.Writer) (CLIConfig, error) {
	flags := flag.NewFlagSet(appName, flag.ContinueOnError)
	flags.SetOutput(output)
	updateOUI := flags.Bool("update-oui", false, "download the latest IEEE vendor database and exit")
	version := flags.Bool("version", false, "print the version and exit")

	if err := flags.Parse(args); err != nil {
		return CLIConfig{}, fmt.Errorf("failed to parse flags: %w", err)
	}

	return CLIConfig{UpdateOUI: *updateOUI, Version: *version}, nil
}
