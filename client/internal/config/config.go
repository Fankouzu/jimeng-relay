package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

const (
	EnvAccessKey = "VOLC_ACCESSKEY"
	EnvSecretKey = "VOLC_SECRETKEY"
	EnvRegion    = "VOLC_REGION"
	EnvHost      = "VOLC_HOST"
	EnvTimeout   = "VOLC_TIMEOUT"
	EnvScheme    = "VOLC_SCHEME"
)

const (
	DefaultRegion  = "cn-north-1"
	DefaultHost    = "visual.volcengineapi.com"
	DefaultTimeout = 30 * time.Second
	DefaultScheme  = "https"
)

type Config struct {
	Credentials Credentials
	Region      string
	Host        string
	Scheme      string
	Timeout     time.Duration
	Debug       bool
}

func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Any("credentials", c.Credentials),
		slog.String("region", c.Region),
		slog.String("host", c.Host),
		slog.String("scheme", c.Scheme),
		slog.String("timeout", c.Timeout.String()),
	)
}

type Options struct {
	AccessKey  *string
	SecretKey  *string
	Region     *string
	Host       *string
	Scheme     *string
	Timeout    *time.Duration
	ConfigFile *string
	Debug      bool
}

func Load(opts Options) (Config, error) {
	cfg := Config{
		Region:  DefaultRegion,
		Host:    DefaultHost,
		Scheme:  DefaultScheme,
		Timeout: DefaultTimeout,
	}
	envFile := ".env"
	if opts.ConfigFile != nil && *opts.ConfigFile != "" {
		envFile = *opts.ConfigFile
	}
	if err := loadEnvFile(envFile); err != nil {
		return Config{}, err
	}
	if v, ok := lookupEnvNonEmpty(EnvRegion); ok {
		cfg.Region = v
	}
	if v, ok := lookupEnvNonEmpty(EnvHost); ok {
		cfg.Host = normalizeHost(v)
	}
	if v, ok := os.LookupEnv(EnvScheme); ok {
		cfg.Scheme = strings.TrimSpace(v)
	}
	if v, ok := lookupEnvNonEmpty(EnvTimeout); ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid %s: %w", EnvTimeout, err)
		}
		cfg.Timeout = d
	}
	if opts.Region != nil {
		v := strings.TrimSpace(*opts.Region)
		if v == "" {
			return Config{}, fmt.Errorf("region must not be empty")
		}
		cfg.Region = v
	}
	if opts.Host != nil {
		v := strings.TrimSpace(*opts.Host)
		if v == "" {
			return Config{}, fmt.Errorf("host must not be empty")
		}
		cfg.Host = normalizeHost(v)
	}
	if opts.Scheme != nil {
		v := strings.TrimSpace(*opts.Scheme)
		if v == "" {
			return Config{}, fmt.Errorf("scheme must not be empty")
		}
		cfg.Scheme = v
	}
	if opts.Timeout != nil {
		if *opts.Timeout <= 0 {
			return Config{}, fmt.Errorf("timeout must be positive")
		}
		cfg.Timeout = *opts.Timeout
	}
	if err := validateScheme(cfg.Scheme); err != nil {
		return Config{}, err
	}
	creds, err := LoadCredentials(CredentialsOptions{
		AccessKey: opts.AccessKey,
		SecretKey: opts.SecretKey,
	})
	if err != nil {
		return Config{}, err
	}
	cfg.Credentials = creds
	cfg.Debug = opts.Debug
	return cfg, nil
}

func normalizeHost(host string) string {
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimRight(host, "/")
	return host
}

func validateScheme(scheme string) error {
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("invalid %s: %q (must be http or https)", EnvScheme, scheme)
	}
	return nil
}

func lookupEnvNonEmpty(key string) (string, bool) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return "", false
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return "", false
	}
	return v, true
}

func loadEnvFile(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read env file %s: %w", path, err)
	}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key != "" {
			if _, ok := os.LookupEnv(key); !ok {
				os.Setenv(key, value)
			}
		}
	}
	return nil
}
