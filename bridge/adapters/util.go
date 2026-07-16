// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Shared adapter helpers — JSON extraction from noisy CLI output and
//
//	temp-file handling for JSON schemas.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package adapters

import (
	"encoding/json"
	"strings"
)

// parseJSON best-effort extracts a JSON object from model output that may carry
// code fences or surrounding prose (common with web-grounded answers).
func parseJSON(text string) (any, bool) {
	s := stripFences(strings.TrimSpace(text))

	var obj any
	if err := json.Unmarshal([]byte(s), &obj); err == nil {
		return obj, true
	}

	// Fall back to the first balanced {...} span.
	start := strings.IndexByte(s, '{')
	if start == -1 {
		return nil, false
	}
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				var o any
				if err := json.Unmarshal([]byte(s[start:i+1]), &o); err == nil {
					return o, true
				}
				return nil, false
			}
		}
	}
	return nil, false
}

// stripFences removes a leading ```json / ``` fence pair if present.
func stripFences(text string) string {
	if !strings.Contains(text, "```") {
		return text
	}
	parts := strings.SplitN(text, "```", 3)
	if len(parts) < 2 {
		return text
	}
	body := parts[1]
	body = strings.TrimPrefix(body, "json")
	return strings.TrimSpace(body)
}
