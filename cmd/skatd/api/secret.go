package api

// SecretSpec holds the secret data.
type SecretSpec struct {
	Data map[string]string `json:"data,omitempty"`
}

// SecretStatus is reserved for future use.
type SecretStatus struct{}

// Secret is a Kubernetes-style resource that holds sensitive key-value pairs.
type Secret struct {
	TypeMeta `json:",inline"`
	Metadata ObjectMeta   `json:"metadata"`
	Spec     SecretSpec   `json:"spec"`
	Status   SecretStatus `json:"status,omitempty"`
}

// SecretList is a list of Secret objects.
type SecretList struct {
	TypeMeta `json:",inline"`
	Metadata ListMeta `json:"metadata"`
	Items    []Secret `json:"items"`
}
