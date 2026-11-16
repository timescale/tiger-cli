package util

import "testing"

func TestFormatStorageMB(t *testing.T) {
	tests := []struct {
		mb   int
		want string
	}{
		{0, "0 MB"},
		{512, "512 MB"},
		{1023, "1023 MB"},
		{1024, "1 GB"},
		{1229, "1.2 GB"},
		{863744, "843.5 GB"},
		{1 << 20, "1 TB"},
		{1 << 40, "1024 PB"},
	}
	for _, tt := range tests {
		if got := FormatStorageMB(tt.mb); got != tt.want {
			t.Errorf("FormatStorageMB(%d) = %q, want %q", tt.mb, got, tt.want)
		}
	}
}
