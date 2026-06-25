package instrumentationdevice

import (
	"regexp"
	"testing"
)

func TestSelectRegexRuleUsesLastMatchingRule(t *testing.T) {
	rules := []workloadNameRegexRule{
		{
			kinds: map[string]struct{}{"Deployment": {}},
			regex: regexp.MustCompile(`^(?P<base>.+)-v?(?P<version>[0-9]+)$`),
		},
		{
			kinds: map[string]struct{}{"Deployment": {}},
			regex: regexp.MustCompile(`^(?P<base>.+)-v(?P<version>[0-9]+)$`),
		},
	}

	selected, match, matchedRules := selectRegexRule(rules, "checkout-v2", "Deployment")
	if matchedRules != 2 {
		t.Fatalf("expected 2 matching rules, got %d", matchedRules)
	}
	if selected == nil || selected.regex.String() != rules[1].regex.String() {
		t.Fatalf("expected last matching rule to be selected")
	}
	if match == nil || match.base != "checkout" || match.version != 2 {
		t.Fatalf("unexpected match: %+v", match)
	}
}

func TestRegexRuleRequiresWholeNameMatch(t *testing.T) {
	rule := workloadNameRegexRule{
		regex: regexp.MustCompile(`(?P<base>.+)-v(?P<version>[0-9]+)`),
	}

	if _, ok := rule.matchName("checkout-v2-extra"); ok {
		t.Fatalf("expected partial regex match to be rejected")
	}
}
