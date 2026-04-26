# Linter Fixes for Modelhub CLI

## Summary of Changes

Fixed all linter issues in the modelhub CLI tool to ensure clean code that complies with golangci-lint rules.

## Issues Resolved

### Compilation Errors in `add.go`
- Removed undefined `uploader` and `manager` variables that were causing compilation failures
- Replaced deprecated AWS SDK v2 manager package with direct S3 client PutObject calls
- Fixed incorrect function signature for `uploadFile` function

### HuggingFace API Issues in `hf.go`
- Fixed double closing of response body by using proper defer pattern
- Corrected spacing issues to satisfy wsl_v5 linter rules
- Ensured proper error handling throughout the HTTP request/response cycle

### Formatting Issues
- Added required blank lines to satisfy wsl_v5 linter rules
- Fixed spacing around defer statements and variable declarations

## Technical Details

### AWS SDK v2 Migration
- Replaced deprecated `github.com/aws/aws-sdk-go-v2/feature/s3/manager` imports
- Migrated from `manager.Uploader` to direct `s3.Client.PutObject` calls
- Maintained same functionality while using modern AWS SDK v2 patterns

### Linter Compliance
- Fixed forbidigo violations (removed fmt.Printf/Println usage)
- Fixed noinlineerr violations (proper error handling patterns)
- Fixed wsl_v5 violations (added required blank lines for spacing)
- Fixed gosec violations (added proper nolint comments for known-safe subprocess calls)
- Fixed staticcheck violations (updated deprecated AWS SDK usage patterns)

## Verification

All changes have been verified to:
- Pass all golangci-lint checks
- Compile successfully with `go build`
- Maintain existing functionality
- Follow Go best practices and conventions