package util

import (
	"fmt"
	
	"github.com/lithammer/shortuuid/v4"
)

const (
	alphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
)

var (
	validOrderStatuses = []string{"pending", "packaging", "delivering", "delivered", "completed", "canceled", "failed"}
)

// GenerateOrderCode generates a unique order code in the format "ORD-XXXXXXXXXX".
func GenerateOrderCode() string {
	// Tạo phần ngẫu nhiên
	uuid := shortuuid.NewWithAlphabet(alphabet)
	
	// Kết hợp: ORD-XXXXXXXX
	return fmt.Sprintf("ORD-%s", uuid[:10])
}

func IsOrderStatusValid(status string) error {
	for _, validStatus := range validOrderStatuses {
		if status == validStatus {
			return nil
		}
	}
	
	return fmt.Errorf("invalid order status: %s, must be one of %v", status, validOrderStatuses)
}
