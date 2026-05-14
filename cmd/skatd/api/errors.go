package api

import (
	"encoding/json"
	"net/http"
)

// StatusCause describes a cause for a Status error.
type StatusCause struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

// StatusDetails holds extended information about a Status error.
type StatusDetails struct {
	Name   string        `json:"name,omitempty"`
	Kind   string        `json:"kind,omitempty"`
	Causes []StatusCause `json:"causes,omitempty"`
}

// Status mirrors Kubernetes metav1.Status for error responses.
type Status struct {
	TypeMeta `json:",inline"`
	Status   string         `json:"status"`
	Message  string         `json:"message,omitempty"`
	Reason   string         `json:"reason,omitempty"`
	Details  *StatusDetails `json:"details,omitempty"`
	Code     int            `json:"code"`
}

// WriteStatus writes a Kubernetes-style Status error response.
func WriteStatus(w http.ResponseWriter, code int, reason, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(Status{
		TypeMeta: TypeMeta{APIVersion: "v1", Kind: "Status"},
		Status:   "Failure",
		Message:  message,
		Reason:   reason,
		Code:     code,
	})
}

// WriteObject writes any value as a JSON response.
func WriteObject(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
