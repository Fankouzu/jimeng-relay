package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	EnvAccessKey                 = "VOLC_ACCESSKEY"
	EnvSecretKey                 = "VOLC_SECRETKEY"
	EnvRegion                    = "VOLC_REGION"
	EnvHost                      = "VOLC_HOST"
	EnvTimeout                   = "VOLC_TIMEOUT"
	EnvServerPort                = "SERVER_PORT"
	EnvDatabaseType              = "DATABASE_TYPE"
	EnvDatabaseURL               = "DATABASE_URL"
	EnvAPIKeyEncryptionKey       = "API_KEY_ENCRYPTION_KEY"
	EnvUpstreamMaxConcurrent     = "UPSTREAM_MAX_CONCURRENT"
	EnvUpstreamMaxQueue          = "UPSTREAM_MAX_QUEUE"
	EnvUpstreamSubmitMinInterval = "UPSTREAM_SUBMIT_MIN_INTERVAL"
	EnvPerKeyMaxConcurrent     = "PER_KEY_MAX_CONCURRENT"
	EnvPerKeyMaxQueue          = "PER_KEY_MAX_QUEUE"
)

const (
	DefaultRegion                    = "cn-north-1"
	DefaultHost                      = "visual.volcengineapi.com"
	DefaultTimeout                   = 30 * time.Second
	DefaultServerPort                = "8080"
	DefaultDatabaseType              = "sqlite"
	DefaultDatabaseURL               = "./jimeng-relay.db"
	DefaultUpstreamMaxConcurrent     = 1
	DefaultUpstreamMaxQueue          = 100
	DefaultUpstreamSubmitMinInterval = 0 * time.Second
	DefaultPerKeyMaxConcurrent     = 1
	DefaultPerKeyMaxQueue          = 1
)

type Config struct {
	Credentials               Credentials
	Region                    string
	Host                      string
	Timeout                   time.Duration
	ServerPort                string
	DatabaseType              string
	DatabaseURL               string
	APIKeyEncryptionKey       string
	UpstreamMaxConcurrent     int
	UpstreamMaxQueue          int
	UpstreamSubmitMinInterval time.Duration
	PerKeyMaxConcurrent     int
	PerKeyMaxQueue          int
}

func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Any("credentials", c.Credentials),
		slog.String("region", c.Region),
		slog.String("host", c.Host),
		slog.String("timeout", c.Timeout.String()),
		slog.String("server_port", c.ServerPort),
		slog.String("database_type", c.DatabaseType),
		slog.String("database_url", c.DatabaseURL),
		slog.String("api_key_encryption_key", "***"),
		slog.Int("upstream_max_concurrent", c.UpstreamMaxConcurrent),
		slog.Int("upstream_max_queue", c.UpstreamMaxQueue),
		slog.String("upstream_submit_min_interval", c.UpstreamSubmitMinInterval.String()),
		slog.Int("per_key_max_concurrent", c.PerKeyMaxConcurrent),
		slog.Int("per_key_max_queue", c.PerKeyMaxQueue),
	)
}

type Options struct {
	AccessKey           *string
	SecretKey           *string
	Region              *string
	Host                *string
	Timeout             *time.Duration
	ServerPort          *string
	DatabaseType        *string
	DatabaseURL         *string
	APIKeyEncryptionKey *string
	ConfigFile          *string
}

