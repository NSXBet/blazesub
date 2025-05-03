package blazesub

import "testing"

func TestValidateTopicFilter(t *testing.T) {
	tests := []struct {
		name    string
		filter  string
		wantErr bool
	}{
		// Basic valid cases
		{"valid simple topic", "topic", false},
		{"valid multi-level topic", "user/created", false},
		{"valid + wildcard", "matches/+/scores", false},
		{"valid + wildcard at end", "matches/123/+", false},
		{"valid # wildcard at end", "matches/#", false},
		{"valid multi-level with # at end", "matches/123/scores/#", false},

		// Invalid + usage
		{"invalid + combined with text", "matches/+scores", true},
		{"invalid text combined with +", "matches/scores+", true},

		// Invalid # usage
		{"invalid # not at end", "matches/#/scores", true},
		{"invalid # before word", "matches/#scores", true},
		{"invalid # after word", "matches/scores#", true},
		{"invalid # in the beginning before word", "#matches/scores", true},
		{"invalid # in the beginning after word", "matches#/scores", true},
		{"invalid # before word multi-level", "matches/#scores/test", true},
		{"invalid # after word multi-level", "matches/scores#/test", true},
		{"invalid # not after slash", "matches#", true},

		// Empty filter case
		{"empty filter", "", true}, // Decide if you want to allow or reject empty filter

		// Single character filters
		{"single + character", "+", false},
		{"single # character", "#", false},

		// Root-level with leading slash
		{"root-level topic with leading slash", "/topic", true},
		{"root-level + with leading slash", "/+", true},
		{"root-level # with leading slash", "/#", true},

		// Multiple consecutive wildcards
		{"multiple consecutive + wildcards", "topic/+/+/+", false},
		{"mix of + and # wildcards", "+/+/#", true},
		{"consecutive + followed by topic", "+/+/subtopic", false},

		// Edge cases with slashes
		{"trailing slash", "topic/", true},
		{"leading and trailing slash", "/topic/", true},
		{"double slash (empty segment)", "topic//subtopic", true},

		// Special cases with numbers and symbols
		{"numbers in path", "topic/123/+", false},
		{"dashes in topic", "topic-with-dashes/+", false},
		{"dots in topic", "topic.with.dots/+", true},
		{"underscores in topic", "topic_with_underscores/+", false},

		// Case sensitivity checks
		{"mixed case", "Topic/+", false},
		{"all uppercase", "UPPERCASE/+", false},

		// More complex combinations
		{"complex path with multiple segments", "users/profile/settings/notifications/#", false},
		{"multiple segments with + wildcards", "devices/+/sensors/+/readings", false},
		{"mixed + and regular segments", "region/+/country/germany/city/+", false},
	}

	topicFilter := NewTopicFilter()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := topicFilter.Validate(tt.filter)
			if (err != nil) != tt.wantErr {
				t.Errorf("TopicFilter.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateTopic(t *testing.T) {
	tests := []struct {
		name    string
		topic   string
		wantErr bool
	}{
		// Valid topics
		{"valid simple topic", "topic", false},
		{"valid multi-level topic", "user/created", false},
		{"valid with numbers", "user123/profile456", false},
		{"valid with underscore", "user_profile/settings", false},
		{"valid with hyphen", "user-profile/settings", false},
		{"valid complex path", "region/north-america/us/california", false},
		{"valid mixed case (lowercase enforced)", "User/Profile", false},

		// Invalid topics
		{"empty topic", "", true},
		{"leading slash", "/topic", true},
		{"trailing slash", "topic/", true},
		{"empty segment", "topic//subtopic", true},
		{"contains wildcard +", "topic/+/subtopic", true},
		{"contains wildcard #", "topic/#", true},
		{"contains dot", "topic.name/subtopic", true},
		{"contains special character", "topic@home/subtopic", true},
		{"contains space", "topic name/subtopic", true},
		{"contains parentheses", "topic(name)/subtopic", true},
	}

	topicFilter := NewTopicFilter()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := topicFilter.ValidateTopic(tt.topic)
			if (err != nil) != tt.wantErr {
				t.Errorf("TopicFilter.ValidateTopic() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
