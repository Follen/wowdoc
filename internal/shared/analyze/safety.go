package analyze

type RiskLevel string

const (
	RiskSafe              RiskLevel = "safe"
	RiskNeverSecret       RiskLevel = "never_secret"
	RiskTaintSensitive    RiskLevel = "taint_sensitive"
	RiskConditionalSecret RiskLevel = "conditional_secret"
	RiskSecret            RiskLevel = "secret"
	RiskProtected         RiskLevel = "protected"
	RiskForbidden         RiskLevel = "forbidden"
	RiskUnknown           RiskLevel = "unknown"
)

type SafetyMetadata struct {
	SecretArguments                           string
	SecretArgumentsAddAspect                  []string
	SecretReturnsForAspect                    []string
	SecretWhenCooldownsRestricted             bool
	SecretWhenUnitSpellCastRestricted         bool
	SecretInChatMessagingLockdown             bool
	RequiresNonSecretAura                     bool
	RequiresRestrictedAbbreviationBreakpoints bool
	IsProtectedFunction                       bool
	ConstSecretAccessor                       bool
	ReturnsNeverSecret                        bool
	NeverSecret                               bool
	ConditionalSecret                         bool
	IsForbidden                               bool
	SetForbidden                              bool
	IsPreventingSecretValues                  bool
	SecretWrapperConstant                     string
	RestrictedTypes                           []string
	Fields                                    []SafetyField
}

type SafetyField struct {
	Name              string `json:"name"`
	ConditionalSecret bool   `json:"conditionalSecret,omitempty"`
	NeverSecret       bool   `json:"neverSecret,omitempty"`
}

type SafetyClassification struct {
	Level  RiskLevel     `json:"level"`
	Fields []SafetyField `json:"fields,omitempty"`
}

type SafetyExplanation struct {
	Scenario       string    `json:"scenario"`
	EffectiveLevel RiskLevel `json:"effectiveLevel"`
	Why            []string  `json:"why"`
	AddonAdvice    []string  `json:"addonAdvice"`
}

func ClassifySafety(meta SafetyMetadata) SafetyClassification {
	level := RiskSafe
	switch {
	case meta.IsForbidden || meta.SetForbidden:
		level = RiskForbidden
	case meta.IsProtectedFunction:
		level = RiskProtected
	case meta.SecretArguments == "NotAllowed" || meta.SecretWrapperConstant == "AlwaysSecret":
		level = RiskSecret
	case meta.SecretArguments == "AllowedWhenUntainted":
		level = RiskTaintSensitive
	case meta.ConditionalSecret || meta.SecretWhenCooldownsRestricted || meta.SecretWhenUnitSpellCastRestricted ||
		meta.SecretInChatMessagingLockdown || meta.RequiresNonSecretAura || meta.SecretWrapperConstant == "ContextuallySecret" ||
		meta.IsPreventingSecretValues || len(meta.SecretArgumentsAddAspect) > 0 || len(meta.SecretReturnsForAspect) > 0 ||
		len(meta.RestrictedTypes) > 0:
		level = RiskConditionalSecret
	case meta.NeverSecret || meta.ReturnsNeverSecret || meta.SecretWrapperConstant == "NeverSecret":
		level = RiskNeverSecret
	}
	return SafetyClassification{Level: level, Fields: meta.Fields}
}

func ExplainSafety(meta SafetyMetadata, scenario string) SafetyExplanation {
	classified := ClassifySafety(meta)
	expl := SafetyExplanation{Scenario: scenario, EffectiveLevel: classified.Level}
	if meta.SecretWhenUnitSpellCastRestricted {
		expl.Why = append(expl.Why, "SecretWhenUnitSpellCastRestricted is true")
	}
	if meta.IsPreventingSecretValues {
		expl.Why = append(expl.Why, "IsPreventingSecretValues is true")
	}
	for _, restricted := range meta.RestrictedTypes {
		expl.Why = append(expl.Why, "restricted type "+restricted+" is present")
	}
	for _, field := range meta.Fields {
		if field.ConditionalSecret {
			expl.Why = append(expl.Why, "return field "+field.Name+" is ConditionalSecret")
		}
		if field.NeverSecret {
			expl.Why = append(expl.Why, field.Name+" is NeverSecret and can be used safely")
		}
	}
	if len(expl.Why) == 0 {
		expl.Why = append(expl.Why, "no unsafe metadata matched")
	}
	expl.AddonAdvice = []string{
		"Treat secret or conditional fields as possibly unavailable.",
		"Do not use secret values to mutate secure UI during combat.",
		"Check nil and use secret-safe fallbacks.",
	}
	for _, field := range meta.Fields {
		if field.ConditionalSecret {
			expl.AddonAdvice = append(expl.AddonAdvice, "Treat "+field.Name+" as possibly unavailable or secret.")
		}
		if field.NeverSecret {
			expl.AddonAdvice = append(expl.AddonAdvice, field.Name+" is marked never secret and can be used as a safer fallback.")
		}
	}
	return expl
}
