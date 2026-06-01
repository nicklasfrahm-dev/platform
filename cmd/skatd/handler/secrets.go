package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/nicklasfrahm-dev/platform/cmd/skatd/api"
	"github.com/nicklasfrahm-dev/platform/cmd/skatd/crypto"
	"github.com/nicklasfrahm-dev/platform/cmd/skatd/store"
)

// SecretsHandler handles CRUD operations for Secret resources.
type SecretsHandler struct {
	Store store.Store
	Enc   crypto.Encryptor
}

func (h *SecretsHandler) List(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("ns")
	resources, err := h.Store.List(r.Context(), store.KindSecret, ns)
	if err != nil {
		api.WriteStatus(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}
	list := api.SecretList{
		TypeMeta: api.TypeMeta{APIVersion: api.APIVersion, Kind: "SecretList"},
		Metadata: api.ListMeta{},
		Items:    make([]api.Secret, 0, len(resources)),
	}
	for _, res := range resources {
		s, convErr := resourceToSecret(res, h.Enc)
		if convErr != nil {
			api.WriteStatus(w, http.StatusInternalServerError, "InternalError", convErr.Error())
			return
		}
		list.Items = append(list.Items, *s)
	}
	api.WriteObject(w, http.StatusOK, list)
}

func (h *SecretsHandler) Get(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("ns"), r.PathValue("name")
	res, err := h.Store.Get(r.Context(), store.KindSecret, ns, name)
	if errors.Is(err, store.ErrNotFound) {
		api.WriteStatus(w, http.StatusNotFound, "NotFound", "secret "+name+" not found")
		return
	}
	if err != nil {
		api.WriteStatus(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}
	s, err := resourceToSecret(res, h.Enc)
	if err != nil {
		api.WriteStatus(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}
	api.WriteObject(w, http.StatusOK, s)
}

func (h *SecretsHandler) Create(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("ns")
	var s api.Secret
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		api.WriteStatus(w, http.StatusBadRequest, "BadRequest", "invalid JSON: "+err.Error())
		return
	}
	if s.Metadata.Name == "" {
		api.WriteStatus(w, http.StatusBadRequest, "BadRequest", "metadata.name is required")
		return
	}
	s.Metadata.Namespace = ns
	res, err := secretToResource(&s, h.Enc)
	if err != nil {
		api.WriteStatus(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}
	created, err := h.Store.Create(r.Context(), res)
	if errors.Is(err, store.ErrAlreadyExists) {
		api.WriteStatus(w, http.StatusConflict, "AlreadyExists", "secret "+s.Metadata.Name+" already exists")
		return
	}
	if err != nil {
		api.WriteStatus(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}
	result, err := resourceToSecret(created, h.Enc)
	if err != nil {
		api.WriteStatus(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}
	api.WriteObject(w, http.StatusCreated, result)
}

func (h *SecretsHandler) Update(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("ns"), r.PathValue("name")
	var s api.Secret
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		api.WriteStatus(w, http.StatusBadRequest, "BadRequest", "invalid JSON: "+err.Error())
		return
	}
	s.Metadata.Namespace = ns
	s.Metadata.Name = name
	res, err := secretToResource(&s, h.Enc)
	if err != nil {
		api.WriteStatus(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}
	updated, err := h.Store.Update(r.Context(), res)
	if errors.Is(err, store.ErrNotFound) {
		api.WriteStatus(w, http.StatusNotFound, "NotFound", "secret "+name+" not found")
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
	result, err := resourceToSecret(updated, h.Enc)
	if err != nil {
		api.WriteStatus(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}
	api.WriteObject(w, http.StatusOK, result)
}

func (h *SecretsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("ns"), r.PathValue("name")
	err := h.Store.Delete(r.Context(), store.KindSecret, ns, name)
	if errors.Is(err, store.ErrNotFound) {
		api.WriteStatus(w, http.StatusNotFound, "NotFound", "secret "+name+" not found")
		return
	}
	if err != nil {
		api.WriteStatus(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}
	api.WriteObject(w, http.StatusOK, api.Status{
		TypeMeta: api.TypeMeta{APIVersion: "v1", Kind: "Status"},
		Status:   "Success",
		Code:     http.StatusOK,
	})
}

// secretToResource converts a Secret to a store.Resource, encrypting spec.data values.
func secretToResource(s *api.Secret, enc crypto.Encryptor) (*store.Resource, error) {
	encrypted := make(map[string]string, len(s.Spec.Data))
	for k, v := range s.Spec.Data {
		ct, err := crypto.EncryptString(enc, v)
		if err != nil {
			return nil, err
		}
		encrypted[k] = ct
	}
	raw, err := json.Marshal(api.SecretSpec{Data: encrypted})
	if err != nil {
		return nil, err
	}
	return &store.Resource{
		Name:            s.Metadata.Name,
		Namespace:       s.Metadata.Namespace,
		Kind:            store.KindSecret,
		ResourceVersion: s.Metadata.ResourceVersion,
		Labels:          s.Metadata.Labels,
		Annotations:     s.Metadata.Annotations,
		Raw:             raw,
	}, nil
}

// resourceToSecret converts a store.Resource back to a Secret, decrypting spec.data values.
func resourceToSecret(res *store.Resource, enc crypto.Encryptor) (*api.Secret, error) {
	var spec api.SecretSpec
	if err := json.Unmarshal(res.Raw, &spec); err != nil {
		return nil, err
	}
	decrypted := make(map[string]string, len(spec.Data))
	for k, v := range spec.Data {
		pt, err := crypto.DecryptString(enc, v)
		if err != nil {
			return nil, err
		}
		decrypted[k] = pt
	}
	return &api.Secret{
		TypeMeta: api.TypeMeta{APIVersion: api.APIVersion, Kind: "Secret"},
		Metadata: api.ObjectMeta{
			Name:              res.Name,
			Namespace:         res.Namespace,
			UID:               res.UID,
			ResourceVersion:   res.ResourceVersion,
			Labels:            res.Labels,
			Annotations:       res.Annotations,
			CreationTimestamp: res.CreatedAt,
		},
		Spec:   api.SecretSpec{Data: decrypted},
		Status: api.SecretStatus{},
	}, nil
}
