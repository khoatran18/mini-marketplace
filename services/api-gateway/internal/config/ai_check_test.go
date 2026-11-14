package config

import "testing"

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
