package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jimeng-relay/client/internal/config"
	"github.com/jimeng-relay/client/internal/jimeng"
	"github.com/jimeng-relay/client/internal/output"
	"github.com/spf13/cobra"
)

type rootFlagValues struct {
	format    string
	accessKey string
	secretKey string
	region    string
	host      string
	timeout   string
	debug     bool
}

var rootFlags rootFlagValues

var rootCmd = &cobra.Command{
	Use:          "jimeng",
	Short:        "Jimeng Relay CLI client",
	Long:         `A CLI client for Jimeng Relay service to submit, query, wait and download tasks.`,
	Version:      "0.1.0",
	SilenceUsage: true,
}

func RootCmd() *cobra.Command {
	return rootCmd
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&rootFlags.format, "format", string(output.FormatText), "Output format: text|json")
	rootCmd.PersistentFlags().StringVar(&rootFlags.accessKey, "access-key", "", fmt.Sprintf("Volcengine access key (overrides %s)", config.EnvAccessKey))
	rootCmd.PersistentFlags().StringVar(&rootFlags.secretKey, "secret-key", "", fmt.Sprintf("Volcengine secret key (overrides %s)", config.EnvSecretKey))
	rootCmd.PersistentFlags().StringVar(&rootFlags.region, "region", "", fmt.Sprintf("Volcengine region (overrides %s)", config.EnvRegion))
	rootCmd.PersistentFlags().StringVar(&rootFlags.host, "host", "", fmt.Sprintf("Volcengine API host (overrides %s)", config.EnvHost))
	rootCmd.PersistentFlags().StringVar(&rootFlags.timeout, "timeout", "", fmt.Sprintf("API request timeout duration, e.g. 30s, 2m (overrides %s)", config.EnvTimeout))
	rootCmd.PersistentFlags().BoolVar(&rootFlags.debug, "debug", false, "Enable debug logging")
}

func flagChanged(cmd *cobra.Command, name string) bool {
	if cmd == nil {
		return false
	}
	if f := cmd.Flags().Lookup(name); f != nil {
		return f.Changed
	}
	if f := cmd.InheritedFlags().Lookup(name); f != nil {
		return f.Changed
	}
	if f := cmd.PersistentFlags().Lookup(name); f != nil {
		return f.Changed
	}
	return false
}

func newFormatterFromRootFlags() (*output.Formatter, error) {
	f := strings.TrimSpace(rootFlags.format)
	if f == "" {
		f = string(output.FormatText)
	}
	switch f {
	case string(output.FormatJSON), string(output.FormatText):
		return output.NewFormatter(output.Format(f)), nil
	default:
		return nil, fmt.Errorf("invalid --format: %q (supported: text|json)", f)
	}
}

func loadConfigFromRootFlags(cmd *cobra.Command) (config.Config, error) {
	var opts config.Options

	if flagChanged(cmd, "access-key") {
		opts.AccessKey = &rootFlags.accessKey
	}
	if flagChanged(cmd, "secret-key") {
		opts.SecretKey = &rootFlags.secretKey
	}
	if flagChanged(cmd, "region") {
		opts.Region = &rootFlags.region
	}
	if flagChanged(cmd, "host") {
		opts.Host = &rootFlags.host
	}
	if flagChanged(cmd, "timeout") {
		raw := strings.TrimSpace(rootFlags.timeout)
		if raw == "" {
			return config.Config{}, fmt.Errorf("--timeout must not be empty")
		}
		d, err := time.ParseDuration(raw)
		if err != nil {
			return config.Config{}, fmt.Errorf("invalid --timeout: %w", err)
		}
		opts.Timeout = &d
	}

	opts.Debug = rootFlags.debug

	return config.Load(opts)
}

func newClientAndFormatter(cmd *cobra.Command) (*jimeng.Client, *output.Formatter, error) {
	cfg, err := loadConfigFromRootFlags(cmd)
	if err != nil {
		return nil, nil, err
	}

	client, err := jimeng.NewClient(cfg)
	if err != nil {
		return nil, nil, err
	}

	formatter, err := newFormatterFromRootFlags()
	if err != nil {
		return nil, nil, err
	}

	return client, formatter, nil
}
