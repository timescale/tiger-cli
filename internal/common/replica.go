package common

import (
	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/util"
)

// IsReadReplica reports whether the service is a standby read replica (which
// shares its parent primary's credentials).
func IsReadReplica(service api.Service) bool {
	return service.ForkedFrom != nil && util.Deref(service.ForkedFrom.IsStandby)
}
