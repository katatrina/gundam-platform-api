package validator

import (
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"unicode"
)

func ValidateString(value string, minLength int, maxLength int) error {
	n := len(value)
	if n < minLength {
		return fmt.Errorf("must contain from %d to %d characters", minLength, maxLength)
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
	
	for _, r := range value {
		if !unicode.IsLetter(r) && !unicode.IsSpace(r) {
			return fmt.Errorf("must contain only letters or spaces")
		}
	}
	
	return nil
}
