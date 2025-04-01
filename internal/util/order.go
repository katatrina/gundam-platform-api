package util

import (
	"fmt"
	"time"
	
	"github.com/lithammer/shortuuid/v4"
)

const (
	alphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
)

func GenerateOrderCode() string {
	// Thêm thành phần thời gian (YYMMDD)
	timeComponent := time.Now().Format("060102")
	
	// Tạo phần ngẫu nhiên
	uuid := shortuuid.NewWithAlphabet(alphabet)
	
	// Kết hợp: ORD-YYMMDD-XXXXX
	return fmt.Sprintf("ORD-%s-%s", timeComponent, uuid[:5])
}
