package analytics

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"slices"
	"strconv"
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

// Option is a function that modifies analytics event properties. Options are
// passed to Track and Identify methods to customize the data sent with events.
type Option func(properties map[string]any)

// Property creates an Option that adds a single key-value pair to the event
// properties. This is useful for adding custom analytics data that isn't
// covered by other Option functions.
func Property(key string, value any) Option {
	return func(properties map[string]any) {
		properties[key] = value
	}
}

// NonZero creates an Option that adds a property only if the value is not the
// zero value for its type. This is useful for optional parameters where you
// only want to track them when they're explicitly set.
//
// For example, NonZero("timeout", 0) won't add the property, but
// NonZero("timeout", 30) will add timeout: 30.
func NonZero[T comparable](key string, value T) Option {
	return func(properties map[string]any) {
		var zero T
		if value == zero {
			return
		}
		properties[key] = value
	}
}

// Fields creates an Option that extracts all exported fields from a struct
// and adds them as properties. This uses JSON marshaling to convert the struct
// to key-value pairs. Fields specified in the ignore list are skipped.
//
// This is useful for including structured data like MCP tool call arguments in
// analytics events without manually specifying each field.
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

		Map(fields, ignore...)
	}
}

func Map(m map[string]any, ignore ...string) Option {
	return func(properties map[string]any) {
		for key, value := range m {
			if slices.Contains(ignore, key) {
				continue
			}
			properties[key] = value
		}
	}
}

// flagNameReplacer converts flag names from kebab-case to snake_case for
// consistent property naming in analytics events.
var flagNameReplacer = strings.NewReplacer("-", "_")

// FlagSet creates an Option that adds all flags that were explicitly set by
// the user (via Visit). Flag names are converted from kebab-case to snake_case
// (e.g., "no-wait" becomes "no_wait"). Flags in the ignore list are skipped.
//
// This is useful for tracking which flags users actually use when running commands.
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

// Flag creates an Option that adds a single flag's value if it was explicitly
// set by the user. The flag name is converted from kebab-case to snake_case.
//
// This is useful when you only want to track specific flags rather than all
// flags in a flag set.
func Flag(flag *pflag.Flag) Option {
	return func(properties map[string]any) {
		if !flag.Changed {
			return
		}
		key := flagNameReplacer.Replace(flag.Name)
		properties[key] = flag.Value.String()
	}
}

// Error creates an Option that adds success and error information to event
// properties. If err is nil, sets success: true. If err is not nil, sets
// success: false and includes the error message.
//
// This is commonly used at the end of command execution to track whether
// operations succeeded or failed, and what errors occurred.
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
	if !a.enabled() {
		logger.Debug("Analytics identify skipped (analytics disabled)")
		return
	}

	// Check for cases where the client was not initialized
	// (e.g. because API credentials are not available)
	if a.client == nil {
		logger.Debug("Analytics identify skipped (client not initialized)")
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
		logger.Debug("Failed to send analytics identify", zap.Error(err))
		return
	}

	if resp.JSON200 == nil || resp.JSON200.Status == nil {
		logger.Debug("Failed to retrieve response from analytics endpoint")
		return
	}

	logger.Debug("Analytics identify sent", zap.String("status", *resp.JSON200.Status))
}

// Track sends an analytics event with the provided event name and properties.
// It automatically includes common properties like ProjectID, OS, and
// architecture. Events are only sent if the client is initialized and
// analytics are enabled in the config, otherwise they are skipped.
func (a *Analytics) Track(event string, options ...Option) {
	// Create properties map with default/common properties
	properties := map[string]any{
		"source":  "cli",
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
	if !a.enabled() {
		logger.Debug("Analytics event skipped (analytics disabled)")
		return
	}

	// Check for cases where the client was not initialized
	// (e.g. because API credentials are not available)
	if a.client == nil {
		logger.Debug("Analytics event skipped (client not initialized)")
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
		logger.Debug("Failed to send analytics event", zap.Error(err))
		return
	}

	if resp.JSON200 == nil || resp.JSON200.Status == nil {
		logger.Debug("Failed to retrieve response from analytics endpoint")
		return
	}

	logger.Debug("Analytics event sent", zap.String("status", *resp.JSON200.Status))
}

func (a *Analytics) enabled() bool {
	if envVarIsTrue("DO_NOT_TRACK") ||
		envVarIsTrue("NO_TELEMETRY") ||
		envVarIsTrue("DISABLE_TELEMETRY") {
		return false
	}

	return a.config.Analytics
}

func envVarIsTrue(envVar string) bool {
	b, _ := strconv.ParseBool(os.Getenv(envVar))
	return b
}
