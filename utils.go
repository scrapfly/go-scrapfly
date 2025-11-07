package scrapfly

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"time"
)

// urlSafeB64Encode encodes data into URL-safe base64 format.
// This is used internally for encoding JS code and other parameters.
func urlSafeB64Encode(data string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(data))
}

// fetchWithRetry performs an HTTP request with automatic retry logic for 5xx errors.
//
// It retries the request up to the specified number of times with a delay between attempts.
// Only server errors (5xx status codes) and network errors are retried.
// The request body must support re-reading via req.GetBody for retries to work properly.
func fetchWithRetry(client *http.Client, req *http.Request, retries int, delay time.Duration) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt < retries; attempt++ {
		// We need to be able to re-read the body on retries
		var bodyReader io.ReadCloser
		if req.Body != nil {
			var err error
			// GetBody is a function that returns a new reader for the request body
			// This is essential for retries as the body can only be read once.
			bodyReader, err = req.GetBody()
			if err != nil {
				return nil, err
			}
			req.Body = bodyReader
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			DefaultLogger.Debug("request failed:", err, "retrying...")
			time.Sleep(delay)
			continue
		}

		if resp.StatusCode >= 500 && resp.StatusCode < 600 {
			resp.Body.Close() // Close body to prevent resource leaks
			lastErr = &APIError{Message: "server error", HTTPStatusCode: resp.StatusCode}
			DefaultLogger.Debug("request failed with status", resp.StatusCode, "retrying...")
			time.Sleep(delay)
			continue
		}

		return resp, nil
	}
	return nil, lastErr
}

// ValidateExclusiveFields checks a struct for fields marked with the "exclusive" tag
// and ensures that only one field per exclusive group is set.
func ValidateExclusiveFields(s interface{}) error {
	v := reflect.ValueOf(s)

	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return errors.New("input must be a struct")
	}

	exclusiveGroups := make(map[string]string)

	for i := 0; i < v.NumField(); i++ {
		field := v.Type().Field(i)
		tag := field.Tag.Get("exclusive")

		if tag != "" {
			fieldValue := v.Field(i)

			if !fieldValue.IsZero() {
				if existingField, found := exclusiveGroups[tag]; found {
					return fmt.Errorf("fields %s and %s are mutually exclusive", existingField, field.Name)
				}
				exclusiveGroups[tag] = field.Name
			}
		}
	}

	return nil
}

// ValidateRequiredFields checks a struct for fields with the `required:"true"` tag
// and returns an error if any of them are zero-valued.
func ValidateRequiredFields(s interface{}) error {
	v := reflect.ValueOf(s)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return errors.New("input must be a struct or a pointer to a struct")
	}

	for i := 0; i < v.NumField(); i++ {
		field := v.Type().Field(i)
		if field.Tag.Get("required") == "true" {
			if v.Field(i).IsZero() {
				return fmt.Errorf("field '%s' is required but was not set", field.Name)
			}
		}
	}

	return nil
}

// ValidateEnums checks fields tagged with `validate:"enum"`.
// It calls the IsValid() bool method on the field if it's a single value,
// or on each element if it's a slice.
func ValidateEnums(s interface{}) error {
	v := reflect.ValueOf(s)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return errors.New("input must be a struct or a pointer to a struct")
	}

	for i := 0; i < v.NumField(); i++ {
		field := v.Type().Field(i)
		if field.Tag.Get("validate") == "enum" {
			fieldValue := v.Field(i)

			// Skip validation for zero-valued fields (e.g., "" or nil slice).
			// The `required` tag should be used to enforce that a field is set.
			if fieldValue.IsZero() {
				continue
			}

			// Handle slices and single values differently.
			if fieldValue.Kind() == reflect.Slice {
				// It's a slice, so iterate over its elements.
				for j := 0; j < fieldValue.Len(); j++ {
					elem := fieldValue.Index(j)
					if err := validateSingleEnumValue(elem, field.Name); err != nil {
						return err
					}
				}
			} else {
				// It's a single value.
				if err := validateSingleEnumValue(fieldValue, field.Name); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// validateSingleEnumValue is a helper that checks if a reflect.Value has a valid
// IsValid() bool method and returns an error if the method returns false.
func validateSingleEnumValue(v reflect.Value, fieldName string) error {
	// Find the IsValid method by name.
	isValidMethod := v.MethodByName("IsValid")

	// Check if the method exists.
	if !isValidMethod.IsValid() {
		return fmt.Errorf("field '%s' is tagged for enum validation but its type '%s' does not have an IsValid() bool method", fieldName, v.Type())
	}

	// Call the method. IsValid() takes no arguments.
	results := isValidMethod.Call(nil)

	// Ensure the method returns exactly one value, and that it's a boolean.
	if len(results) != 1 || results[0].Kind() != reflect.Bool {
		return fmt.Errorf("method IsValid() on field '%s' does not return a single boolean value", fieldName)
	}

	// If the result is false, the enum is invalid.
	if !results[0].Bool() {
		// v.Interface() will call the String() method, giving a nice error message.
		return fmt.Errorf("field '%s' contains an invalid enum value: '%v'", fieldName, v.Interface())
	}

	return nil
}
