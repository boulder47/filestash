package plg_authorisation_rules

import (
	"encoding/json"
	"strings"
)

// Rule is one access rule. Empty/"*" fields match anything. Effect is
// "allow" or "deny". Matching is first-rule-wins; if no rule matches, the
// configured default effect applies.
type Rule struct {
	Users   []string `json:"users"`
	Paths   []string `json:"paths"`   // path prefixes, or "*"/"/"
	Actions []string `json:"actions"` // "read", "write", or "*"
	Effect  string   `json:"effect"`
}

// parseRules decodes the JSON rules blob. An empty string yields no rules; a
// parse error is returned so the caller can fail open (allow) and log.
func parseRules(blob string) ([]Rule, error) {
	blob = strings.TrimSpace(blob)
	if blob == "" {
		return nil, nil
	}
	var rules []Rule
	if err := json.Unmarshal([]byte(blob), &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

func inList(list []string, v string) bool {
	for _, x := range list {
		if x == "*" || x == v {
			return true
		}
	}
	return false
}

func pathMatch(prefixes []string, path string) bool {
	for _, p := range prefixes {
		if p == "*" || p == "/" {
			return true
		}
		p = strings.TrimRight(p, "/")
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

func (r Rule) matches(user, action, path string) bool {
	return inList(r.Users, user) && inList(r.Actions, action) && pathMatch(r.Paths, path)
}

// evaluate returns true if the (user, action, path) is allowed. First matching
// rule wins; with no match, defaultEffect decides ("deny" => false).
func evaluate(rules []Rule, defaultEffect, user, action, path string) bool {
	for _, r := range rules {
		if r.matches(user, action, path) {
			return r.Effect != "deny"
		}
	}
	return defaultEffect != "deny"
}
