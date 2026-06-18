package common

import (
	"errors"
	"testing"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

func TestCheckServiceReady(t *testing.T) {
	tests := []struct {
		name    string
		status  *api.DeployStatus
		wantErr error
	}{
		{name: "ready", status: util.Ptr(api.READY), wantErr: nil},
		{name: "paused", status: util.Ptr(api.PAUSED), wantErr: ErrPaused},
		{name: "pausing", status: util.Ptr(api.PAUSING), wantErr: ErrPaused},
		{name: "queued", status: util.Ptr(api.QUEUED), wantErr: ErrNotReady},
		{name: "configuring", status: util.Ptr(api.CONFIGURING), wantErr: ErrNotReady},
		{name: "resuming", status: util.Ptr(api.RESUMING), wantErr: ErrNotReady},
		{name: "upgrading", status: util.Ptr(api.UPGRADING), wantErr: ErrNotReady},
		{name: "optimizing", status: util.Ptr(api.OPTIMIZING), wantErr: ErrNotReady},
		{name: "unstable", status: util.Ptr(api.UNSTABLE), wantErr: ErrNotReady},
		{name: "deleting", status: util.Ptr(api.DELETING), wantErr: ErrNotReady},
		{name: "deleted", status: util.Ptr(api.DELETED), wantErr: ErrNotReady},
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
