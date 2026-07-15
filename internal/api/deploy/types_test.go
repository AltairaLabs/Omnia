package deploy

import "testing"

func TestValidate(t *testing.T) {
	valid := DeployIntent{
		APIVersion: APIVersionV1,
		Pack:       PackIntent{Name: "p", Version: "1.0.0", Content: "{}"},
		Agents:     []AgentIntent{{Name: "a", Providers: []ProviderBind{{Name: "default", Ref: "claude"}}}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid intent rejected: %v", err)
	}

	for name, mut := range map[string]func(*DeployIntent){
		"bad apiVersion":     func(d *DeployIntent) { d.APIVersion = "deploy.omnia.altairalabs.ai/v2" },
		"empty pack name":    func(d *DeployIntent) { d.Pack.Name = "" },
		"empty pack version": func(d *DeployIntent) { d.Pack.Version = "" },
		"no agents":          func(d *DeployIntent) { d.Agents = nil },
		"empty agent name":   func(d *DeployIntent) { d.Agents[0].Name = "" },
		"agent no providers": func(d *DeployIntent) { d.Agents[0].Providers = nil },
	} {
		d := valid
		d.Agents = append([]AgentIntent(nil), valid.Agents...)
		d.Agents[0].Providers = append([]ProviderBind(nil), valid.Agents[0].Providers...)
		mut(&d)
		if err := d.Validate(); err == nil {
			t.Errorf("%s: expected validation error, got nil", name)
		}
	}
}

func TestValidate_RuntimeQuantities(t *testing.T) {
	base := DeployIntent{
		APIVersion: APIVersionV1,
		Pack:       PackIntent{Name: "p", Version: "1.0.0", Content: "{}"},
		Agents:     []AgentIntent{{Name: "a", Providers: []ProviderBind{{Name: "default", Ref: "claude"}}}},
	}

	for name, mut := range map[string]func(*DeployIntent){
		"malformed cpu":    func(d *DeployIntent) { d.Agents[0].Runtime = &RuntimeIntent{CPU: "not-a-quantity"} },
		"malformed memory": func(d *DeployIntent) { d.Agents[0].Runtime = &RuntimeIntent{Memory: "bad"} },
	} {
		d := base
		d.Agents = append([]AgentIntent(nil), base.Agents...)
		mut(&d)
		if err := d.Validate(); err == nil {
			t.Errorf("%s: expected validation error, got nil", name)
		}
	}

	valid := base
	valid.Agents = append([]AgentIntent(nil), base.Agents...)
	valid.Agents[0].Runtime = &RuntimeIntent{CPU: "500m", Memory: "256Mi"}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid runtime quantities rejected: %v", err)
	}
}
