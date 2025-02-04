package api

import (
	"context"
	"math/rand"
	
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/rs/zerolog/log"
)

func (server *Server) seedData(ctx *gin.Context) {
	// Create seller accounts:
	sellerInfos := []map[string]any{
		{
			"full_name":    "Gundam Warzone",
			"email":        "seller1@gmail.com",
			"phone_number": "0394231235",
			"role":         db.UserRoleSeller,
			"avatar_url":   "https://res.cloudinary.com/cvp/image/upload/v1738543918/f19fcef473da0837d575d5bf02564a62_thp5l3.png",
			"addresses": map[string]any{
				"receiver_name":         "Nguyễn Văn Tuấn",
				"receiver_phone_number": "0394231235",
				"province_name":         "Hồ Chí Minh",
				"district_name":         "Quận 1",
				"ward_name":             "Phường Bến Nghé",
				"detail":                "123 Nguyễn Huệ",
			},
		},
		{
			"full_name":    "Mecha Prime",
			"email":        "seller2@gmail.com",
			"phone_number": "0394263125",
			"role":         db.UserRoleSeller,
			"avatar_url":   "https://res.cloudinary.com/cvp/image/upload/v1738543861/6498a96ff70d54a01c2138bea8270363_kooii9.png",
			"addresses": map[string]any{
				"receiver_name":         "Trần Thị Thanh",
				"receiver_phone_number": "0394263125",
				"province_name":         "Hồ Chí Minh",
				"district_name":         "Quận 3",
				"ward_name":             "Phường 6",
				"detail":                "123 Cách Mạng Tháng 8",
			},
		},
	}
	
	hashedPassword, _ := util.HashPassword("12345")
	var sellers []db.User
	for _, seller := range sellerInfos {
		user, err := server.dbStore.CreateUser(context.Background(), db.CreateUserParams{
			HashedPassword: pgtype.Text{
				String: hashedPassword,
				Valid:  true,
			},
			Email:         seller["email"].(string),
			EmailVerified: true,
			PhoneNumber: pgtype.Text{
				String: seller["phone_number"].(string),
				Valid:  true,
			},
			PhoneNumberVerified: true,
			Role:                db.UserRoleSeller,
			AvatarUrl: pgtype.Text{
				String: seller["avatar_url"].(string),
				Valid:  true,
			},
		})
		if err != nil {
			log.Error().Err(err).Msg("Failed to create user")
			ctx.JSON(500, gin.H{"error": "Failed to create seller accounts"})
			return
		}
		
		sellers = append(sellers, user)
		
		err = server.dbStore.CreateUserAddress(context.Background(), db.CreateUserAddressParams{
			UserID:              user.ID,
			ReceiverName:        seller["full_name"].(string),
			ReceiverPhoneNumber: seller["phone_number"].(string),
			ProvinceName:        seller["addresses"].(map[string]any)["province_name"].(string),
			DistrictName:        seller["addresses"].(map[string]any)["district_name"].(string),
			WardName:            seller["addresses"].(map[string]any)["ward_name"].(string),
			Detail:              seller["addresses"].(map[string]any)["detail"].(string),
			IsPrimary:           true,
			IsPickupAddress:     true,
		})
		if err != nil {
			log.Error().Err(err).Msg("Failed to create address")
			ctx.JSON(500, gin.H{"error": "Failed to create seller addresses"})
			return
		}
	}
	
	// Create gundams:
	grades, err := server.dbStore.ListGundamGrades(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("Failed to list categories")
		ctx.JSON(500, gin.H{"error": "Failed to list categories"})
		return
	}
	
	gundams := []map[string]any{
		// Entry Grade
		{
			"name":         "Gundam EG LAH",
			"grade_id":     grades[0].ID,
			"condition":    db.GundamConditionGood,
			"manufacturer": "Bandai",
			"scale":        db.GundamScale1144,
			"description":  "",
			"price":        450000,
			"images": []map[string]any{
				{
					"url":        "https://res.cloudinary.com/cvp/image/upload/v1738543918/f19fcef473da0837d575d5bf02564a62_thp5l3.png",
					"is_primary": true,
				},
			},
		},
		{
			"name":         "Gundam EG RX-93FF NU - Fukuoka Limited",
			"grade_id":     grades[0].ID,
			"condition":    db.GundamConditionNearmint,
			"manufacturer": "Bandai",
			"scale":        db.GundamScale1144,
			"description":  "",
			"price":        850000,
		},
		
		// High Grade
		{
			"name":         "Act Zaku (Kycilia's Forces)",
			"grade_id":     grades[1].ID,
			"condition":    db.GundamConditionGood,
			"manufacturer": "Bandai",
			"scale":        db.GundamScale1144,
			"description":  "Độ chi tiết vừa phải, khớp chuyển động tương đối linh hoạt. Ráp theo kiểu bấm khớp, không cần dùng keo dán.",
			"price":        220000,
			"images":       "",
		},
		{
			"name":         "Cherudim Gundam Saga Type.GBF",
			"grade_id":     grades[1].ID,
			"condition":    db.GundamConditionMint,
			"manufacturer": "Bandai",
			"scale":        db.GundamScale1144,
			"description":  "Phiên bản màu đen vàng đặc biệt.",
			"price":        610000,
		},
	}
	for _, gundam := range gundams {
		randomOwnerID := sellers[rand.Intn(len(sellers))].ID
		_, err := server.dbStore.CreateGundam(context.Background(), db.CreateGundamParams{
			OwnerID:      randomOwnerID,
			Name:         gundam["name"].(string),
			GradeID:      gundam["grade_id"].(int64),
			Condition:    db.GundamCondition(gundam["condition"].(string)),
			Manufacturer: "Bandai",
			Scale:        db.GundamScale(gundam["scale"].(string)),
			Description:  gundam["description"].(string),
			Price:        gundam["price"].(int64),
			Status:       db.GundamStatusAvailable,
		})
		if err != nil {
			log.Error().Err(err).Msg("Failed to create gundam")
			ctx.JSON(500, gin.H{"error": "Failed to create gundams"})
			return
		}
		
	}
}
