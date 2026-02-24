package jimeng

import (
	"log/slog"
	"os"

	"github.com/volcengine/volc-sdk-golang/service/visual"
	"github.com/jimeng-relay/client/internal/config"
)

type Client struct {
	visual *visual.Visual
	config config.Config
	logger *slog.Logger
}

func NewClient(cfg config.Config) (*Client, error) {
	v := visual.NewInstance()

	v.Client.SetAccessKey(cfg.Credentials.AccessKey)
	v.Client.SetSecretKey(cfg.Credentials.SecretKey)

	v.SetRegion(cfg.Region)
	v.SetHost(cfg.Host)
	v.SetSchema(cfg.Scheme)

	if cfg.Timeout > 0 {
		v.Client.SetTimeout(cfg.Timeout)
	}

	var logger *slog.Logger
	if cfg.Debug {
		opts := &slog.HandlerOptions{Level: slog.LevelDebug}
		logger = slog.New(slog.NewJSONHandler(os.Stderr, opts))
		logger.Debug("debug mode enabled", "config", cfg.LogValue())
		logger.Debug("client initialized",
			"region", cfg.Region,
			"host", cfg.Host,
			"scheme", cfg.Scheme,
			"timeout", cfg.Timeout.String(),
		)
	}

	return &Client{
		visual: v,
		config: cfg,
		logger: logger,
	}, nil
}

func (c *Client) GetConfig() config.Config {
	return c.config
}

func (c *Client) debug(msg string, args ...any) {
	if c.logger != nil {
		c.logger.Debug(msg, args...)
	}
}
