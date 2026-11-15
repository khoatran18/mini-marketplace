package config

import (
	"strings"
	"testing"
)

func TestCheckAIConfigStopsAnAIFeatureThatCannotWork(t *testing.T) {
	cases := []struct {
		on        bool
		url, keys string
		wantErr   bool
	}{
		{false, "", "", false}, // feature off: nothing required
		{true, "http://ai-agent:8090", "k1:s", false},
		{true, "", "k1:s", true},                 // no upstream
		{true, "http://ai-agent:8090", "", true}, // no agent keys
	}
	for _, c := range cases {
		if err := checkAIConfig(c.on, c.url, c.keys); (err != nil) != c.wantErr {
			t.Errorf("checkAIConfig(%v,%q,%q) = %v, wantErr %v", c.on, c.url, c.keys, err, c.wantErr)
		}
	}
}

func TestCheckSystemKey(t *testing.T) {
	long := strings.Repeat("k", 32)
	for _, c := range []struct {
		key, keys string
		wantErr   bool
	}{{"", "", false}, {"", "k1:x", false}, {long, "k1:s", false}, {long, "", true}, {"short", "k1:s", true}} {
		if err := checkSystemKey(c.key, c.keys); (err != nil) != c.wantErr {
			t.Errorf("checkSystemKey(%q,%q) = %v, wantErr %v", c.key, c.keys, err, c.wantErr)
		}
	}
}
