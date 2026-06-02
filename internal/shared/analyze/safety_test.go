package analyze

import "testing"

func TestClassifySafetyMetadata(t *testing.T) {
	tests := []struct {
		name string
		meta SafetyMetadata
		want RiskLevel
	}{
		{name: "forbidden", meta: SafetyMetadata{IsForbidden: true}, want: RiskForbidden},
		{name: "protected", meta: SafetyMetadata{IsProtectedFunction: true}, want: RiskProtected},
		{name: "secret args", meta: SafetyMetadata{SecretArguments: "NotAllowed"}, want: RiskSecret},
		{name: "taint", meta: SafetyMetadata{SecretArguments: "AllowedWhenUntainted"}, want: RiskTaintSensitive},
		{name: "conditional", meta: SafetyMetadata{SecretWhenUnitSpellCastRestricted: true}, want: RiskConditionalSecret},
		{name: "never secret", meta: SafetyMetadata{ReturnsNeverSecret: true}, want: RiskNeverSecret},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifySafety(tt.meta).Level; got != tt.want {
				t.Fatalf("level = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExplainUnitCastScenarioIncludesFieldAdvice(t *testing.T) {
	meta := SafetyMetadata{
		SecretWhenUnitSpellCastRestricted: true,
		Fields: []SafetyField{
			{Name: "target", ConditionalSecret: true},
			{Name: "castBarID", NeverSecret: true},
		},
	}
	expl := ExplainSafety(meta, "unit_cast")
	if expl.EffectiveLevel != RiskConditionalSecret || len(expl.Why) < 2 || len(expl.AddonAdvice) == 0 {
		t.Fatalf("bad explanation: %#v", expl)
	}
}
