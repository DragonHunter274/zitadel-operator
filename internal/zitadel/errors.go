package zitadel

import "fmt"

// APIError represents an error from the Zitadel API.
type APIError struct {
	StatusCode int
	Code       int    `json:"code"`
	Message    string `json:"message"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("zitadel API error (HTTP %d, code %d): %s", e.StatusCode, e.Code, e.Message)
}

func IsNotFound(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.StatusCode == 404
	}
	return false
}

func IsConflict(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.StatusCode == 409 || apiErr.Code == 6 // ALREADY_EXISTS
	}
	return false
}
