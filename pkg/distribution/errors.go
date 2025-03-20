package distribution

import (
	"errors"
	"fmt"

	"github.com/docker/model-distribution/pkg/types"
)

var (
	ErrInvalidReference     = errors.New("invalid model reference")
	ErrModelNotFound        = errors.New("model not found")
	ErrUnauthorized         = errors.New("unauthorized access to model")
	ErrUnsupportedMediaType = errors.New(fmt.Sprintf(
		"client supports only models of type %q and older - try upgrading",
		types.MediaTypeModelConfigV01,
	))
)

// ReferenceError represents an error related to an invalid model reference
type ReferenceError struct {
	Reference string
	Err       error
}

func (e *ReferenceError) Error() string {
	return fmt.Sprintf("invalid model reference %q: %v", e.Reference, e.Err)
}

func (e *ReferenceError) Unwrap() error {
	return e.Err
}

// Is implements error matching for ReferenceError
func (e *ReferenceError) Is(target error) bool {
	return target == ErrInvalidReference
}

// PullError represents an error that occurs when pulling a model
type PullError struct {
	Reference string
	Code      string // "UNAUTHORIZED" or "MANIFEST_UNKNOWN"
	Message   string
	Err       error
}

func (e *PullError) Error() string {
	return fmt.Sprintf("failed to pull model %q: %s - %s", e.Reference, e.Code, e.Message)
}

func (e *PullError) Unwrap() error {
	return e.Err
}

// Is implements error matching for PullError
func (e *PullError) Is(target error) bool {
	switch target {
	case ErrModelNotFound:
		return e.Code == "MANIFEST_UNKNOWN"
	case ErrUnauthorized:
		return e.Code == "UNAUTHORIZED"
	default:
		return false
	}
}

// NewReferenceError creates a new ReferenceError
func NewReferenceError(reference string, err error) error {
	return &ReferenceError{
		Reference: reference,
		Err:       err,
	}
}

// NewPullError creates a new PullError
func NewPullError(reference, code, message string, err error) error {
	return &PullError{
		Reference: reference,
		Code:      code,
		Message:   message,
		Err:       err,
	}
}

// ArtifactTypeError occurs when the model config json schema is newer than supported by the client
type ArtifactTypeError struct {
	Reference string
	MediaType string
	error
}

// NewArtifactVersionError creates a new ArtifactTypeError
func NewArtifactVersionError(reference, mediaType string) error {
	return &ArtifactTypeError{
		Reference: reference,
		MediaType: mediaType,
	}
}

func (e *ArtifactTypeError) Error() string {
	return fmt.Sprintf("model at reference %q has unsupported config media type %q - please upgrade to support newer versions",
		e.Reference, e.MediaType)
}
