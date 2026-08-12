package common

import (
	"errors"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/util"
)

func TestCheckServiceReady(t *testing.T) {
	tests := []struct {
		name    string
		status  *api.DeployStatus
		wantErr error
	}{
		{name: "ready", status: util.Ptr(api.DeployStatusREADY), wantErr: nil},
		{name: "paused", status: util.Ptr(api.DeployStatusPAUSED), wantErr: ErrPaused},
		{name: "pausing", status: util.Ptr(api.DeployStatusPAUSING), wantErr: ErrPaused},
		{name: "queued", status: util.Ptr(api.DeployStatusQUEUED), wantErr: ErrNotReady},
		{name: "configuring", status: util.Ptr(api.DeployStatusCONFIGURING), wantErr: ErrNotReady},
		{name: "resuming", status: util.Ptr(api.DeployStatusRESUMING), wantErr: ErrNotReady},
		{name: "upgrading", status: util.Ptr(api.DeployStatusUPGRADING), wantErr: ErrNotReady},
		{name: "optimizing", status: util.Ptr(api.DeployStatusOPTIMIZING), wantErr: ErrNotReady},
		{name: "unstable", status: util.Ptr(api.DeployStatusUNSTABLE), wantErr: ErrNotReady},
		{name: "deleting", status: util.Ptr(api.DeployStatusDELETING), wantErr: ErrNotReady},
		{name: "deleted", status: util.Ptr(api.DeployStatusDELETED), wantErr: ErrNotReady},
		{name: "unknown status", status: util.Ptr(api.DeployStatus("SOMETHING_NEW")), wantErr: ErrNotReady},
		{name: "nil status", status: nil, wantErr: ErrNotReady},
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
