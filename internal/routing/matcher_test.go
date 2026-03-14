package routing

import (
	"reflect"
	"testing"

	"github.com/orvice/loki-gateway/internal/config"
)

func TestMatchTargetsExactMatch(t *testing.T) {
	rules := []config.RouteRule{{Name: "prod", Match: map[string]string{"env": "prod"}, Target: "loki-b"}}
	got := MatchTargets(map[string]string{"env": "prod"}, rules)
	want := []string{"loki-b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected targets: got=%v want=%v", got, want)
	}
}

func TestMatchTargetsMultipleHitsAndDedup(t *testing.T) {
	rules := []config.RouteRule{
		{Name: "r1", Match: map[string]string{"env": "prod"}, Target: "loki-b"},
		{Name: "r2", Match: map[string]string{"team": "core"}, Target: "loki-c"},
		{Name: "r3", Match: map[string]string{"cluster": "cn"}, Target: "loki-b"},
	}
	got := MatchTargets(map[string]string{"env": "prod", "team": "core", "cluster": "cn"}, rules)
	want := []string{"loki-b", "loki-c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected targets: got=%v want=%v", got, want)
	}
}

func TestMatchTargetsNoMatch(t *testing.T) {
	rules := []config.RouteRule{{Name: "prod", Match: map[string]string{"env": "prod"}, Target: "loki-b"}}
	got := MatchTargets(map[string]string{"env": "staging"}, rules)
	if len(got) != 0 {
		t.Fatalf("expected no match, got=%v", got)
	}
}
