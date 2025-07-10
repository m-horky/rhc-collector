package insights

import (
	"testing"
)

func TestParseCollector(t *testing.T) {
	tests := []struct {
		blob string
		name string
		id   string
	}{
		{
			blob: "[meta]\nid=\"org.example.greeting\"\nname=\"Greeting\"",
			name: "Greeting",
			id:   "org.example.greeting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			actual, err := newCollectorFromConfiguration("/tmp/mock.toml", tt.blob)
			if err != nil {
				t.Errorf("'%s': got '%v'", tt.id, err)
			}
			if tt.id != actual.Meta.ID {
				t.Errorf("'%s': wanted '%s', got '%s'", tt.id, tt.id, actual.Meta.ID)
			}
			if tt.name != actual.Meta.Name {
				t.Errorf("'%s': wanted '%s', got '%s'", tt.id, tt.name, actual.Meta.Name)
			}
		})
	}
}
