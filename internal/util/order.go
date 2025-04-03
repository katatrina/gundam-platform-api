package util

import (
	"fmt"
	
	"github.com/lithammer/shortuuid/v4"
)

const (
	alphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
)

// GenerateOrderCode generates a unique order code in the format "ORD-XXXXXXXXXX".
func GenerateOrderCode() string {
	// Tạo phần ngẫu nhiên
	uuid := shortuuid.NewWithAlphabet(alphabet)
	
	// Kết hợp: ORD-XXXXXXXX
	return fmt.Sprintf("ORD-%s", uuid[:10])
}
