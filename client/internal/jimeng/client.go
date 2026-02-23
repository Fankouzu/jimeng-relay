package jimeng

import (
	"github.com/volcengine/volc-sdk-golang/service/visual"

	"github.com/jimeng-relay/client/internal/config"
)

type Client struct {
	visual *visual.Visual
	config config.Config
}

func NewClient(cfg config.Config) (*Client, error) {
	v := visual.DefaultInstance

	v.Client.SetAccessKey(cfg.Credentials.AccessKey)
	v.Client.SetSecretKey(cfg.Credentials.SecretKey)

	v.SetRegion(cfg.Region)
	v.SetHost(cfg.Host)

	if cfg.Timeout > 0 {
		v.Client.SetTimeout(cfg.Timeout)
	}

	return &Client{
		visual: v,
		config: cfg,
	}, nil
}

func (c *Client) GetConfig() config.Config {
	return c.config
}
