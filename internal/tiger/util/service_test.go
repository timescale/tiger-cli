package util

import (
	"reflect"
	"strings"
	"testing"
)

func TestValidAddons(t *testing.T) {
	expected := []string{"time-series", "ai"}
	result := ValidAddons()

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("ValidAddons() = %v, want %v", result, expected)
	}
}

func TestIsValidAddon(t *testing.T) {
	tests := []struct {
		name  string
		addon string
		want  bool
	}{
		{"Valid time-series", "time-series", true},
		{"Valid ai", "ai", true},
		{"Invalid addon", "invalid", false},
		{"Empty string", "", false},
		{"Case sensitive - AI uppercase", "AI", false},
		{"Case sensitive - Time-Series mixed", "Time-Series", false},
		{"Similar but wrong", "timeseries", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidAddon(tt.addon); got != tt.want {
				t.Errorf("IsValidAddon(%q) = %v, want %v", tt.addon, got, tt.want)
			}
		})
	}
}

func TestAddonConstants(t *testing.T) {
	// Verify constants have expected values
	if AddonTimeSeries != "time-series" {
		t.Errorf("AddonTimeSeries = %q, want %q", AddonTimeSeries, "time-series")
	}
	if AddonAI != "ai" {
		t.Errorf("AddonAI = %q, want %q", AddonAI, "ai")
	}
	if AddonNone != "none" {
		t.Errorf("AddonNone = %q, want %q", AddonNone, "none")
	}
}

func TestValidateAddons(t *testing.T) {
	tests := []struct {
		name       string
		addons     []string
		want       []string
		wantErr    bool
		errMessage string
	}{
		{"Empty slice", []string{}, nil, false, ""},
		{"Nil slice", nil, nil, false, ""},
		{"Single addon - time-series", []string{"time-series"}, []string{"time-series"}, false, ""},
		{"Single addon - ai", []string{"ai"}, []string{"ai"}, false, ""},
		{"Multiple addons", []string{"time-series", "ai"}, []string{"time-series", "ai"}, false, ""},
		{"Multiple addons reversed", []string{"ai", "time-series"}, []string{"ai", "time-series"}, false, ""},
		{"Duplicates removed", []string{"time-series", "time-series", "ai"}, []string{"time-series", "ai"}, false, ""},
		{"None as single element", []string{"none"}, []string{}, false, ""},
		{"None uppercase", []string{"NONE"}, []string{}, false, ""},
		{"None mixed case", []string{"None"}, []string{}, false, ""},
		{"Invalid addon", []string{"invalid"}, nil, true, "invalid add-on 'invalid'"},
		{"Mix valid and invalid", []string{"time-series", "invalid"}, nil, true, "invalid add-on 'invalid'"},
		{"Whitespace trimmed", []string{" time-series ", " ai "}, []string{"time-series", "ai"}, false, ""},
		{"Case sensitive - AI uppercase fails", []string{"AI"}, nil, true, "invalid add-on 'AI'"},
		{"Case sensitive - Time-Series mixed case fails", []string{"Time-Series"}, nil, true, "invalid add-on 'Time-Series'"},
		{"Empty string is invalid", []string{""}, nil, true, "invalid add-on ''"},
		{"Mix of valid, invalid, and empty", []string{"time-series", "", "invalid"}, nil, true, "invalid add-on ''"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateAddons(tt.addons)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateAddons(%v) expected error but got none", tt.addons)
					return
				}
				if tt.errMessage != "" && !strings.Contains(err.Error(), tt.errMessage) {
					t.Errorf("ValidateAddons(%v) error = %q, want error containing %q", tt.addons, err.Error(), tt.errMessage)
				}
				return
			}

			if err != nil {
				t.Errorf("ValidateAddons(%v) unexpected error = %v", tt.addons, err)
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ValidateAddons(%v) = %v, want %v", tt.addons, got, tt.want)
			}
		})
	}
}
