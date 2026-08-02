package biz

import (
	"testing"

	"github.com/rulego/rulego/api/types"
	"gorm.io/datatypes"
)

func TestFingerprintStable(t *testing.T) {
	s1 := datatypes.JSON([]byte(`{"a":1}`))
	f1 := fingerprint("jsFilter", s1, "desc")
	f2 := fingerprint("jsFilter", s1, "desc")
	if f1 != f2 {
		t.Fatalf("fingerprint not stable: %s vs %s", f1, f2)
	}
	f3 := fingerprint("jsFilter", s1, "desc-changed")
	if f1 == f3 {
		t.Fatalf("fingerprint should change when description changes")
	}
}

func TestFormToMeta(t *testing.T) {
	form := &types.ComponentForm{
		Type:     "jsFilter",
		Category: "filter",
		Label:    "JS 过滤",
		Desc:     "用 JS 过滤消息",
		Fields: types.ComponentFormFieldList{
			{Name: "jsScript", Type: "string", Label: "脚本", DefaultValue: "return true;", Required: true},
		},
	}
	m := formToMeta(form)
	if m.Type != "jsFilter" || m.Category != "filter" || m.Name != "JS 过滤" {
		t.Fatalf("unexpected meta: %+v", m)
	}
	if len(m.ConfigSchema) == 0 {
		t.Fatalf("configSchema should be serialized")
	}
	if len(m.Example) == 0 || string(m.Example) == "null" {
		t.Fatalf("example should include default values")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "x", "y"); got != "x" {
		t.Fatalf("firstNonEmpty=%q want x", got)
	}
	if got := firstNonEmpty("", "", ""); got != "" {
		t.Fatalf("firstNonEmpty empty=%q", got)
	}
}
