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

func SerializeToYAML(w io.Writer, v any, omitNull bool) error {
	encoder := yaml.NewEncoder(w)
	defer encoder.Close()
	encoder.SetIndent(2)

	if omitNull {
		if toOutput, err := toJSON(v); err != nil {
			return err
		} else {
			return encoder.Encode(toOutput)
		}
	}

	return encoder.Encode(v)
}
