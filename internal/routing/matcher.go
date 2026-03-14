package routing

import (
	"github.com/orvice/loki-gateway/internal/config"
)

func MatchTargets(labels map[string]string, rules []config.RouteRule) []string {
	matched := make([]string, 0)
	seen := make(map[string]struct{})

	for _, rule := range rules {
		if !ruleMatches(labels, rule.Match) {
			continue
		}
		if _, ok := seen[rule.Target]; ok {
			continue
		}
		seen[rule.Target] = struct{}{}
		matched = append(matched, rule.Target)
	}

	return matched
}

func ruleMatches(labels map[string]string, match map[string]string) bool {
	for k, v := range match {
		if labels[k] != v {
			return false
		}
	}
	return true
}
