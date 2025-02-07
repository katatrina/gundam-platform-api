package util

import (
	"fmt"
	
	"github.com/gosimple/slug"
	"github.com/lithammer/shortuuid/v4"
)

func GenerateRandomSlug(name string) string {
	baseSlug := slug.Make(name)
	shortID := shortuuid.New()[:8] // Lấy 8 ký tự đầu
	
	return fmt.Sprintf("%s-%s", baseSlug, shortID)
}
