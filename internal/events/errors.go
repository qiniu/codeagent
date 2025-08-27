package events

import (
	"errors"
	"fmt"
)

// Predefined error types for event operations
var (
	// Event parsing errors
	ErrUnsupportedEventType = errors.New("unsupported event type")
	ErrEventNotFound        = errors.New("event not found")
	ErrInvalidEventFormat   = errors.New("invalid event format")
	ErrEventParsingFailed   = errors.New("event parsing failed")

	// Event validation errors
	ErrMissingRepository  = errors.New("missing repository in event")
	ErrMissingSender      = errors.New("missing sender in event")
	ErrMissingIssue       = errors.New("missing issue in event")
	ErrMissingComment     = errors.New("missing comment in event")
	ErrMissingPullRequest = errors.New("missing pull request in event")
	ErrMissingReview      = errors.New("missing review in event")
)

// EventError represents an event-related error with context
type EventError struct {
	Op        string // Operation that failed
	EventType string // Event type (if applicable)
	Err       error  // Underlying error
	Context   string // Additional context
}

func (e *EventError) Error() string {
	if e.EventType != "" && e.Context != "" {
		return fmt.Sprintf("event %s failed for type %s: %v (context: %s)", e.Op, e.EventType, e.Err, e.Context)
	}
	if e.EventType != "" {
		return fmt.Sprintf("event %s failed for type %s: %v", e.Op, e.EventType, e.Err)
	}
	if e.Context != "" {
		return fmt.Sprintf("event %s failed: %v (context: %s)", e.Op, e.Err, e.Context)
	}
	return fmt.Sprintf("event %s failed: %v", e.Op, e.Err)
}

func (e *EventError) Unwrap() error {
	return e.Err
}

// NewEventError creates a new EventError
func NewEventError(op, eventType string, err error, context string) *EventError {
	return &EventError{
		Op:        op,
		EventType: eventType,
		Err:       err,
		Context:   context,
	}
}

// Helper functions for creating specific errors

// UnsupportedEventTypeError creates an unsupported event type error
func UnsupportedEventTypeError(eventType string) error {
	return NewEventError("parse", eventType, ErrUnsupportedEventType, "")
}

// ParsingError creates a parsing-related error
func ParsingError(eventType string, err error) error {
	return NewEventError("parse", eventType, fmt.Errorf("parsing: %w", err), "")
}

// ValidationError creates a validation-related error
func ValidationError(eventType string, err error, context string) error {
	return NewEventError("validate", eventType, fmt.Errorf("validation: %w", err), context)
}
