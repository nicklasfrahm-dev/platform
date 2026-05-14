package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/nicklasfrahm-dev/platform/cmd/skatd/api"
	"github.com/nicklasfrahm-dev/platform/cmd/skatd/authz"
	"github.com/nicklasfrahm-dev/platform/cmd/skatd/store"
)

// PoliciesHandler handles CRUD operations for Policy resources.
// After any mutation it reloads the authz engine with the full policy set.
type PoliciesHandler struct {
	Store  store.Store
	Engine authz.Engine
}

func (h *PoliciesHandler) List(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("ns")
	resources, err := h.Store.List(r.Context(), store.KindPolicy, ns)
	if err != nil {
		api.WriteStatus(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}
	list := api.PolicyList{
		TypeMeta: api.TypeMeta{APIVersion: api.APIVersion, Kind: "PolicyList"},
		Metadata: api.ListMeta{},
		Items:    make([]api.Policy, 0, len(resources)),
	}
	for _, res := range resources {
		p, convErr := resourceToPolicy(res)
		if convErr != nil {
			api.WriteStatus(w, http.StatusInternalServerError, "InternalError", convErr.Error())
			return
		}
		list.Items = append(list.Items, *p)
	}
	api.WriteObject(w, http.StatusOK, list)
}

func (h *PoliciesHandler) Get(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("ns"), r.PathValue("name")
	res, err := h.Store.Get(r.Context(), store.KindPolicy, ns, name)
	if errors.Is(err, store.ErrNotFound) {
		api.WriteStatus(w, http.StatusNotFound, "NotFound", "policy "+name+" not found")
		return
	}
	if err != nil {
		api.WriteStatus(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}
	p, err := resourceToPolicy(res)
	if err != nil {
		api.WriteStatus(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}
	api.WriteObject(w, http.StatusOK, p)
}

func (h *PoliciesHandler) Create(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("ns")
	var p api.Policy
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		api.WriteStatus(w, http.StatusBadRequest, "BadRequest", "invalid JSON: "+err.Error())
		return
	}
	if p.Metadata.Name == "" {
		api.WriteStatus(w, http.StatusBadRequest, "BadRequest", "metadata.name is required")
		return
	}
	p.Metadata.Namespace = ns
	res, err := policyToResource(&p)
	if err != nil {
		api.WriteStatus(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}
	created, err := h.Store.Create(r.Context(), res)
	if errors.Is(err, store.ErrAlreadyExists) {
		api.WriteStatus(w, http.StatusConflict, "AlreadyExists", "policy "+p.Metadata.Name+" already exists")
		return
	}
	if err != nil {
		api.WriteStatus(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}
	if reloadErr := h.reload(r.Context()); reloadErr != nil {
		api.WriteStatus(w, http.StatusInternalServerError, "InternalError", reloadErr.Error())
		return
	}
	result, err := resourceToPolicy(created)
	if err != nil {
		api.WriteStatus(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}
	api.WriteObject(w, http.StatusCreated, result)
}

func (h *PoliciesHandler) Update(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("ns"), r.PathValue("name")
	var p api.Policy
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		api.WriteStatus(w, http.StatusBadRequest, "BadRequest", "invalid JSON: "+err.Error())
		return
	}
	p.Metadata.Namespace = ns
	p.Metadata.Name = name
	res, err := policyToResource(&p)
	if err != nil {
		api.WriteStatus(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}
	updated, err := h.Store.Update(r.Context(), res)
	if errors.Is(err, store.ErrNotFound) {
		api.WriteStatus(w, http.StatusNotFound, "NotFound", "policy "+name+" not found")
		return
	}
	if errors.Is(err, store.ErrConflict) {
		api.WriteStatus(w, http.StatusConflict, "Conflict", "resource version conflict")
		return
	}
	if err != nil {
		api.WriteStatus(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}
	if reloadErr := h.reload(r.Context()); reloadErr != nil {
		api.WriteStatus(w, http.StatusInternalServerError, "InternalError", reloadErr.Error())
		return
	}
	result, err := resourceToPolicy(updated)
	if err != nil {
		api.WriteStatus(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}
	api.WriteObject(w, http.StatusOK, result)
}

func (h *PoliciesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("ns"), r.PathValue("name")
	err := h.Store.Delete(r.Context(), store.KindPolicy, ns, name)
	if errors.Is(err, store.ErrNotFound) {
		api.WriteStatus(w, http.StatusNotFound, "NotFound", "policy "+name+" not found")
		return
	}
	if err != nil {
		api.WriteStatus(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}
	if reloadErr := h.reload(r.Context()); reloadErr != nil {
		api.WriteStatus(w, http.StatusInternalServerError, "InternalError", reloadErr.Error())
		return
	}
	api.WriteObject(w, http.StatusOK, api.Status{
		TypeMeta: api.TypeMeta{APIVersion: "v1", Kind: "Status"},
		Status:   "Success",
		Code:     http.StatusOK,
	})
}

// reload lists all policies and reloads the authz engine.
func (h *PoliciesHandler) reload(ctx context.Context) error {
	resources, err := h.Store.List(ctx, store.KindPolicy, "")
	if err != nil {
		return err
	}
	policies := make([]api.Policy, 0, len(resources))
	for _, res := range resources {
		p, convErr := resourceToPolicy(res)
		if convErr != nil {
			return convErr
		}
		policies = append(policies, *p)
	}
	h.Engine.Reload(policies)
	return nil
}

// policyToResource converts a Policy to a store.Resource.
func policyToResource(p *api.Policy) (*store.Resource, error) {
	raw, err := json.Marshal(p.Spec)
	if err != nil {
		return nil, err
	}
	return &store.Resource{
		Name:            p.Metadata.Name,
		Namespace:       p.Metadata.Namespace,
		Kind:            store.KindPolicy,
		ResourceVersion: p.Metadata.ResourceVersion,
		Labels:          p.Metadata.Labels,
		Annotations:     p.Metadata.Annotations,
		Raw:             raw,
	}, nil
}

// resourceToPolicy converts a store.Resource back to a Policy.
func resourceToPolicy(res *store.Resource) (*api.Policy, error) {
	var spec api.PolicySpec
	if err := json.Unmarshal(res.Raw, &spec); err != nil {
		return nil, err
	}
	return &api.Policy{
		TypeMeta: api.TypeMeta{APIVersion: api.APIVersion, Kind: "Policy"},
		Metadata: api.ObjectMeta{
			Name:              res.Name,
			Namespace:         res.Namespace,
			UID:               res.UID,
			ResourceVersion:   res.ResourceVersion,
			Labels:            res.Labels,
			Annotations:       res.Annotations,
			CreationTimestamp: res.CreatedAt,
		},
		Spec:   spec,
		Status: api.PolicyStatus{},
	}, nil
}
