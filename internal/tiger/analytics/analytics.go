package analytics

import (
	"context"
	"encoding/json"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/logging"
	"go.uber.org/zap"
)

type Analytics struct {
	config    *config.Config
	projectID string
	client    *api.ClientWithResponses
}

// New initializes a new [Analytics] instance.
func New(cfg *config.Config, client *api.ClientWithResponses, projectID string) *Analytics {
	return &Analytics{
		config:    cfg,
		projectID: projectID,
		client:    client,
	}
}

// TryInit tries to load credentials to initialize an [Analytics]
// instance.  It returns an instance with a nil client if credentials do not
// exist or it otherwise fails to create a new client. This function is
// intended to be used when the caller does not otherwise need an API client to
// function, but would use one if available to track analytics events.
// Otherwise, call NewAnalytics directly.
func TryInit(cfg *config.Config) *Analytics {
	apiKey, projectID, err := config.GetCredentials()
	if err != nil {
		return New(cfg, nil, "")
	}

	client, err := api.NewTigerClient(cfg, apiKey)
	if err != nil {
		return New(cfg, nil, projectID)
	}

	return New(cfg, client, projectID)
}

type Option func(properties map[string]any)

func Property(key string, value any) Option {
	return func(properties map[string]any) {
		properties[key] = value
	}
}

func Fields(s any, ignore ...string) Option {
	return func(properties map[string]any) {
		out, err := json.Marshal(s)
		if err != nil {
			return
		}

		var fields map[string]any
		if err := json.Unmarshal(out, &fields); err != nil {
			return
		}

		for key, value := range fields {
			if slices.Contains(ignore, key) {
				continue
			}
			properties[key] = value
		}
	}
}

func NonZero[T comparable](key string, value T) Option {
	return func(properties map[string]any) {
		var zero T
		if value == zero {
			return
		}
		properties[key] = value
	}
}

var flagNameReplacer = strings.NewReplacer("-", "_")

func FlagSet(flagSet *pflag.FlagSet, ignore ...string) Option {
	return func(properties map[string]any) {
		flagSet.Visit(func(flag *pflag.Flag) {
			if slices.Contains(ignore, flag.Name) {
				return
			}
			key := flagNameReplacer.Replace(flag.Name)
			properties[key] = flag.Value.String()
		})
	}
}

func Flag(flag *pflag.Flag) Option {
	return func(properties map[string]any) {
		if !flag.Changed {
			return
		}
		key := flagNameReplacer.Replace(flag.Name)
		properties[key] = flag.Value.String()
	}
}

func Error(err error) Option {
	return func(properties map[string]any) {
		if err != nil {
			properties["success"] = false
			properties["error"] = err.Error()
		} else {
			properties["success"] = true
		}
	}
}

// Identify associates the provided properties with the user for the sake of
// analytics. It automatically includes common properties like ProjectID. The
// identification is only sent if the client is initialized and analytics are
// enabled in the config, otherwise it is skipped.
func (a *Analytics) Identify(event string, options ...Option) {
	// Create properties map with default/common properties
	properties := map[string]any{}
	if a.projectID != "" {
		properties["project_id"] = a.projectID
	}

	// Merge in user-provided properties (they can override common properties if needed)
	for _, option := range options {
		option(properties)
	}

	logger := logging.GetLogger().With(
		zap.Any("properties", properties),
	)

	// Check if analytics is disabled
	if !a.config.Analytics {
		if a.config.Debug {
			logger.Debug("Analytics identify skipped (analytics disabled)")
		}
		return
	}

	// Check for cases where the client was not initialized
	// (e.g. because API credentials are not available)
	if a.client == nil {
		if a.config.Debug {
			logger.Debug("Analytics identify skipped (client not initialized)")
		}
		return
	}

	// Set a 5 second timeout for tracking analytics events. We intentionally
	// use context.Background() here so we can track events even if a command
	// times out or is canceled.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Send the event
	resp, err := a.client.PostAnalyticsIdentifyWithResponse(ctx, api.PostAnalyticsIdentifyJSONRequestBody{
		Properties: &properties,
	})
	if err != nil {
		// Log error but don't fail the operation - analytics should never block user actions
		if a.config.Debug {
			logger.Debug("Failed to send analytics identify", zap.Error(err))
		}
		return
	}

	// Check if the API layer skipped the event
	if resp.JSON200 != nil && resp.JSON200.Status != nil && *resp.JSON200.Status == "skipped" {
		if a.config.Debug {
			logger.Debug("Analytics identify skipped (by API)")
		}
	}
}

// Track sends an analytics event with the provided event name and properties.
// It automatically includes common properties like ProjectID, OS, and
// architecture. Events are only sent if the client is initialized and
// analytics are enabled in the config, otherwise they are skipped.
func (a *Analytics) Track(event string, options ...Option) {
	// Create properties map with default/common properties
	properties := map[string]any{
		"version": config.Version,
		"os":      runtime.GOOS,
		"arch":    runtime.GOARCH,
	}
	if a.projectID != "" {
		properties["project_id"] = a.projectID
	}

	// Merge in user-provided properties (they can override common properties if needed)
	for _, option := range options {
		option(properties)
	}

	logger := logging.GetLogger().With(
		zap.String("event", event),
		zap.Any("properties", properties),
	)

	// Check if analytics is disabled
	if !a.config.Analytics {
		if a.config.Debug {
			logger.Debug("Analytics event skipped (analytics disabled)")
		}
		return
	}

	// Check for cases where the client was not initialized
	// (e.g. because API credentials are not available)
	if a.client == nil {
		if a.config.Debug {
			logger.Debug("Analytics event skipped (client not initialized)")
		}
		return
	}

	// Set a 5 second timeout for tracking analytics events. We intentionally
	// use context.Background() here so we can track events even if a command
	// times out or is canceled.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Send the event
	resp, err := a.client.PostAnalyticsTrackWithResponse(ctx, api.PostAnalyticsTrackJSONRequestBody{
		Event:      event,
		Properties: &properties,
	})
	if err != nil {
		// Log error but don't fail the operation - analytics should never block user actions
		if a.config.Debug {
			logger.Debug("Failed to send analytics event", zap.Error(err))
		}
		return
	}

	// Check if the API layer skipped the event
	if resp.JSON200 != nil && resp.JSON200.Status != nil && *resp.JSON200.Status == "skipped" {
		if a.config.Debug {
			logger.Debug("Analytics event skipped (by API)")
		}
	}
}
