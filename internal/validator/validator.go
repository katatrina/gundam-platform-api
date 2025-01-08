package validator

import (
	"errors"
	"fmt"
	"net/mail"
	"regexp"
)

var (
	isValidUsername = regexp.MustCompile(`^[a-zA-Z0-9_]+$`).MatchString
	isValidFullName = regexp.MustCompile(`^[a-zA-Z\s]+$`).MatchString
)

func ValidateString(value string, minLength int, maxLength int) error {
	n := len(value)
	if n < minLength {
		return fmt.Errorf("must contain from %d to %d characters", minLength, maxLength)
	}
	
	return nil
}

func ValidateUsername(value string) error {
	if err := ValidateString(value, 3, 25); err != nil {
		return err
	}
	
	if !isValidUsername(value) {
		return fmt.Errorf("must contain only letters, numbers, or underscores")
	}
	
	return nil
}

func ValidatePassword(value string) (err error) {
	// Define a general value rule that covers all conditions
	err = errors.New("value must be between 8 and 30 characters long, contain at least one digit, one lowercase letter, one uppercase letter, and one special character")
	
	// Check if value is between 8 and 30 characters
	if len(value) < 8 || len(value) > 30 {
		return
	}
	
	// Check if value contains at least one digit
	if !regexp.MustCompile(`[0-9]`).MatchString(value) {
		return
	}
	
	// Check if value contains at least one lowercase letter
	if !regexp.MustCompile(`[a-z]`).MatchString(value) {
		return
	}
	
	// Check if value contains at least one uppercase letter
	if !regexp.MustCompile(`[A-Z]`).MatchString(value) {
		return
	}
	
	// Check if value contains at least one special character
	if !regexp.MustCompile(`[\W_]`).MatchString(value) {
		return
	}
	
	// If all checks pass, return nil indicating no error
	return nil
}

func ValidateEmail(value string) error {
	if err := ValidateString(value, 6, 200); err != nil {
		return err
	}
	
	if _, err := mail.ParseAddress(value); err != nil {
		return fmt.Errorf("is not a valid email address")
	}
	
	return nil
}

func ValidateFullName(value string) error {
	if err := ValidateString(value, 3, 100); err != nil {
		return err
	}
	
	if !isValidFullName(value) {
		return fmt.Errorf("must contain only letters or spaces")
	}
	
	return nil
}

func ValidateEmailID(value int64) error {
	if value <= 0 {
		return fmt.Errorf("must be a positive integer")
	}
	
	return nil
}

func ValidateSecretCode(value string) error {
	return ValidateString(value, 32, 128)
}
