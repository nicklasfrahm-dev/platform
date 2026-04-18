// Package main is the entry point for the nodeprofiler CLI.
package main

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var version = "dev"

func main() {
	logger, err := setupLogger()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stdout, "error: failed to setup logger: %v\n", err)
	}

	rootCommand := RootCommand(logger)

	err = rootCommand.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func setupLogger() (*zap.Logger, error) {
	cfg := zap.NewDevelopmentConfig()
	cfg.EncoderConfig.EncodeLevel = zapcore.LowercaseColorLevelEncoder
	cfg.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder

	logger, err := cfg.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build logger config: %w", err)
	}

	return logger, nil
}
