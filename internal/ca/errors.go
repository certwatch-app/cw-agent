// Package ca provides Certificate Authority bundle loading and validation.
package ca

import (
	"errors"
	"fmt"
)

var (
	// ErrFileNotFound is returned when a CA bundle file doesn't exist
	ErrFileNotFound = errors.New("ca bundle file not found")

	// ErrInvalidPEM is returned when a file contains invalid PEM data
	ErrInvalidPEM = errors.New("invalid PEM data in ca bundle")

	// ErrEmptyBundle is returned when no valid certificates are found in a bundle
	ErrEmptyBundle = errors.New("ca bundle contains no valid certificates")

	// ErrInsecurePermissions is returned when a CA bundle has world-writable permissions
	ErrInsecurePermissions = errors.New("ca bundle has insecure permissions (world-writable)")

	// ErrInvalidFileType is returned when the CA bundle path is not a regular file
	ErrInvalidFileType = errors.New("ca bundle path is not a regular file (symlink or directory)")

	// ErrSystemCAUnavailable is returned when system CAs cannot be loaded
	ErrSystemCAUnavailable = errors.New("system ca certificates are not available")
)

// FileNotFoundError provides detailed information about a missing CA bundle file
type FileNotFoundError struct {
	Path string
}

func (e *FileNotFoundError) Error() string {
	return fmt.Sprintf("ca bundle file not found: %s", e.Path)
}

func (e *FileNotFoundError) Unwrap() error {
	return ErrFileNotFound
}

// InvalidPEMError provides detailed information about invalid PEM data
type InvalidPEMError struct {
	Path   string
	Reason string
}

func (e *InvalidPEMError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("invalid PEM data in %s: %s", e.Path, e.Reason)
	}
	return fmt.Sprintf("invalid PEM data: %s", e.Reason)
}

func (e *InvalidPEMError) Unwrap() error {
	return ErrInvalidPEM
}

// EmptyBundleError provides detailed information about an empty CA bundle
type EmptyBundleError struct {
	Path string
}

func (e *EmptyBundleError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("ca bundle contains no valid certificates: %s", e.Path)
	}
	return "ca bundle contains no valid certificates"
}

func (e *EmptyBundleError) Unwrap() error {
	return ErrEmptyBundle
}

// InsecurePermissionsError provides detailed information about insecure file permissions
type InsecurePermissionsError struct {
	Path        string
	Permissions string
}

func (e *InsecurePermissionsError) Error() string {
	return fmt.Sprintf("ca bundle has insecure permissions: %s (permissions: %s). File must not be world-writable", e.Path, e.Permissions)
}

func (e *InsecurePermissionsError) Unwrap() error {
	return ErrInsecurePermissions
}

// InvalidFileTypeError provides detailed information about invalid file types
type InvalidFileTypeError struct {
	Path string
	Type string // "symlink", "directory", etc.
}

func (e *InvalidFileTypeError) Error() string {
	return fmt.Sprintf("ca bundle path is not a regular file: %s (type: %s)", e.Path, e.Type)
}

func (e *InvalidFileTypeError) Unwrap() error {
	return ErrInvalidFileType
}