func Load(opts Options) (Config, error) {
	cfg := Config{
		Region:                    DefaultRegion,
		Host:                      DefaultHost,
		Timeout:                   DefaultTimeout,
		ServerPort:                DefaultServerPort,
		DatabaseType:              DefaultDatabaseType,
		DatabaseURL:               DefaultDatabaseURL,
		UpstreamMaxConcurrent:     DefaultUpstreamMaxConcurrent,
		UpstreamMaxQueue:          DefaultUpstreamMaxQueue,
		UpstreamSubmitMinInterval: DefaultUpstreamSubmitMinInterval,
		PerKeyMaxConcurrent:     DefaultPerKeyMaxConcurrent,
		PerKeyMaxQueue:          DefaultPerKeyMaxQueue,
	}

	envFile := ".env"
	if opts.ConfigFile != nil && *opts.ConfigFile != "" {
		envFile = *opts.ConfigFile
	}
	if err := LoadEnvFile(envFile); err != nil {
		return Config{}, err
	}

	if v, ok := lookupEnvNonEmpty(EnvRegion); ok {
		cfg.Region = v
	}
	if v, ok := lookupEnvNonEmpty(EnvHost); ok {
		cfg.Host = v
	}
	if v, ok := lookupEnvNonEmpty(EnvTimeout); ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid %s: %w", EnvTimeout, err)
		}
		cfg.Timeout = d
	}
	if v, ok := lookupEnvNonEmpty(EnvServerPort); ok {
		cfg.ServerPort = v
	}
	if v, ok := lookupEnvNonEmpty(EnvDatabaseType); ok {
		cfg.DatabaseType = v
	}
	if v, ok := lookupEnvNonEmpty(EnvDatabaseURL); ok {
		cfg.DatabaseURL = v
	}
	if v, ok := lookupEnvNonEmpty(EnvAPIKeyEncryptionKey); ok {
		cfg.APIKeyEncryptionKey = v
	}
	if v, ok := lookupEnvNonEmpty(EnvUpstreamMaxConcurrent); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid %s: %w", EnvUpstreamMaxConcurrent, err)
		}
		cfg.UpstreamMaxConcurrent = n
	}
	if v, ok := lookupEnvNonEmpty(EnvUpstreamMaxQueue); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid %s: %w", EnvUpstreamMaxQueue, err)
		}
		cfg.UpstreamMaxQueue = n
	}
	if v, ok := lookupEnvNonEmpty(EnvUpstreamSubmitMinInterval); ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid %s: %w", EnvUpstreamSubmitMinInterval, err)
		}
		if d < 0 {
			return Config{}, fmt.Errorf("%s must be >= 0", EnvUpstreamSubmitMinInterval)
		}
		cfg.UpstreamSubmitMinInterval = d
	}
	if v, ok := lookupEnvNonEmpty(EnvPerKeyMaxConcurrent); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid %s: %w", EnvPerKeyMaxConcurrent, err)
		}
		cfg.PerKeyMaxConcurrent = n
	}
	if v, ok := lookupEnvNonEmpty(EnvPerKeyMaxQueue); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid %s: %w", EnvPerKeyMaxQueue, err)
		}
		cfg.PerKeyMaxQueue = n
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
		cfg.Host = v
	}
	if opts.Timeout != nil {
		if *opts.Timeout <= 0 {
			return Config{}, fmt.Errorf("timeout must be positive")
		}
		cfg.Timeout = *opts.Timeout
	}
	if opts.ServerPort != nil {
		v := strings.TrimSpace(*opts.ServerPort)
		if v == "" {
			return Config{}, fmt.Errorf("server port must not be empty")
		}
		cfg.ServerPort = v
	}
	if opts.DatabaseType != nil {
		v := strings.TrimSpace(*opts.DatabaseType)
		if v == "" {
			return Config{}, fmt.Errorf("database type must not be empty")
		}
		cfg.DatabaseType = v
	}
	if opts.DatabaseURL != nil {
		v := strings.TrimSpace(*opts.DatabaseURL)
		if v == "" {
			return Config{}, fmt.Errorf("database URL must not be empty")
		}
		cfg.DatabaseURL = v
	}
	if opts.APIKeyEncryptionKey != nil {
		v := strings.TrimSpace(*opts.APIKeyEncryptionKey)
		if v == "" {
			return Config{}, fmt.Errorf("api key encryption key must not be empty")
		}
		cfg.APIKeyEncryptionKey = v
	}

	creds, err := LoadCredentials(CredentialsOptions{
		AccessKey: opts.AccessKey,
		SecretKey: opts.SecretKey,
	})
	if err != nil {
		return Config{}, err
	}
	cfg.Credentials = creds
	if strings.TrimSpace(cfg.APIKeyEncryptionKey) == "" {
		return Config{}, fmt.Errorf("%s is required", EnvAPIKeyEncryptionKey)
	}

	return cfg, nil
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

// LoadEnvFile loads environment variables from a file into os.Environ.
// It skips lines that are empty, comments, or already set in the environment.
func LoadEnvFile(path string) error {
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

type Credentials struct {
	AccessKey string
	SecretKey string
}

func (c Credentials) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("access_key", redactAccessKey(c.AccessKey)),
		slog.String("secret_key", "***"),
	)
}

type CredentialsOptions struct {
	AccessKey *string
	SecretKey *string
}

type MissingCredentialsError struct {
	Missing []string
}

func (e *MissingCredentialsError) Error() string {
	return fmt.Sprintf("missing credentials: %s", strings.Join(e.Missing, ", "))
}

func LoadCredentials(opts CredentialsOptions) (Credentials, error) {
	var c Credentials

	if v, ok := lookupEnvNonEmpty(EnvAccessKey); ok {
		c.AccessKey = v
	}
	if v, ok := lookupEnvNonEmpty(EnvSecretKey); ok {
		c.SecretKey = v
	}

	if opts.AccessKey != nil {
		c.AccessKey = strings.TrimSpace(*opts.AccessKey)
	}
	if opts.SecretKey != nil {
		c.SecretKey = strings.TrimSpace(*opts.SecretKey)
	}

	missing := make([]string, 0, 2)
	if c.AccessKey == "" {
		missing = append(missing, EnvAccessKey)
	}
	if c.SecretKey == "" {
		missing = append(missing, EnvSecretKey)
	}
	if len(missing) > 0 {
		return Credentials{}, &MissingCredentialsError{Missing: missing}
	}

	return c, nil
}

func redactAccessKey(ak string) string {
	ak = strings.TrimSpace(ak)
	if ak == "" {
		return ""
	}
	if len(ak) <= 4 {
		return ak + "..."
	}
	return ak[:4] + "..."
}
