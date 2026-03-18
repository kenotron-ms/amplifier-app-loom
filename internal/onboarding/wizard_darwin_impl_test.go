//go:build darwin && cgo

package onboarding

import (
	"strings"
	"testing"
)

func TestBuildHTML_EscapesSpecialCharsInKeys(t *testing.T) {
	s := &state{
		anthropicKey: `sk-ant-"><script>alert(1)</script>`,
		openAIKey:    `sk-' onmouseover='x`,
	}
	got := buildHTML(s)
	if strings.Contains(got, `"><script>`) {
		t.Error("anthropicKey was not HTML-escaped: contains unescaped >\"")
	}
	if strings.Contains(got, `' onmouseover=`) {
		t.Error("openAIKey was not HTML-escaped: contains unescaped '")
	}
	if strings.Contains(got, "{{ANTHROPIC_KEY}}") {
		t.Error("{{ANTHROPIC_KEY}} placeholder was not substituted")
	}
	if strings.Contains(got, "{{OPENAI_KEY}}") {
		t.Error("{{OPENAI_KEY}} placeholder was not substituted")
	}
	if strings.Contains(got, "{{FDA_GRANTED}}") {
		t.Error("{{FDA_GRANTED}} placeholder was not substituted")
	}
	if strings.Contains(got, "{{FDA_GUIDE_DATA_URI}}") {
		t.Error("{{FDA_GUIDE_DATA_URI}} placeholder was not substituted")
	}
}

func TestBuildHTML_FDAGrantedTrue(t *testing.T) {
	s := &state{}
	s.fdaGranted.Store(true)
	got := buildHTML(s)
	if !strings.Contains(got, "fdaGranted") || !strings.Contains(got, "= true") {
		t.Error("expected fdaGranted=true in HTML when state.fdaGranted is true")
	}
}
