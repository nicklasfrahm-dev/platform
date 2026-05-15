package api

import "time"

const (
	APIGroup   = "skatd.io"
	APIVersion = "skatd.io/v1"
	Version    = "v1"
)

// TypeMeta describes the type of a Kubernetes-style resource.
type TypeMeta struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

// ObjectMeta contains metadata common to all resources.
type ObjectMeta struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace,omitempty"`
	UID               string            `json:"uid,omitempty"`
	ResourceVersion   string            `json:"resourceVersion,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	CreationTimestamp time.Time         `json:"creationTimestamp,omitempty"`
}

// ListMeta contains metadata for list objects.
type ListMeta struct {
	ResourceVersion string `json:"resourceVersion"`
}
