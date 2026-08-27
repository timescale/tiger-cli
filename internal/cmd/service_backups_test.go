package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/timescale/tiger-cli/internal/api"
)

func createTestBackups() []api.Backup {
	started := time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC)
	finished := time.Date(2026, 1, 15, 9, 41, 12, 0, time.UTC)

	return []api.Backup{
		{
			Label:           "20260115-093000F",
			Type:            api.BackupTypeFULL,
			StartedAt:       started,
			FinishedAt:      &finished,
			DurationSeconds: new(int64(672)),
			SizeBytes:       new(int64(4831838208)),
			Regions: []api.BackupRegionState{
				{RegionCode: "us-east-1", Status: new(api.BackupCopyStatusFINISHED)},
				{RegionCode: "eu-central-1", Status: new(api.BackupCopyStatusFAILED)},
			},
		},
		{
			// Still running: no finish time, duration or size.
			// No per-region status: the backend does not always report one.
			Label:     "20260115-093000F_20260116-100000I",
			Type:      api.BackupTypeINCREMENTAL,
			StartedAt: started.Add(24 * time.Hour),
			Regions:   []api.BackupRegionState{{RegionCode: "us-east-1"}},
		},
	}
}

func TestOutputBackups_JSON(t *testing.T) {
	setupServiceTest(t)

	cmd := &cobra.Command{}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	if err := outputBackups(cmd, createTestBackups(), "json"); err != nil {
		t.Fatalf("Failed to output JSON: %v", err)
	}

	var result []api.Backup
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("Invalid JSON output: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("Expected 2 backups in JSON, got %d", len(result))
	}
	// The label is dropped from the table but kept here.
	if result[0].Label != "20260115-093000F" {
		t.Errorf("Expected label in JSON, got %q", result[0].Label)
	}
	if result[1].FinishedAt != nil {
		t.Errorf("Expected no finished_at for a running backup, got %v", result[1].FinishedAt)
	}
}

func TestOutputBackups_YAML(t *testing.T) {
	setupServiceTest(t)

	cmd := &cobra.Command{}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	if err := outputBackups(cmd, createTestBackups(), "yaml"); err != nil {
		t.Fatalf("Failed to output YAML: %v", err)
	}

	var result []api.Backup
	if err := yaml.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("Invalid YAML output: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("Expected 2 backups in YAML, got %d", len(result))
	}
}

func TestOutputBackups_Table(t *testing.T) {
	setupServiceTest(t)

	cmd := &cobra.Command{}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	if err := outputBackups(cmd, createTestBackups(), "table"); err != nil {
		t.Fatalf("Failed to output table: %v", err)
	}
	output := buf.String()

	started := time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC)
	for _, want := range []string{
		"STARTED", "TYPE", "DURATION", "SIZE", "REGIONS",
		started.Local().Format("2006-01-02 15:04 MST"), "FULL", "11m12s", "4.5GiB",
		"us-east-1 (FINISHED), eu-central-1 (FAILED)",
		started.Add(24 * time.Hour).Local().Format("2006-01-02 15:04 MST"), "INCREMENTAL",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("Expected table to contain %q, got:\n%s", want, output)
		}
	}

	for _, unwanted := range []string{"LABEL", "20260115-093000F"} {
		if strings.Contains(output, unwanted) {
			t.Errorf("Expected table not to contain %q, got:\n%s", unwanted, output)
		}
	}
}

func TestOutputBackups_EnvUnsupported(t *testing.T) {
	setupServiceTest(t)

	cmd := &cobra.Command{}
	cmd.SetOut(new(bytes.Buffer))

	if err := outputBackups(cmd, createTestBackups(), "env"); err == nil {
		t.Fatal("Expected an error for env output")
	}
}

func TestFormatSizeBytes(t *testing.T) {
	cases := []struct {
		name  string
		bytes *int64
		want  string
	}{
		{name: "absent", bytes: nil, want: ""},
		{name: "bytes", bytes: new(int64(512)), want: "512B"},
		{name: "kibibytes", bytes: new(int64(2048)), want: "2KiB"},
		{name: "mebibytes", bytes: new(int64(53_300_000)), want: "50.83MiB"},
		{name: "gibibytes", bytes: new(int64(4831838208)), want: "4.5GiB"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatSizeBytes(tc.bytes); got != tc.want {
				t.Errorf("formatSizeBytes() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatDurationSeconds(t *testing.T) {
	cases := []struct {
		name    string
		seconds *int64
		want    string
	}{
		{name: "absent", seconds: nil, want: ""},
		{name: "seconds", seconds: new(int64(2)), want: "2s"},
		{name: "minutes", seconds: new(int64(672)), want: "11m12s"},
		{name: "hours", seconds: new(int64(7325)), want: "2h2m5s"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatDurationSeconds(tc.seconds); got != tc.want {
				t.Errorf("formatDurationSeconds() = %q, want %q", got, tc.want)
			}
		})
	}
}
