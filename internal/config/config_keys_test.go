package config

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// mapstructureKeys returns the mapstructure tag of every field of a struct
// type, skipping the "-" fields that aren't config keys.
func mapstructureKeys(t *testing.T, v any) []string {
	t.Helper()
	typ := reflect.TypeOf(v)
	keys := make([]string, 0, typ.NumField())
	for field := range typ.Fields() {
		tag := field.Tag.Get("mapstructure")
		if tag == "" {
			t.Errorf("%s.%s has no mapstructure tag", typ.Name(), field.Name)
			continue
		}
		if tag == "-" {
			continue
		}
		keys = append(keys, tag)
	}
	slices.Sort(keys)
	return keys
}

// A config key has to be spelled out in several places that Go can't tie
// together: defaultValues, the Config struct, the ConfigOutput struct that
// `tiger config show` renders, and validateValue's switch. Missing one is
// silent — a key absent from ConfigOutput just never appears in `config show`,
// and one absent from validateValue can't be set at all — so assert the lists
// agree rather than relying on whoever adds the next key to find all four.
func TestConfigKeyRegistriesAgree(t *testing.T) {
	want := ValidConfigOptions()
	slices.Sort(want)

	t.Run("Config struct", func(t *testing.T) {
		assertSameKeys(t, want, mapstructureKeys(t, Config{}))
	})

	t.Run("ConfigOutput struct", func(t *testing.T) {
		assertSameKeys(t, want, mapstructureKeys(t, ConfigOutput{}))
	})

	// A bogus value proves only that the key reaches a case: whether it then
	// passes validation is the individual key's business.
	t.Run("validateValue accepts every key", func(t *testing.T) {
		for _, key := range want {
			if _, err := validateValue(key, "!not-a-valid-value!"); err != nil &&
				strings.Contains(err.Error(), "unknown configuration key") {
				t.Errorf("validateValue has no case for %q", key)
			}
		}
		if _, err := validateValue("no_such_key", "x"); err == nil {
			t.Error("validateValue accepted an unknown key")
		}
	})

	// Every flag that claims to override a config value must name a real one.
	t.Run("flagBindings target real keys", func(t *testing.T) {
		for flag, key := range flagBindings {
			if !slices.Contains(want, key) {
				t.Errorf("flag --%s binds to unknown config key %q", flag, key)
			}
		}
	})
}

func assertSameKeys(t *testing.T, want, got []string) {
	t.Helper()
	for _, key := range want {
		if !slices.Contains(got, key) {
			t.Errorf("missing config key %q", key)
		}
	}
	for _, key := range got {
		if !slices.Contains(want, key) {
			t.Errorf("unexpected config key %q (not in defaultValues)", key)
		}
	}
}
