package util

import (
	"fmt"
	"strings"
	"time"
)

// FormatVND chuyển đổi số tiền từ int64 sang chuỗi định dạng VND.
// Ví dụ: 1000000 -> "1.000.000 ₫".
func FormatVND(amount int64) string {
	// Định dạng số với dấu phẩy phân cách hàng nghìn
	formatted := fmt.Sprintf("%d", amount)
	
	// Thay thế dấu phẩy bằng dấu chấm để phù hợp với định dạng VND
	length := len(formatted)
	if length <= 3 {
		return formatted + " ₫"
	}
	
	var result strings.Builder
	for i, char := range formatted {
		result.WriteRune(char)
		if (length-i-1)%3 == 0 && i < length-1 {
			result.WriteRune('.')
		}
	}
	
	// Thêm ký hiệu tiền tệ VND
	result.WriteString(" ₫")
	
	return result.String()
}

// Hàm helper để rút gọn tiêu đề
func TruncateContent(title string, maxLength int) string {
	if len(title) <= maxLength {
		return title
	}
	return title[:maxLength] + "..."
}

func BoolPointer(b bool) *bool {
	return &b
}

func StringPointer(s string) *string {
	return &s
}

func Int64Pointer(i int64) *int64 {
	return &i
}

func TimePointer(t time.Time) *time.Time {
	return &t
}
