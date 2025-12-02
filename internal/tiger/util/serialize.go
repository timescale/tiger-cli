package util

import (
	"encoding/json"
	"io"

	"gopkg.in/yaml.v3"
)

// Convert to JSON to respect omitempty tags, then unmarshal
func toJSON(v any) (any, error) {
	jsonBytes, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	var jsonOut any
	if err := json.Unmarshal(jsonBytes, &jsonOut); err != nil {
		return nil, err
	}
	return jsonOut, nil
}

func SerializeToJSON(w io.Writer, v any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

// SerializeToYAML serializes a value to YAML format.
//
// This function marshals to JSON first, then encodes to YAML. This ensures that
// structs from third-party libraries and generated code that only have `json:` tags
// (and no `yaml:` tags) are correctly serialized. It also ensures consistent behavior
// between JSON and YAML output by respecting `omitempty` tags.
func SerializeToYAML(w io.Writer, v any) error {
	encoder := yaml.NewEncoder(w)
	defer encoder.Close()
	encoder.SetIndent(2)

	toOutput, err := toJSON(v)
	if err != nil {
		return err
	}
	return encoder.Encode(toOutput)
}
