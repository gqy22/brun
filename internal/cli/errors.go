package cli

import (
	"errors"
	"fmt"
)

type CLIError struct {
	Code    string
	Message string
	Hint    string
	Err     error
}

func (e *CLIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Code
}

func (e *CLIError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func cliError(code, message, hint string, err error) error {
	return &CLIError{Code: code, Message: message, Hint: hint, Err: err}
}

func formatCLIError(err error) string {
	var cliErr *CLIError
	if errors.As(err, &cliErr) {
		msg := fmt.Sprintf("Error: %s\nCode: %s", cliErr.Error(), cliErr.Code)
		if cliErr.Hint != "" {
			msg += "\nHint: " + cliErr.Hint
		}
		return msg + "\n"
	}
	return fmt.Sprintf("Error: %s\n", err)
}
