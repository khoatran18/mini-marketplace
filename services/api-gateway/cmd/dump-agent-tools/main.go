// Command dump-agent-tools prints the agent tool table (per role: reads and proposals, with params) as JSON.
// The AI evaluation set (services/ai-agent/evals) is checked against this output, so a renamed tool or param fails the check.
package main

import (
	"encoding/json"
	"os"

	"api-gateway/internal/agent"
)

func main() {
	out := map[string]any{"scopes": agent.DefaultScopes, "actions": agent.DefaultActions}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", " ")
	if err := enc.Encode(out); err != nil {
		os.Exit(1)
	}
}
