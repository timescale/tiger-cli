package util

import (
	"encoding"
	"time"
)

var (
	_ encoding.TextMarshaler   = Duration{}
	_ encoding.TextUnmarshaler = (*Duration)(nil)
)

// Duration is a wrapper around time.Duration that allows it to be
// marshalled to/from JSON via the standard duration string format.
type Duration struct {
	time.Duration
}

// Implements the [encoding.TextUnmarshaler] interface.
func (d *Duration) UnmarshalText(b []byte) error {
	duration, err := time.ParseDuration(string(b))
	if err != nil {
		return err
	}
	*d = Duration{
		Duration: duration,
	}
	return nil
}

// Implements the [encoding.TextMarshaler] interface.
func (d Duration) MarshalText() ([]byte, error) {
	str := d.String()
	return []byte(str), nil
}
