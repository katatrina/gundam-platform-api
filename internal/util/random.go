package util

import (
	"fmt"
	"math/rand"
	"time"
	
	"github.com/gosimple/slug"
	"github.com/lithammer/shortuuid/v4"
)

func GenerateRandomSlug(name string) string {
	baseSlug := slug.Make(name)
	shortID := shortuuid.New()[:8] // Lấy 8 ký tự đầu
	
	return fmt.Sprintf("%s-%s", baseSlug, shortID)
}

func GenerateZalopayAppTransID() string {
	// Đảm bảo sử dụng múi giờ Việt Nam (GMT+7)
	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")
	now := time.Now().In(loc)
	
	// Format yymmdd theo yêu cầu
	datePrefix := now.Format("060102")
	
	// Tạo mã đơn hàng unique (sử dụng timestamp nano và random string)
	orderID := fmt.Sprintf("%d%s", now.UnixNano()%100000,
		randomString(5))
	
	// Kết hợp theo format yymmdd_orderID
	return fmt.Sprintf("%s_%s", datePrefix, orderID)
}

// Hàm hỗ trợ tạo chuỗi ngẫu nhiên
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
