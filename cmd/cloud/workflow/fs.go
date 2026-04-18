// Package workflow provides reusable workflow steps for file and directory operations.
package workflow

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	dirPermissions  = 0750
	filePermissions = 0600
)

// EnsureDirectory creates a step that ensures the specified directory exists.
func EnsureDirectory(dir string) Step {
	return func() error {
		err := os.MkdirAll(dir, dirPermissions)
		if err != nil {
			return fmt.Errorf("failed to ensure directory: %s: %w", dir, err)
		}

		return nil
	}
}

// EnsureFile ensures that a file with the specified content exists.
// If the file already exists, it will remain unchanged. If the content
// is empty, an empty file will be created.
func EnsureFile(filePath string, content []byte) Step {
	return func() error {
		file, err := os.OpenFile(filepath.Clean(filePath), os.O_CREATE|os.O_RDWR, filePermissions)
		if err != nil {
			return fmt.Errorf("failed to open file: %s: %w", filePath, err)
		}

					defer func() {
				closeErr := file.Close()
				if closeErr != nil && err == nil {
					err = closeErr
				}
			}()

		info, err := file.Stat()
		if err != nil {
			return fmt.Errorf("failed to stat file: %s: %w", filePath, err)
		}

		if info.Size() == 0 && len(content) > 0 {
			_, err = file.Write(content)
			if err != nil {
				return fmt.Errorf("failed to write content to file: %s: %w", filePath, err)
			}
		}

		return nil
	}
}
