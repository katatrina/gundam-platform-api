package util

import (
	"strings"
)

// IsValidVietnamesePhoneNumber kiểm tra định dạng số điện thoại Việt Nam
func IsValidVietnamesePhoneNumber(phone string) bool {
	// Loại bỏ các ký tự không phải số
	phone = strings.TrimSpace(phone)
	phone = strings.ReplaceAll(phone, "-", "")
	phone = strings.ReplaceAll(phone, " ", "")
	
	// Kiểm tra độ dài (10-11 số)
	if len(phone) < 10 || len(phone) > 11 {
		return false
	}
	
	// Kiểm tra prefix
	validPrefixes := []string{"03", "05", "07", "08", "09", "84"}
	hasValidPrefix := false
	for _, prefix := range validPrefixes {
		if strings.HasPrefix(phone, prefix) {
			hasValidPrefix = true
			break
		}
	}
	if !hasValidPrefix {
		return false
	}
	
	// Kiểm tra tất cả ký tự là số
	for _, c := range phone {
		if c < '0' || c > '9' {
			return false
		}
	}
	
	return true
}
