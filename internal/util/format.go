package util

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	
	"github.com/dustin/go-humanize"
)

// FormatVND chuyển đổi số tiền từ int64 sang chuỗi định dạng VND.
// Ví dụ: 1000000 -> "1.000.000₫".
func FormatVND(amount int64) string {
	// Định dạng số với dấu phẩy phân cách hàng nghìn
	formatted := fmt.Sprintf("%d", amount)
	
	// Thay thế dấu phẩy bằng dấu chấm để phù hợp với định dạng VND
	length := len(formatted)
	if length <= 3 {
		return formatted + "₫"
	}
	
	var result strings.Builder
	for i, char := range formatted {
		result.WriteRune(char)
		if (length-i-1)%3 == 0 && i < length-1 {
			result.WriteRune('.')
		}
	}
	
	// Thêm ký hiệu tiền tệ VND
	result.WriteString("₫")
	
	return result.String()
}

// FormatMoney định dạng số tiền với dấu chấm phân cách hàng nghìn
// VD: 1000000 -> "1.000.000"
func FormatMoney(amount int64) string {
	// Sử dụng humanize.Comma để định dạng với dấu phẩy
	formatted := humanize.Comma(amount)
	
	// Thay thế dấu phẩy bằng dấu chấm
	formatted = strings.ReplaceAll(formatted, ",", ".")
	
	return formatted
}

// Hàm helper để rút gọn tiêu đề
func TruncateString(title string, maxLength int) string {
	if len(title) <= maxLength {
		return title
	}
	return title[:maxLength] + "..."
}

func ExtractPublicIDFromURL(url string) (string, error) {
	// Kiểm tra URL hợp lệ
	if !strings.Contains(url, "cloudinary.com") {
		return "", fmt.Errorf("not a valid Cloudinary URL")
	}
	
	// Tìm vị trí của phần version (vXXXXXXXXX)
	parts := strings.Split(url, "/")
	var startIdx int
	for i, part := range parts {
		if strings.HasPrefix(part, "v") && len(part) > 1 {
			// Kiểm tra nếu phần còn lại toàn là số
			if _, err := strconv.Atoi(part[1:]); err == nil {
				startIdx = i + 1
				break
			}
		}
	}
	
	if startIdx == 0 || startIdx >= len(parts) {
		return "", fmt.Errorf("cannot find version in URL")
	}
	
	// Ghép tất cả các phần sau version, bỏ phần mở rộng file
	result := strings.Join(parts[startIdx:], "/")
	// Loại bỏ phần mở rộng file (.jpg, .png, v.v)
	result = strings.TrimSuffix(result, filepath.Ext(result))
	
	return result, nil
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
