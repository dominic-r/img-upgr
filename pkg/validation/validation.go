package validation

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

// Error implements the error interface
func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error for %s: %s", e.Field, e.Message)
}

// ValidationErrors is a collection of validation errors
type ValidationErrors struct {
	Errors []*ValidationError
}

// Error implements the error interface
func (e *ValidationErrors) Error() string {
	if len(e.Errors) == 0 {
		return "no validation errors"
	}

	var sb strings.Builder
	sb.WriteString("validation errors:\n")
	for _, err := range e.Errors {
		sb.WriteString("  - ")
		sb.WriteString(err.Error())
		sb.WriteString("\n")
	}
	return sb.String()
}

// HasErrors returns true if there are any validation errors
func (e *ValidationErrors) HasErrors() bool {
	return len(e.Errors) > 0
}

// Add adds a validation error
func (e *ValidationErrors) Add(field, message string) {
	e.Errors = append(e.Errors, &ValidationError{
		Field:   field,
		Message: message,
	})
}

// AddIf adds a validation error if the condition is true
func (e *ValidationErrors) AddIf(condition bool, field, message string) {
	if condition {
		e.Add(field, message)
	}
}

// ValidateLogLevel validates a log level
func ValidateLogLevel(level string, validLevels []string) error {
	if level == "" {
		return nil
	}

	upperLevel := strings.ToUpper(level)
	for _, validLevel := range validLevels {
		if upperLevel == validLevel {
			return nil
		}
	}

	return fmt.Errorf("invalid log level: %s (valid levels: %s)",
		level, strings.Join(validLevels, ", "))
}

// ValidateDirectory checks if a directory exists
func ValidateDirectory(path string) error {
	if path == "" {
		return nil
	}

	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("directory does not exist: %s", path)
	}
	if err != nil {
		return fmt.Errorf("error accessing directory: %s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}

	return nil
}

// ValidateFile checks if a file exists
func ValidateFile(path string) error {
	if path == "" {
		return nil
	}

	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %s", path)
	}
	if err != nil {
		return fmt.Errorf("error accessing file: %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("path is a directory, not a file: %s", path)
	}

	return nil
}

// ValidateURL validates a URL
func ValidateURL(urlStr string) error {
	if urlStr == "" {
		return nil
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL: %s (parse error: %w)", urlStr, err)
	}

	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return fmt.Errorf("invalid URL: %s (missing scheme or host)", urlStr)
	}

	return nil
}

// ValidateNotEmpty checks if a string is not empty
func ValidateNotEmpty(value, fieldName string) error {
	if value == "" {
		return fmt.Errorf("%s cannot be empty", fieldName)
	}
	return nil
}

// ValidateFileOrDir checks if a path exists, whether it's a file or directory
func ValidateFileOrDir(path string) error {
	if path == "" {
		return nil
	}

	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", path)
	}
	if err != nil {
		return fmt.Errorf("error accessing path: %s: %w", path, err)
	}

	return nil
}

// ValidatePathInDir checks if a path is within a directory
func ValidatePathInDir(path, dir string) error {
	if path == "" || dir == "" {
		return nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("error getting absolute path for %s: %w", path, err)
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("error getting absolute path for %s: %w", dir, err)
	}

	if !strings.HasPrefix(absPath, absDir) {
		return fmt.Errorf("path %s is not within directory %s", path, dir)
	}

	return nil
}

// IsEmpty checks if a value is empty
func IsEmpty(value string) bool {
	return value == ""
}

// IsNotEmpty checks if a value is not empty
func IsNotEmpty(value string) bool {
	return value != ""
}

// IsMissingRequiredVar checks if an environment variable is missing
func IsMissingRequiredVar(value, envVarName string, required bool) bool {
	return required && value == ""
}

// GetMissingVars returns a list of missing environment variables
func GetMissingVars(requiredVars map[string]string) []string {
	var missing []string
	for name, value := range requiredVars {
		if value == "" {
			missing = append(missing, name)
		}
	}
	return missing
}

// ValidateRequiredVars validates that required variables are not empty
func ValidateRequiredVars(requiredVars map[string]string) error {
	missing := GetMissingVars(requiredVars)
	if len(missing) > 0 {
		return fmt.Errorf("missing required variables: %s", strings.Join(missing, ", "))
	}
	return nil
}

// IsValidOutputFormat checks if an output format is valid
func IsValidOutputFormat(format string, validFormats []string) bool {
	for _, validFormat := range validFormats {
		if format == validFormat {
			return true
		}
	}
	return false
}

// CombineErrors combines multiple errors into one
func CombineErrors(errs ...error) error {
	var nonNilErrs []error
	for _, err := range errs {
		if err != nil {
			nonNilErrs = append(nonNilErrs, err)
		}
	}

	if len(nonNilErrs) == 0 {
		return nil
	}

	if len(nonNilErrs) == 1 {
		return nonNilErrs[0]
	}

	return errors.Join(nonNilErrs...)
}
