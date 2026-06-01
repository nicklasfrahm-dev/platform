// Package handler implements HTTP handlers for the skatd API.
package handler

import (
	"encoding/json"
	"net/http"
)

type apiVersions struct {
	Kind       string   `json:"kind"`
	APIVersion string   `json:"apiVersion"`
	Versions   []string `json:"versions"`
}

type apiGroupList struct {
	Kind       string     `json:"kind"`
	APIVersion string     `json:"apiVersion"`
	Groups     []apiGroup `json:"groups"`
}

type apiGroup struct {
	Name             string         `json:"name"`
	Versions         []groupVersion `json:"versions"`
	PreferredVersion groupVersion   `json:"preferredVersion"`
}

type groupVersion struct {
	GroupVersion string `json:"groupVersion"`
	Version      string `json:"version"`
}

type apiResourceList struct {
	Kind         string        `json:"kind"`
	APIVersion   string        `json:"apiVersion"`
	GroupVersion string        `json:"groupVersion"`
	APIResources []apiResource `json:"resources"`
}

type apiResource struct {
	Name         string   `json:"name"`
	SingularName string   `json:"singularName"`
	Namespaced   bool     `json:"namespaced"`
	Kind         string   `json:"kind"`
	Verbs        []string `json:"verbs"`
	ShortNames   []string `json:"shortNames,omitempty"`
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// APIVersions handles GET /api — returns the legacy core API versions.
func APIVersions(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, apiVersions{
		Kind:       "APIVersions",
		APIVersion: "v1",
		Versions:   []string{"v1"},
	})
}

// CoreResourceList handles GET /api/v1 — core group has no skatd resources.
func CoreResourceList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, apiResourceList{
		Kind:         "APIResourceList",
		APIVersion:   "v1",
		GroupVersion: "v1",
		APIResources: []apiResource{},
	})
}

// APIGroupList handles GET /apis — lists the skatd.io API group.
func APIGroupList(w http.ResponseWriter, _ *http.Request) {
	gv := groupVersion{GroupVersion: "skatd.io/v1", Version: "v1"}
	writeJSON(w, apiGroupList{
		Kind:       "APIGroupList",
		APIVersion: "v1",
		Groups: []apiGroup{
			{
				Name:             "skatd.io",
				Versions:         []groupVersion{gv},
				PreferredVersion: gv,
			},
		},
	})
}

// APIResourceListV1 handles GET /apis/skatd.io/v1 — lists secrets and policies.
func APIResourceListV1(w http.ResponseWriter, _ *http.Request) {
	verbs := []string{"get", "list", "create", "update", "delete"}
	writeJSON(w, apiResourceList{
		Kind:         "APIResourceList",
		APIVersion:   "v1",
		GroupVersion: "skatd.io/v1",
		APIResources: []apiResource{
			{Name: "secrets", SingularName: "secret", Namespaced: true, Kind: "Secret", Verbs: verbs},
			{Name: "policies", SingularName: "policy", Namespaced: true, Kind: "Policy", Verbs: verbs},
		},
	})
}
