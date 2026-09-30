package main

import "testing"

func TestFallbackUsesCurrentAvailableModelsAndPolicyEffort(t *testing.T) {
	policy := routingPolicy{
		Default: routingTarget{Model: "gpt-6-luna", Effort: "medium"},
		ModelSettings: map[string]routingModelSettings{
			"gpt-6-luna": {Enabled: true, Effort: "medium"},
			"gpt-6-sol":  {Enabled: true, Effort: "low"},
			"grok-4.6":   {Enabled: true, Effort: "low"},
		},
	}
	router := newDynamicSmartRouter(&routingPolicyManager{policy: policy}, "", "", nil, []string{"gpt-6-luna", "gpt-6-sol"})
	model, effort := router.fallbackModel("gpt-6-luna")
	if model != "gpt-6-sol" || effort != "low" {
		t.Fatalf("unexpected fallback: %s/%s", model, effort)
	}
	model, _ = router.fallbackModel("gpt-6-sol")
	if model != "gpt-6-luna" {
		t.Fatalf("unexpected default fallback: %s", model)
	}
	router.available = map[string]bool{"gpt-6-luna": true}
	if model, _ := router.fallbackModel("gpt-6-luna"); model != "" {
		t.Fatalf("invented unavailable fallback: %s", model)
	}
}
