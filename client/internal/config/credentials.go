package config

import (
	"fmt"
	"log/slog"
	"strings"
)

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
