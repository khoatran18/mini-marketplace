package playbooks

import (
	"encoding/json"
	"strings"
)

func jsonOf(v any) (string, error) { b, err := json.Marshal(v); return string(b), err }
func contains(s, sub string) bool  { return strings.Contains(s, sub) }
