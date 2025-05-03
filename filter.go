package blazesub

import (
	"regexp"
	"strings"
)

type topicFilter struct {
	// Single comprehensive pattern for validating filters
	// This pattern validates:
	// 1. No leading or trailing slashes
	// 2. No empty segments (double slashes)
	// 3. Only allowed characters in segments
	// 4. Proper + usage (standalone in a segment)
	// 5. Proper # usage (only at the end as standalone segment)
	validFilter *regexp.Regexp

	// Secondary pattern to ensure # and + aren't mixed in the same filter
	mixedWildcards *regexp.Regexp

	// Pattern for validating topics (no wildcards allowed)
	validTopic *regexp.Regexp
}

func NewTopicFilter() TopicFilter {
	// Key components of the regex:
	// ^                         - Start of string
	// (?:                       - Non-capturing group for a segment
	//   (?:[a-z0-9_-]+)         - Valid character segment (alphanumeric, underscore, dash)
	//   |                        - OR
	//   (?:\+)                   - A standalone + wildcard
	//   |                        - OR
	//   (?:#)                    - A standalone # wildcard
	// )                         - End of non-capturing group
	// (?:                       - Non-capturing group for additional segments
	//   /                        - Segment separator (slash)
	//   (?:                      - Non-capturing group for a segment
	//     (?:[a-z0-9_-]+)        - Valid character segment
	//     |                       - OR
	//     (?:\+)                  - A standalone + wildcard
	//     |                       - OR
	//     (?:#)                   - A standalone # wildcard (only valid at the end)
	//   )                        - End of non-capturing group
	// )*                        - Zero or more additional segments
	// $                         - End of string
	//
	// Note: This regex doesn't fully enforce # only at the end - we'll check that separately
	// Main filter validation regex for general pattern
	validFilter := regexp.MustCompile(`^(?:[a-z0-9_-]+|\+|#)(?:/(?:[a-z0-9_-]+|\+|#))*$`)

	// Check for mixed wildcards (+ and # in same filter)
	mixedWildcards := regexp.MustCompile(`.*\+.*#|.*#.*\+`)

	// Topic validation regex (only allows alphanumeric, underscore, dash)
	// No wildcards, no leading/trailing slashes, no empty segments
	validTopic := regexp.MustCompile(`^[a-z0-9_-]+(?:/[a-z0-9_-]+)*$`)

	return &topicFilter{
		validFilter:    validFilter,
		mixedWildcards: mixedWildcards,
		validTopic:     validTopic,
	}
}

func (f *topicFilter) Validate(filter string) error {
	// Special cases for single wildcards
	if filter == "+" || filter == "#" {
		return nil
	}

	// Convert to lowercase for validation
	lowercaseFilter := strings.ToLower(filter)

	// Check if filter matches the general pattern
	if !f.validFilter.MatchString(lowercaseFilter) {
		return ErrInvalidFilter
	}

	// Check # is only used at the end
	if strings.Contains(lowercaseFilter, "#") && !strings.HasSuffix(lowercaseFilter, "/#") && lowercaseFilter != "#" {
		return ErrInvalidFilter
	}

	// Check for mixed wildcards
	if f.mixedWildcards.MatchString(lowercaseFilter) {
		return ErrInvalidFilter
	}

	return nil
}

// ValidateTopic validates a topic string (no wildcards allowed).
func (f *topicFilter) ValidateTopic(topic string) error {
	// Convert to lowercase for validation
	lowercaseTopic := strings.ToLower(topic)

	// Check if topic matches the valid pattern
	if !f.validTopic.MatchString(lowercaseTopic) {
		return ErrInvalidTopic
	}

	return nil
}

type TopicFilter interface {
	Validate(filter string) error
	ValidateTopic(topic string) error
}
