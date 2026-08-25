package migration

// StaticMappingRegistry is the allowlisted process-local adapter registry used
// by composition roots and tests. It has no registration method, so callers
// cannot introduce a source/table mapping after startup.
type StaticMappingRegistry struct {
	definitions map[AdapterID]AdapterDefinition
}

func NewStaticMappingRegistry(definitions ...AdapterDefinition) (*StaticMappingRegistry, error) {
	registry := &StaticMappingRegistry{definitions: make(map[AdapterID]AdapterDefinition, len(definitions))}
	for _, definition := range definitions {
		if definition.Source == nil || definition.Mapper == nil || definition.Cursors == nil || definition.Manifest.Validate() != nil {
			return nil, ErrInvalidManifest
		}
		if _, exists := registry.definitions[definition.Manifest.ID]; exists {
			return nil, ErrInvalidManifest
		}
		registry.definitions[definition.Manifest.ID] = definition
	}
	return registry, nil
}

func (registry *StaticMappingRegistry) Lookup(id AdapterID) (AdapterDefinition, bool) {
	if registry == nil {
		return AdapterDefinition{}, false
	}
	definition, found := registry.definitions[id]
	return definition, found
}

// StaticPolicyRegistry seals the six closed dispositions at startup.
type StaticPolicyRegistry struct{ policies map[PolicyID]Policy }

func NewStaticPolicyRegistry(policies ...Policy) (*StaticPolicyRegistry, error) {
	registry := &StaticPolicyRegistry{policies: make(map[PolicyID]Policy, len(policies))}
	for _, policy := range policies {
		if !policy.valid() {
			return nil, ErrUnknownPolicy
		}
		if _, exists := registry.policies[policy.ID]; exists {
			return nil, ErrUnknownPolicy
		}
		registry.policies[policy.ID] = policy
	}
	return registry, nil
}

func (registry *StaticPolicyRegistry) Lookup(id PolicyID) (Policy, bool) {
	if registry == nil {
		return Policy{}, false
	}
	policy, found := registry.policies[id]
	return policy, found
}
