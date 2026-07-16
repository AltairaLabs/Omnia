package deploy

import (
	"testing"

	"k8s.io/utils/ptr"
)

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
		"empty pack content": func(d *DeployIntent) { d.Pack.Content = "" },
		"no agents":          func(d *DeployIntent) { d.Agents = nil },
		"empty agent name":   func(d *DeployIntent) { d.Agents[0].Name = "" },
		"agent no providers": func(d *DeployIntent) { d.Agents[0].Providers = nil },
		"provider empty name": func(d *DeployIntent) {
			d.Agents[0].Providers[0].Name = ""
		},
		"provider empty ref": func(d *DeployIntent) {
			d.Agents[0].Providers[0].Ref = ""
		},
		"rollout with no steps": func(d *DeployIntent) {
			d.Agents[0].Rollout = &RolloutIntent{Trigger: &RolloutTriggerIntent{PromptPackChannel: "stable"}}
		},
		"rollout trigger with no channel": func(d *DeployIntent) {
			d.Agents[0].Rollout = &RolloutIntent{
				Trigger: &RolloutTriggerIntent{},
				Steps:   []RolloutStepIntent{{SetWeight: ptr.To(int32(25))}},
			}
		},
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

// TestValidate_Rollout covers the positive counterpart of the "rollout with
// no steps" / "rollout trigger with no channel" rejection cases in
// TestValidate: a rollout with at least one step and a populated trigger
// channel passes.
func TestValidate_Rollout(t *testing.T) {
	base := DeployIntent{
		APIVersion: APIVersionV1,
		Pack:       PackIntent{Name: "p", Version: "1.0.0", Content: "{}"},
		Agents:     []AgentIntent{{Name: "a", Providers: []ProviderBind{{Name: "default", Ref: "claude"}}}},
	}

	valid := base
	valid.Agents = append([]AgentIntent(nil), base.Agents...)
	valid.Agents[0].Rollout = &RolloutIntent{
		Trigger: &RolloutTriggerIntent{PromptPackChannel: "stable"},
		Steps:   []RolloutStepIntent{{SetWeight: ptr.To(int32(25))}},
	}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid rollout (trigger + step) rejected: %v", err)
	}
}

// baseDeployIntent returns a fresh minimal-valid DeployIntent for the
// Plan B validation tests below (tools/policy/externalAuth/memory).
func baseDeployIntent() DeployIntent {
	return DeployIntent{
		APIVersion: APIVersionV1,
		Pack:       PackIntent{Name: "p", Version: "1.0.0", Content: "{}"},
		Agents:     []AgentIntent{{Name: "a", Providers: []ProviderBind{{Name: "default", Ref: "claude"}}}},
	}
}

func TestValidate_Tools(t *testing.T) {
	valid := map[string]*ToolsIntent{
		"nil tools":     nil,
		"ref only":      {Ref: "reg1"},
		"handlers only": {Handlers: []HandlerIntent{{Name: "h1", Type: "http"}}},
	}
	for name, tools := range valid {
		d := baseDeployIntent()
		d.Tools = tools
		if err := d.Validate(); err != nil {
			t.Errorf("%s: valid tools rejected: %v", name, err)
		}
	}

	invalid := map[string]*ToolsIntent{
		"ref and handlers mutually exclusive": {Ref: "reg1", Handlers: []HandlerIntent{{Name: "h1", Type: "http"}}},
		"handler missing name":                {Handlers: []HandlerIntent{{Type: "http"}}},
		"handler missing type":                {Handlers: []HandlerIntent{{Name: "h1"}}},
		"handler invalid type":                {Handlers: []HandlerIntent{{Name: "h1", Type: "ftp"}}},
	}
	for name, tools := range invalid {
		d := baseDeployIntent()
		d.Tools = tools
		if err := d.Validate(); err == nil {
			t.Errorf("%s: expected validation error, got nil", name)
		}
	}
}

func TestValidate_Policy(t *testing.T) {
	valid := []struct {
		name   string
		policy *PolicyIntent
		tools  *ToolsIntent
	}{
		{"nil policy", nil, nil},
		{"empty blocklist, no tools needed", &PolicyIntent{}, nil},
		{"blocklist with ref tools", &PolicyIntent{ToolBlocklist: []string{"t1"}}, &ToolsIntent{Ref: "reg1"}},
		{"blocklist with handler tools", &PolicyIntent{ToolBlocklist: []string{"t1"}}, &ToolsIntent{Handlers: []HandlerIntent{{Name: "h1", Type: "http"}}}},
	}
	for _, tc := range valid {
		d := baseDeployIntent()
		d.Policy = tc.policy
		d.Tools = tc.tools
		if err := d.Validate(); err != nil {
			t.Errorf("%s: valid policy rejected: %v", tc.name, err)
		}
	}

	invalid := []struct {
		name   string
		policy *PolicyIntent
		tools  *ToolsIntent
	}{
		{"blocklist with no tools", &PolicyIntent{ToolBlocklist: []string{"t1"}}, nil},
		{"blocklist with empty tools", &PolicyIntent{ToolBlocklist: []string{"t1"}}, &ToolsIntent{}},
	}
	for _, tc := range invalid {
		d := baseDeployIntent()
		d.Policy = tc.policy
		d.Tools = tc.tools
		if err := d.Validate(); err == nil {
			t.Errorf("%s: expected validation error, got nil", tc.name)
		}
	}
}

func TestValidate_ExternalAuth(t *testing.T) {
	valid := map[string]*ExternalAuthIntent{
		"nil externalAuth": nil,
		"clientKeys only":  {ClientKeys: &ClientKeysIntent{DefaultRole: "user"}},
		"oidc valid":       {OIDC: &OIDCIntent{Issuer: "https://issuer.example.com", Audience: "aud"}},
	}
	for name, ea := range valid {
		d := baseDeployIntent()
		d.Agents[0].ExternalAuth = ea
		if err := d.Validate(); err != nil {
			t.Errorf("%s: valid externalAuth rejected: %v", name, err)
		}
	}

	invalid := map[string]*ExternalAuthIntent{
		"oidc missing issuer":   {OIDC: &OIDCIntent{Audience: "aud"}},
		"oidc missing audience": {OIDC: &OIDCIntent{Issuer: "https://issuer.example.com"}},
	}
	for name, ea := range invalid {
		d := baseDeployIntent()
		d.Agents[0].ExternalAuth = ea
		if err := d.Validate(); err == nil {
			t.Errorf("%s: expected validation error, got nil", name)
		}
	}
}

func TestValidate_Memory(t *testing.T) {
	valid := map[string]*MemoryIntent{
		"nil memory":              nil,
		"empty strategy":          {Retrieval: &MemoryRetrievalIntent{}},
		"strategy keyword":        {Retrieval: &MemoryRetrievalIntent{Strategy: "keyword"}},
		"strategy semantic":       {Retrieval: &MemoryRetrievalIntent{Strategy: "semantic"}},
		"strategy composite":      {Retrieval: &MemoryRetrievalIntent{Strategy: "composite"}},
		"no retrieval, just tool": {Tools: &MemoryToolsIntent{Enabled: ptr.To(true)}},
	}
	for name, m := range valid {
		d := baseDeployIntent()
		d.Agents[0].Memory = m
		if err := d.Validate(); err != nil {
			t.Errorf("%s: valid memory rejected: %v", name, err)
		}
	}

	invalid := map[string]*MemoryIntent{
		"invalid strategy": {Retrieval: &MemoryRetrievalIntent{Strategy: "bogus"}},
	}
	for name, m := range invalid {
		d := baseDeployIntent()
		d.Agents[0].Memory = m
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
