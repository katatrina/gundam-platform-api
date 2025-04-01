package util

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"time"
	
	"github.com/gosimple/slug"
	"github.com/lithammer/shortuuid/v4"
)

var encoding = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

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
	
	// Tạo 10 byte ngẫu nhiên (80 bit)
	randomBytes := make([]byte, 10)
	_, err := rand.Read(randomBytes)
	if err != nil {
		panic(err) // Trong thực tế, nên xử lý lỗi một cách phù hợp hơn
	}
	
	// Mã hóa thành chuỗi base32
	randomString := encoding.EncodeToString(randomBytes)
	
	// Kết hợp theo format yymmdd_orderID, giới hạn độ dài tổng cộng là 40 ký tự
	appTransID := fmt.Sprintf("%s_%s", datePrefix, randomString)
	if len(appTransID) > 40 {
		appTransID = appTransID[:40]
	}
	
	return appTransID
}
