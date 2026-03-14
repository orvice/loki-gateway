package config

import (
	"fmt"
)

const defaultTimeoutMS = 3000

type LokiConfig struct {
	DefaultTarget string       `yaml:"default_target"`
	Targets       []LokiTarget `yaml:"targets"`
	Rules         []RouteRule  `yaml:"rules"`
}

type LokiTarget struct {
	Name      string `yaml:"name"`
	URL       string `yaml:"url"`
	TenantID  string `yaml:"tenant_id"`
	BasicAuth BasicAuth `yaml:"basic_auth"`
	ExtraHeaders map[string]string `yaml:"extra_headers"`
	TimeoutMS int    `yaml:"timeout_ms"`
}

type BasicAuth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type RouteRule struct {
	Name   string            `yaml:"name"`
	Match  map[string]string `yaml:"match"`
	Target string            `yaml:"target"`
}

func (c *LokiConfig) Validate() error {
	if c == nil {
		return fmt.Errorf("loki config is nil")
	}

	if c.DefaultTarget == "" {
		return fmt.Errorf("loki.default_target is required")
	}

	if len(c.Targets) == 0 {
		return fmt.Errorf("loki.targets must contain at least one target")
	}

	targets := make(map[string]struct{}, len(c.Targets))
	for i := range c.Targets {
		t := &c.Targets[i]
		if t.Name == "" {
			return fmt.Errorf("loki.targets[%d].name is required", i)
		}
		if t.URL == "" {
			return fmt.Errorf("loki.targets[%d].url is required", i)
		}
		if _, exists := targets[t.Name]; exists {
			return fmt.Errorf("duplicate loki target name: %s", t.Name)
		}
		if t.BasicAuth.Username == "" && t.BasicAuth.Password != "" {
			return fmt.Errorf("loki.targets[%d].basic_auth.username is required when password is set", i)
		}
		if t.BasicAuth.Password == "" && t.BasicAuth.Username != "" {
			return fmt.Errorf("loki.targets[%d].basic_auth.password is required when username is set", i)
		}
		for k := range t.ExtraHeaders {
			if k == "" {
				return fmt.Errorf("loki.targets[%d].extra_headers contains empty key", i)
			}
		}
		targets[t.Name] = struct{}{}
		if t.TimeoutMS <= 0 {
			t.TimeoutMS = defaultTimeoutMS
		}
	}

	if _, ok := targets[c.DefaultTarget]; !ok {
		return fmt.Errorf("loki.default_target references unknown target: %s", c.DefaultTarget)
	}

	for i, r := range c.Rules {
		if r.Target == "" {
			return fmt.Errorf("loki.rules[%d].target is required", i)
		}
		if _, ok := targets[r.Target]; !ok {
			return fmt.Errorf("loki.rules[%d].target references unknown target: %s", i, r.Target)
		}
	}

	return nil
}

func (c LokiConfig) TargetByName(name string) (LokiTarget, bool) {
	for _, t := range c.Targets {
		if t.Name == name {
			return t, true
		}
	}
	return LokiTarget{}, false
}
