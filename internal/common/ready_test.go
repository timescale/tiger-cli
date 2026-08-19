package common

import (
	"errors"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
)

func TestCheckServiceReady(t *testing.T) {
	tests := []struct {
		name    string
		status  api.DeployStatus
		wantErr error
	}{
		{name: "ready", status: api.DeployStatusREADY, wantErr: nil},
		{name: "paused", status: api.DeployStatusPAUSED, wantErr: ErrPaused},
		{name: "pausing", status: api.DeployStatusPAUSING, wantErr: ErrPaused},
		{name: "queued", status: api.DeployStatusQUEUED, wantErr: ErrNotReady},
		{name: "configuring", status: api.DeployStatusCONFIGURING, wantErr: ErrNotReady},
		{name: "resuming", status: api.DeployStatusRESUMING, wantErr: ErrNotReady},
		{name: "upgrading", status: api.DeployStatusUPGRADING, wantErr: ErrNotReady},
		{name: "optimizing", status: api.DeployStatusOPTIMIZING, wantErr: ErrNotReady},
		{name: "unstable", status: api.DeployStatusUNSTABLE, wantErr: ErrNotReady},
		{name: "deleting", status: api.DeployStatusDELETING, wantErr: ErrNotReady},
		{name: "deleted", status: api.DeployStatusDELETED, wantErr: ErrNotReady},
		{name: "unknown status", status: api.DeployStatus("SOMETHING_NEW"), wantErr: ErrNotReady},
		{name: "empty status", status: "", wantErr: ErrNotReady},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckServiceReady(api.Service{Status: tt.status})
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("CheckServiceReady() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
