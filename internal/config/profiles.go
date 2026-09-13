package config

import "fmt"

type ProviderProfile struct {
	Name           string            `json:"name"`
	Program        string            `json:"program"`
	Provider       string            `json:"provider"`
	Model          string            `json:"model"`
	Temperature    float64           `json:"temperature"`
	Env            map[string]string `json:"env"`
	ToolAllowList  []string          `json:"tool_allow_list"`
	HookPolicyName string            `json:"hook_policy_name"`
}

func (c Config) Profile(name string) (ProviderProfile, error) {
	for _, p := range c.Profiles {
		if p.Name == name {
			return p, nil
		}
	}
	return ProviderProfile{}, fmt.Errorf("profile not found: %s", name)
}

func (c Config) DefaultProfile() (ProviderProfile, error) {
	if c.DefaultProvider == "" {
		if len(c.Profiles) == 0 {
			return ProviderProfile{}, fmt.Errorf("no profiles configured")
		}
		return c.Profiles[0], nil
	}
	return c.Profile(c.DefaultProvider)
}
