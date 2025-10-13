package util

import (
	"testing"
)

// Define local types for testing to avoid import cycles
type testStringType string

func TestDeref(t *testing.T) {
	// Test service ID formatting (now uses string instead of UUID)
	testServiceID := "12345678-9abc-def0-1234-56789abcdef0"
	if Deref(&testServiceID) != testServiceID {
		t.Error("Deref should return service ID string")
	}
	if Deref((*string)(nil)) != "" {
		t.Error("Deref should return empty string for nil")
	}

	// Test Deref with string
	testStr := "test"
	if Deref(&testStr) != "test" {
		t.Error("Deref should return string value")
	}
	if Deref((*string)(nil)) != "" {
		t.Error("Deref should return empty string for nil")
	}
}

func TestDerefStr(t *testing.T) {
	status := testStringType("running")
	if DerefStr(&status) != "running" {
		t.Error("DerefStr should return status string")
	}
	if DerefStr((*testStringType)(nil)) != "" {
		t.Error("DerefStr should return empty string for nil")
	}
}

func TestConvertStringSlice(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		wantNil bool
	}{
		{
			name:    "Empty",
			input:   []string{},
			wantNil: true,
		},
		{
			name:    "Nil",
			input:   nil,
			wantNil: true,
		},
		{
			name:    "Single",
			input:   []string{"time-series"},
			wantNil: false,
		},
		{
			name:    "Multiple",
			input:   []string{"time-series", "ai"},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertStringSlicePtr[testStringType](tt.input)

			if tt.wantNil {
				if got != nil {
					t.Errorf("ConvertStringSlice(%v) = %v, want nil", tt.input, got)
				}
				return
			}

			if got == nil {
				t.Errorf("ConvertStringSlice(%v) = nil, want non-nil", tt.input)
				return
			}

			if len(got) != len(tt.input) {
				t.Errorf("ConvertStringSlice(%v) length = %d, want %d", tt.input, len(got), len(tt.input))
			}

			// Verify the conversion is correct
			for i, s := range tt.input {
				if string(got[i]) != s {
					t.Errorf("ConvertStringSlice(%v)[%d] = %s, want %s", tt.input, i, got[i], s)
				}
			}
		})
	}
}
