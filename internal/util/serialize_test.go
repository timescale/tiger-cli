package util

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSerializeToYAML(t *testing.T) {
	type payload struct {
		Value   any    `json:"value"`
		Omitted string `json:"omitted,omitempty"`
		Hidden  string `json:"-"`
	}
	for _, tt := range []struct {
		name  string
		value any
		want  string
	}{
		{"large signed integer", int64(9007199254740993), "value: 9007199254740993\n"},
		{"maximum signed integer", int64(9223372036854775807), "value: 9223372036854775807\n"},
		{"maximum unsigned integer", uint64(18446744073709551615), "value: 18446744073709551615\n"},
		{"negative signed integer", int64(-9007199254740993), "value: -9007199254740993\n"},
		{"nested integer", []int64{9007199254740993}, "value:\n  - 9007199254740993\n"},
		{"numeric string", "9007199254740993", "value: \"9007199254740993\"\n"},
		{"fraction", 1.25, "value: 1.25\n"},
		{"boolean", true, "value: true\n"},
		{"null", nil, "value: null\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			err := SerializeToYAML(&output, payload{Value: tt.value, Hidden: "hidden"})
			require.NoError(t, err)
			require.Equal(t, tt.want, output.String())
		})
	}
}
