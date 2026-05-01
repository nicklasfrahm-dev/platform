package main

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

var versionRE = regexp.MustCompile(`^v(\d+)$`)

// newS3Client creates an S3 client configured for the given endpoint and credentials.
func newS3Client(endpoint, accessKey, secretKey string) *s3.Client {
	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
	}

	return s3.NewFromConfig(cfg, func(opts *s3.Options) {
		opts.BaseEndpoint = aws.String(endpoint)
		opts.UsePathStyle = true
	})
}

// modelName derives a normalised storage name from a HuggingFace repo path.
// Example: "Qwen/Qwen3-Coder-480B-A35B-FP8" → "qwen3-coder-480b-a35b-fp8".
func modelName(hfRepo string) string {
	parts := strings.SplitN(hfRepo, "/", 2) //nolint:mnd

	return strings.ToLower(strings.ReplaceAll(parts[len(parts)-1], "/", "-"))
}

// nextVersion returns the next version string (e.g. "v3") for a model by
// inspecting the highest existing versioned prefix under <model>/ in the bucket.
func nextVersion(ctx context.Context, client *s3.Client, bucket, model string) (string, error) {
	versions, err := listVersions(ctx, client, bucket, model)
	if err != nil {
		return "", err
	}

	if len(versions) == 0 {
		return "v1", nil
	}

	maxVer := 0

	for _, ver := range versions {
		match := versionRE.FindStringSubmatch(ver)
		if match == nil {
			continue
		}

		num, _ := strconv.Atoi(match[1])
		if num > maxVer {
			maxVer = num
		}
	}

	return fmt.Sprintf("v%d", maxVer+1), nil
}

// listVersions returns the sorted list of versioned prefixes for a model.
func listVersions(ctx context.Context, client *s3.Client, bucket, model string) ([]string, error) {
	prefix := model + "/"

	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket:    aws.String(bucket),
		Prefix:    aws.String(prefix),
		Delimiter: aws.String("/"),
	})

	var versions []string

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list versions for %s: %w", model, err)
		}

		for _, commonPrefix := range page.CommonPrefixes {
			segment := strings.TrimSuffix(strings.TrimPrefix(*commonPrefix.Prefix, prefix), "/")
			if versionRE.MatchString(segment) {
				versions = append(versions, segment)
			}
		}
	}

	sort.Strings(versions)

	return versions, nil
}

// listModels returns the sorted list of top-level model names in the bucket.
func listModels(ctx context.Context, client *s3.Client, bucket string) ([]string, error) {
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("/"),
	})

	var models []string

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list models: %w", err)
		}

		for _, commonPrefix := range page.CommonPrefixes {
			models = append(models, strings.TrimSuffix(*commonPrefix.Prefix, "/"))
		}
	}

	sort.Strings(models)

	return models, nil
}

// deletePrefix removes all objects whose key starts with the given prefix and
// returns the number of deleted objects.
func deletePrefix(ctx context.Context, client *s3.Client, bucket, prefix string) (int, error) {
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})

	deleted := 0

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return deleted, fmt.Errorf("list objects for deletion: %w", err)
		}

		if len(page.Contents) == 0 {
			continue
		}

		identifiers := make([]types.ObjectIdentifier, len(page.Contents))
		for idx, obj := range page.Contents {
			identifiers[idx] = types.ObjectIdentifier{Key: obj.Key}
		}

		_, err = client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(bucket),
			Delete: &types.Delete{Objects: identifiers},
		})
		if err != nil {
			return deleted, fmt.Errorf("delete objects: %w", err)
		}

		deleted += len(identifiers)
	}

	return deleted, nil
}
