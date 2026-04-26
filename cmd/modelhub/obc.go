package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

// bucketManifestTpl is the OBC manifest; namespace is substituted at runtime.
const bucketManifestTpl = `apiVersion: objectbucket.io/v1alpha1
kind: ObjectBucketClaim
metadata:
  name: models
  namespace: %s
spec:
  generateBucketName: models
  storageClassName: ceph-bucket
`

const obcTimeoutSeconds = 150
const obcWaitIntervalSeconds = 5

// clusterCreds holds the S3 and HuggingFace secrets extracted from Kubernetes.
type clusterCreds struct {
	bucketName string
	accessKey  string
	secretKey  string
	hfToken    string
}

var errOBCTimeout = errors.New("OBC did not become Bound within 150s")

// ensureOBC applies the ObjectBucketClaim, waits until Bound, and returns
// the S3 and HuggingFace credentials extracted from the cluster.
func ensureOBC(ctx context.Context, cfg *globalConfig) (*clusterCreds, error) {
	manifest := fmt.Sprintf(bucketManifestTpl, cfg.namespace)

	applyCmd := kubectl(ctx, cfg, "apply", "-f", "-")
	applyCmd.Stdin = bytes.NewBufferString(manifest)

	out, err := applyCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("apply OBC: %w\n%s", err, out)
	}

	err = waitForOBCBound(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return loadCredentials(ctx, cfg)
}

func waitForOBCBound(ctx context.Context, cfg *globalConfig) error {
	deadline := time.Now().Add(obcTimeoutSeconds * time.Second)

	for {
		out, err := kubectl(ctx, cfg,
			"get", "objectbucketclaim", "models",
			"-n", cfg.namespace,
			"-o", "jsonpath={.status.phase}",
		).Output()

		if err == nil && string(out) == "Bound" {
			return nil
		}

		phase := "pending"
		if err == nil {
			phase = string(out)
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("%w (last phase: %s)", errOBCTimeout, phase)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled waiting for OBC: %w", ctx.Err())
		case <-time.After(obcWaitIntervalSeconds * time.Second):
		}
	}
}

// loadCredentials extracts bucket name, S3 keys, and HF token from the cluster.
func loadCredentials(ctx context.Context, cfg *globalConfig) (*clusterCreds, error) {
	bucketName, err := kubectlField(ctx, cfg, "configmap", "models", "data.BUCKET_NAME")
	if err != nil {
		return nil, fmt.Errorf("get bucket name: %w", err)
	}

	accessKey, err := kubectlSecretField(ctx, cfg, "models", "data.AWS_ACCESS_KEY_ID")
	if err != nil {
		return nil, fmt.Errorf("get access key: %w", err)
	}

	secretKey, err := kubectlSecretField(ctx, cfg, "models", "data.AWS_SECRET_ACCESS_KEY")
	if err != nil {
		return nil, fmt.Errorf("get secret key: %w", err)
	}

	hfToken, err := kubectlSecretField(ctx, cfg, "hf-secret", "data.HF_TOKEN")
	if err != nil {
		return nil, fmt.Errorf("get HF token: %w", err)
	}

	return &clusterCreds{
		bucketName: bucketName,
		accessKey:  accessKey,
		secretKey:  secretKey,
		hfToken:    hfToken,
	}, nil
}

func kubectl(ctx context.Context, cfg *globalConfig, args ...string) *exec.Cmd {
	fullArgs := append([]string{"--context", cfg.kubeContext}, args...)
	
	// #nosec G204 - These arguments are controlled by the program and are not user-provided
	return exec.CommandContext(ctx, "kubectl", fullArgs...)
}

func kubectlField(ctx context.Context, cfg *globalConfig, resource, name, jsonpath string) (string, error) {
	out, err := kubectl(ctx, cfg,
		"get", resource, name,
		"-n", cfg.namespace,
		"-o", fmt.Sprintf("jsonpath={.%s}", jsonpath),
	).Output()
	if err != nil {
		return "", fmt.Errorf("kubectl get %s %s: %w", resource, name, err)
	}

	return string(out), nil
}

func kubectlSecretField(ctx context.Context, cfg *globalConfig, secretName, jsonpath string) (string, error) {
	encoded, err := kubectlField(ctx, cfg, "secret", secretName, jsonpath)
	if err != nil {
		return "", err
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode %s %s: %w", secretName, jsonpath, err)
	}

	return string(decoded), nil
}
