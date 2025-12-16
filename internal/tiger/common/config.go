package common

import (
	"context"
	"fmt"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/config"
)

// Config is a convenience wrapper around [config.Config] that adds an API
// client and the current project ID. Since most commands require all of these
// to function, it is often easier to load them and pass them around together.
// Functions that only require a config but not a client (i.e. functions that
// do not make any API calls) should call [config.Load] directly instead.
type Config struct {
	*config.Config
	Client    *api.ClientWithResponses `json:"-"`
	ProjectID string                   `json:"-"`
}

func LoadConfig(ctx context.Context) (*Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	client, projectID, err := NewAPIClient(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &Config{
		Config:    cfg,
		Client:    client,
		ProjectID: projectID,
	}, nil
}
