package auth

import (
	"strings"
	"testing"
)

func TestProviderJSONBoundary(t *testing.T) {
	for _, input := range []string{`{"name":"first","name":"second"}`, `{"name":"ok"} {}`, strings.Repeat(" ", 2049), strings.Repeat("[", 40) + strings.Repeat("]", 40)} {
		var target struct {
			Name string `json:"name"`
		}
		if decodeProviderJSON(strings.NewReader(input), 2048, &target) == nil {
			t.Fatal("unsafe JSON accepted")
		}
	}
}

func FuzzBoundedJSON(f *testing.F) {
	f.Add(`{"name":"fixture"}`)
	f.Add(`{"name":"x","name":"y"}`)
	f.Fuzz(func(t *testing.T, input string) {
		var target struct {
			Name string `json:"name"`
		}
		decodeProviderJSON(strings.NewReader(input), 4096, &target)
	})
}
