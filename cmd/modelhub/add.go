package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"
)

func newAddCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "add <hf-repo>",
		Short: "Import a HuggingFace model into S3 storage.",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			return runAdd(ctx, cfg, args[0])
		},
	}
}

func runAdd(ctx context.Context, cfg *globalConfig, hfRepo string) error {
	creds, err := ensureOBC(ctx, cfg)
	if err != nil {
		return fmt.Errorf("ensure OBC: %w", err)
	}

	s3Client := newS3Client(cfg.endpoint, creds.accessKey, creds.secretKey)
	uploader := manager.NewUploader(s3Client)

	name := modelName(hfRepo)

	version, err := nextVersion(ctx, s3Client, creds.bucketName, name)
	if err != nil {
		return fmt.Errorf("determine next version: %w", err)
	}

	fmt.Printf("==> Importing %s as %s/%s\n", hfRepo, name, version)

	files, err := hfListFiles(ctx, creds.hfToken, hfRepo)
	if err != nil {
		return fmt.Errorf("list HuggingFace files: %w", err)
	}

	fmt.Printf("    Found %d files.\n", len(files))

	for _, file := range files {
		if err := uploadFile(ctx, uploader, creds, name, version, hfRepo, file); err != nil {
			return err
		}
	}

	fmt.Printf("==> Done: %s → s3://%s/%s/%s/\n", hfRepo, creds.bucketName, name, version)

	return nil
}

func uploadFile(
	ctx context.Context,
	uploader *manager.Uploader,
	creds *clusterCreds,
	name, version, hfRepo, file string,
) error {
	fmt.Printf("  -> %s\n", file)

	body, err := hfOpenFile(ctx, creds.hfToken, hfRepo, file)
	if err != nil {
		return fmt.Errorf("open %s: %w", file, err)
	}

	defer body.Close()

	key := fmt.Sprintf("%s/%s/%s", name, version, file)

	_, err = uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(creds.bucketName),
		Key:    aws.String(key),
		Body:   body,
	})
	if err != nil {
		return fmt.Errorf("upload %s: %w", file, err)
	}

	return nil
}
