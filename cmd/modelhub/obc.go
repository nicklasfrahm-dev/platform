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

	fmt.Println("==> Applying ObjectBucketClaim...")

	applyCmd := kubectl(ctx, cfg, "apply", "-f", "-")
	applyCmd.Stdin = bytes.NewBufferString(manifest)

	if out, err := applyCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("apply OBC: %w\n%s", err, out)
	}

	fmt.Println("==> Waiting for OBC to be Bound...")

	if err := waitForOBCBound(ctx, cfg); err != nil {
		return nil, err
	}

	return loadCredentials(ctx, cfg)
}

func waitForOBCBound(ctx context.Context, cfg *globalConfig) error {
	deadline := time.Now().Add(150 * time.Second)

	for {
		out, err := kubectl(ctx, cfg,
			"get", "objectbucketclaim", "models",
			"-n", cfg.namespace,
			"-o", "jsonpath={.status.phase}",
		).Output()

		if err == nil && string(out) == "Bound" {
			fmt.Println("    OBC is Bound.")

			return nil
		}

		phase := "pending"
		if err == nil {
			phase = string(out)
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("%w (last phase: %s)", errOBCTimeout, phase)
		}

		fmt.Printf("    Phase: %s...\n", phase)

		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled waiting for OBC: %w", ctx.Err())
		case <-time.After(5 * time.Second):
		}
	}
}

// loadCredentials extracts bucket name, S3 keys, and HF token from the cluster.
func loadCredentials(ctx context.Context, cfg *globalConfig) (*clusterCreds, error) {
	fmt.Println("==> Extracting S3 credentials...")

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

	fmt.Printf("    Bucket: %s\n", bucketName)

	return &clusterCreds{
		bucketName: bucketName,
		accessKey:  accessKey,
		secretKey:  secretKey,
		hfToken:    hfToken,
	}, nil
}

func kubectl(ctx context.Context, cfg *globalConfig, args ...string) *exec.Cmd {
	fullArgs := append([]string{"--context", cfg.kubeContext}, args...) //nolint:gocritic

	return exec.CommandContext(ctx, "kubectl", fullArgs...) //nolint:gosec
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
