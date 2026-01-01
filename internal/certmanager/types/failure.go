package types

import "strings"

// CategorizeFailure determines the failure category from reason/message.
// This helps group failures by root cause for better debugging and alerting.
func CategorizeFailure(reason, message string) string {
	lower := strings.ToLower(reason + " " + message)

	// Issuer problems - issuer not found, not ready, or misconfigured
	if strings.Contains(lower, "does not exist") ||
		strings.Contains(lower, "issuer not found") ||
		strings.Contains(lower, "no issuer") ||
		(strings.Contains(lower, "issuer") && strings.Contains(lower, "not ready")) ||
		(strings.Contains(lower, "issuer") && strings.Contains(lower, "failed")) {
		return FailureCategoryIssuer
	}

	// ACME protocol issues - rate limits, challenges, orders
	if strings.Contains(lower, "acme") ||
		strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "too many") ||
		strings.Contains(lower, "order") ||
		strings.Contains(lower, "challenge") ||
		strings.Contains(lower, "authorization") ||
		strings.Contains(lower, "dns-01") ||
		strings.Contains(lower, "http-01") ||
		strings.Contains(lower, "tls-alpn") {
		return FailureCategoryACME
	}

	// Validation errors - CSR issues, invalid requests
	if strings.Contains(lower, "invalid") ||
		strings.Contains(lower, "validation") ||
		strings.Contains(lower, "csr") ||
		strings.Contains(lower, "certificate request") ||
		strings.Contains(lower, "malformed") ||
		strings.Contains(lower, "bad request") {
		return FailureCategoryValidation
	}

	// Policy rejection - policy controller denied
	if strings.Contains(lower, "policy") ||
		strings.Contains(lower, "denied") ||
		strings.Contains(lower, "rejected") ||
		strings.Contains(lower, "not allowed") ||
		strings.Contains(lower, "forbidden") {
		return FailureCategoryPolicy
	}

	return FailureCategoryUnknown
}

// IsFailureEvent returns true if the event reason indicates a failure.
// These are the known failure reason strings from cert-manager.
func IsFailureEvent(reason string) bool {
	failureReasons := []string{
		"Failed",
		"DoesNotExist",
		"InvalidRequest",
		"OrderFailed",
		"Denied",
		"Error",
		"ErrInitIssuer",
		"ErrGetKeyPair",
		"ErrIssue",
		"ValidationFailed",
		"ChallengeFailed",
		"IssuerNotFound",
		"IssuerNotReady",
		"MissingData",
	}

	for _, fr := range failureReasons {
		if strings.Contains(reason, fr) {
			return true
		}
	}
	return false
}

// IsSuccessEvent returns true if the event reason indicates success.
func IsSuccessEvent(reason string) bool {
	successReasons := []string{
		"Issued",
		"Ready",
		"Renewed",
		"Generated",
		"IssuerReady",
	}

	for _, sr := range successReasons {
		if strings.Contains(reason, sr) {
			return true
		}
	}
	return false
}

// IsFailureMessage returns true if the message indicates a failure condition.
// This is used to detect failures in CertificateRequest conditions where
// the reason is "Pending" but the message indicates an actual error.
func IsFailureMessage(message string) bool {
	lower := strings.ToLower(message)

	failurePatterns := []string{
		"not found",
		"does not exist",
		"failed",
		"error",
		"denied",
		"rejected",
		"invalid",
		"not ready",
		"not available",
		"timeout",
		"rate limit",
	}

	for _, pattern := range failurePatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}
