package biz

import (
	"encoding/json"
	"testing"
)

// hasSubstance 决定 MCP 暴露/SKILL 生成是否已有"实质入参 schema"。
func TestHasSubstance(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"空串", "", false},
		{"空白", "   ", false},
		{"空对象", "{}", false},
		{"带空格的空对象", " { } ", false},
		{"null", "null", false},
		{"非法 JSON", "{not json", false},
		{"实质 schema", `{"type":"object","properties":{"t":{"type":"number"}},"required":["t"]}`, true},
		{"仅 type", `{"type":"object"}`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := hasSubstance(json.RawMessage(c.in))
			if got != c.want {
				t.Fatalf("hasSubstance(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
