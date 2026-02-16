package log

import (
	"github.com/goccy/go-yaml"
)

// PrettyYaml marshals the given data into a YAML string with an indent of 2 spaces.
// It uses github.com/goccy/go-yaml for marshaling.
func PrettyYaml(data any) ([]byte, error) {
	return yaml.MarshalWithOptions(data, yaml.Indent(2))
}

// PrettyYamlString is a convenience wrapper around PrettyYaml that returns
// the YAML as a string. If an error occurs during marshaling, it returns an empty string.
func PrettyYamlString(data any) string {
	prettyData, err := PrettyYaml(data)
	if err != nil {
		return ""
	}
	return string(prettyData)
}
