package api

// Subject identifies a principal by OIDC issuer and JWT claim values.
type Subject struct {
	Issuer string            `json:"issuer"`
	Claims map[string]string `json:"claims,omitempty"`
}

// Rule defines a set of permitted operations on resources.
// Use "*" as a wildcard for any verb, resource, or namespace.
type Rule struct {
	Verbs      []string `json:"verbs"`
	Resources  []string `json:"resources"`
	Namespaces []string `json:"namespaces,omitempty"`
}

// PolicySpec binds subjects to access rules.
type PolicySpec struct {
	Subjects []Subject `json:"subjects"`
	Rules    []Rule    `json:"rules"`
}

// PolicyStatus is reserved for future use.
type PolicyStatus struct{}

// Policy is a Kubernetes-style resource that grants access to subjects.
type Policy struct {
	TypeMeta `json:",inline"`
	Metadata ObjectMeta   `json:"metadata"`
	Spec     PolicySpec   `json:"spec"`
	Status   PolicyStatus `json:"status,omitempty"`
}

// PolicyList is a list of Policy objects.
type PolicyList struct {
	TypeMeta `json:",inline"`
	Metadata ListMeta `json:"metadata"`
	Items    []Policy `json:"items"`
}
