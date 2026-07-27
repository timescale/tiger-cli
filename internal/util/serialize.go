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

// SerializeToYAML serializes data to YAML format.
//
// This function first marshals to JSON, then unmarshals and encodes to YAML.
// This approach ensures that:
//  1. Structs from third-party libraries or generated code that only include
//     json: struct tags (without yaml: tags) are correctly marshaled
//  2. JSON and YAML marshaling produce consistent output, both respecting
//     the same json: tags and omitempty directives
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
