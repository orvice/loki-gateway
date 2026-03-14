package config

import "testing"

func TestValidateRejectsUnknownDefaultTarget(t *testing.T) {
	cfg := LokiConfig{
		DefaultTarget: "missing",
		Targets: []LokiTarget{
			{Name: "loki-a", URL: "http://localhost:3100"},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for unknown default target")
	}
}

func TestValidateRejectsRuleWithUnknownTarget(t *testing.T) {
	cfg := LokiConfig{
		DefaultTarget: "loki-a",
		Targets: []LokiTarget{
			{Name: "loki-a", URL: "http://localhost:3100"},
		},
		Rules: []RouteRule{
			{Name: "bad", Match: map[string]string{"env": "prod"}, Target: "loki-b"},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for unknown rule target")
	}
}

func TestValidateAcceptsMinimalValidConfig(t *testing.T) {
	cfg := LokiConfig{
		DefaultTarget: "loki-a",
		Targets: []LokiTarget{
			{Name: "loki-a", URL: "http://localhost:3100"},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
	if cfg.Targets[0].TimeoutMS <= 0 {
		t.Fatalf("expected timeout default to be set")
	}
}

func TestValidateRejectsBasicAuthWithoutUsername(t *testing.T) {
	cfg := LokiConfig{
		DefaultTarget: "loki-a",
		Targets: []LokiTarget{
			{Name: "loki-a", URL: "http://localhost:3100", BasicAuth: BasicAuth{Password: "secret"}},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for basic auth missing username")
	}
}

func TestValidateRejectsBasicAuthWithoutPassword(t *testing.T) {
	cfg := LokiConfig{
		DefaultTarget: "loki-a",
		Targets: []LokiTarget{
			{Name: "loki-a", URL: "http://localhost:3100", BasicAuth: BasicAuth{Username: "alice"}},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for basic auth missing password")
	}
}

func TestValidateRejectsTargetExtraHeadersWithEmptyKey(t *testing.T) {
	cfg := LokiConfig{
		DefaultTarget: "loki-a",
		Targets: []LokiTarget{
			{Name: "loki-a", URL: "http://localhost:3100", ExtraHeaders: map[string]string{"": "v"}},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for extra header with empty key")
	}
}
