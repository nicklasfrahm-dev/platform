package main

import "errors"

var (
	errMissingDep      = errors.New("required command not found")
	errNodesFailed     = errors.New("node(s) failed")
	errTalosconfig     = errors.New("talosconfig not found in gen/ or cluster root — run 'tman gen' first")
	errTalosTimeout    = errors.New("timed out waiting for talos")
	errKubeTimeout     = errors.New("timed out waiting for kubernetes node")
	errMissingName     = errors.New("meta.yaml: missing name")
	errMissingEndpoint = errors.New("meta.yaml: missing endpoint")
	errNoPools         = errors.New("meta.yaml: no pools defined")
	errPoolMissingName = errors.New("meta.yaml: pool missing name")
	errInvalidRole     = errors.New("role must be 'controlplane' or 'worker'")
	errMissingHost     = errors.New("missing host")
)
