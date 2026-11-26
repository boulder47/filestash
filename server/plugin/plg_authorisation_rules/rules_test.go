package plg_authorisation_rules

import "testing"

func TestParseRules(t *testing.T) {
	if r, err := parseRules("   "); err != nil || r != nil {
		t.Errorf("blank => nil,nil; got %v,%v", r, err)
	}
	if _, err := parseRules("not json"); err == nil {
		t.Error("invalid JSON should error (caller fails open)")
	}
	r, err := parseRules(`[{"users":["alice"],"paths":["/x"],"actions":["*"],"effect":"deny"}]`)
	if err != nil || len(r) != 1 || r[0].Effect != "deny" {
		t.Fatalf("parse failed: %v %v", r, err)
	}
}

func TestEvaluateDefaults(t *testing.T) {
	if !evaluate(nil, "allow", "alice", "read", "/x") {
		t.Error("no rules + default allow should allow")
	}
	if evaluate(nil, "deny", "alice", "read", "/x") {
		t.Error("no rules + default deny should deny")
	}
}

func TestEvaluateDenyRule(t *testing.T) {
	rules := []Rule{{Users: []string{"*"}, Paths: []string{"/finance"}, Actions: []string{"*"}, Effect: "deny"}}
	if evaluate(rules, "allow", "alice", "read", "/finance/q3.xlsx") {
		t.Error("deny rule should block the /finance subtree")
	}
	if !evaluate(rules, "allow", "alice", "read", "/public/readme") {
		t.Error("unmatched path should fall through to default allow")
	}
}

func TestEvaluateFirstMatchWins(t *testing.T) {
	rules := []Rule{
		{Users: []string{"alice"}, Paths: []string{"/finance/public"}, Actions: []string{"read"}, Effect: "allow"},
		{Users: []string{"*"}, Paths: []string{"/finance"}, Actions: []string{"*"}, Effect: "deny"},
	}
	if !evaluate(rules, "allow", "alice", "read", "/finance/public/x") {
		t.Error("allow exception should win as the first match")
	}
	if evaluate(rules, "allow", "alice", "write", "/finance/public/x") {
		t.Error("write isn't in the allow rule, so the deny rule applies")
	}
	if evaluate(rules, "allow", "bob", "read", "/finance/secret") {
		t.Error("bob should be denied under /finance")
	}
}

func TestMatchers(t *testing.T) {
	if !inList([]string{"*"}, "anything") || !inList([]string{"a", "b"}, "b") || inList([]string{"a"}, "z") {
		t.Error("inList semantics wrong")
	}
	if !pathMatch([]string{"/x"}, "/x") || !pathMatch([]string{"/x"}, "/x/y") {
		t.Error("pathMatch should match exact and subtree")
	}
	if pathMatch([]string{"/x"}, "/xy") {
		t.Error("pathMatch must not treat /xy as under /x")
	}
	if !pathMatch([]string{"*"}, "/anything") || !pathMatch([]string{"/"}, "/anything") {
		t.Error("wildcard/root should match all")
	}
}
