package cmd

import "strings"

// filterCompletionsByPrefix filters a slice of strings to only include items
// that start with the given prefix. This is used by shell completion functions
// to narrow down suggestions based on what the user has typed so far.
func filterCompletionsByPrefix(items []string, prefix string) []string {
	var filtered []string
	for _, item := range items {
		if strings.HasPrefix(item, prefix) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
