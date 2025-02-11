package api

import (
	"context"
	"math/rand"
	"net/http"
	
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
			"avatar_url":   "https://res.cloudinary.com/cvp/image/upload/v1738543861/6498a96ff70d54a01c2138bea8270363_kooii9.png",
			"addresses": map[string]any{
				"receiver_name":         "Nguyễn Văn Tuấn",
				"receiver_phone_number": "0394231235",
				"province_name":         "Hồ Chí Minh",
				"district_name":         "Quận Tân Bình",
				"ward_name":             "Phường 2",
				"detail":                "18 Trường Sơn",
			},
		},
		{
			"full_name":    "Mecha Prime",
			"email":        "seller2@gmail.com",
			"phone_number": "0394263125",
			"role":         db.UserRoleSeller,
			"avatar_url":   "https://res.cloudinary.com/cvp/image/upload/v1738543918/f19fcef473da0837d575d5bf02564a62_thp5l3.png",
			"addresses": map[string]any{
				"receiver_name":         "Trần Thị Thanh",
				"receiver_phone_number": "0394263125",
				"province_name":         "Hồ Chí Minh",
				"district_name":         "Quận 10",
				"ward_name":             "Phường 12",
				"detail":                "11 Sư Vạn Hạnh",
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
			FullName: pgtype.Text{
				String: seller["full_name"].(string),
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
			"description":  "Gundam EG LAH là mô hình Entry Grade, tỉ lệ 1/144, thuộc dòng sản phẩm Gundam mới của Bandai, với thiết kế đơn giản nhưng chi tiết, dễ lắp ráp, phù hợp cho người mới bắt đầu. Mô hình này mang đến một bản sao của Mobile Suit với các khớp cơ bản, màu sắc tươi sáng và chi tiết vừa phải.",
			"price":        int64(230000),
			"images": []map[string]any{
				{
					"url":        "https://res.cloudinary.com/cvp/image/upload/v1738061923/pic-1_e4qvxk.jpg",
					"is_primary": true,
				},
				{
					"url":        "https://res.cloudinary.com/cvp/image/upload/v1738061923/pic-2_zgzpbv.webp",
					"is_primary": false,
				},
				{
					"url":        "https://res.cloudinary.com/cvp/image/upload/v1738061924/pic-3_xn9qbb.webp",
					"is_primary": false,
				},
				{
					"url":        "https://res.cloudinary.com/cvp/image/upload/v1738061924/pic-4_txfro7.webp",
					"is_primary": false,
				},
				{
					"url":        "https://res.cloudinary.com/cvp/image/upload/v1738061925/pic-5_ohfx7d.webp",
					"is_primary": false,
				},
			},
		},
		{
			"name":         "Gundam EG RX-93FF NU - Fukuoka Limited",
			"grade_id":     grades[0].ID,
			"condition":    db.GundamConditionGood,
			"manufacturer": "Bandai",
			"scale":        db.GundamScale1144,
			"description":  "Gundam EG RX-93FF Nu - Fukuoka Limited là phiên bản đặc biệt của mô hình Entry Grade tỉ lệ 1/144, được phát hành giới hạn tại Fukuoka, Nhật Bản. Mô hình này tái hiện RX-93FF Nu Gundam với các chi tiết sắc nét, dễ lắp ráp và màu sắc nổi bật, đồng thời mang đến một bản sao giản đơn nhưng tinh tế của Mobile Suit trong Gundam: Char's Counterattack.",
			"price":        int64(140000),
			"images": []map[string]any{
				{
					"url":        "https://res.cloudinary.com/cvp/image/upload/v1738061943/pic-1_f2fy26.jpg",
					"is_primary": true,
				},
				{
					"url":        "https://res.cloudinary.com/cvp/image/upload/v1738061944/pic-2_jj1bpw.webp",
					"is_primary": false,
				},
				{
					"url":        "https://res.cloudinary.com/cvp/image/upload/v1738061945/pic-3_zrq07p.webp",
					"is_primary": false,
				},
				{
					"url":        "https://res.cloudinary.com/cvp/image/upload/v1738061945/pic-4_wxsvm8.webp",
					"is_primary": false,
				},
				{
					"url":        "https://res.cloudinary.com/cvp/image/upload/v1738061946/pic-5_veoe4c.webp",
					"is_primary": false,
				},
			},
		},
		
		// High Grade
		{
			"name":         "Gundam HG GN-005 Virtue Bandai",
			"grade_id":     grades[1].ID,
			"condition":    db.GundamConditionGood,
			"manufacturer": "Bandai",
			"scale":        db.GundamScale1144,
			"description":  "Gundam HG GN-005 Virtue của Bandai là mô hình High Grade tỉ lệ 1/144, tái hiện Mobile Suit Virtue từ Gundam 00, nổi bật với thiết kế mạnh mẽ, bộ giáp dày đặc và các chi tiết mô phỏng chính xác. Mô hình này đi kèm với vũ khí như GN Bazooka và khả năng tạo dáng đơn giản nhưng vẫn giữ được sự ấn tượng.",
			"price":        int64(510000),
			"images": []map[string]any{
				{
					"url":        "https://res.cloudinary.com/cvp/image/upload/v1738062079/pic-1_t73xgp.webp",
					"is_primary": true,
				},
				{
					"url":        "https://res.cloudinary.com/cvp/image/upload/v1738062079/pic-2_wgh7oe.webp",
					"is_primary": false,
				},
				{
					"url":        "https://res.cloudinary.com/cvp/image/upload/v1738062080/pic-3_aekyoj.webp",
					"is_primary": false,
				},
				{
					"url":        "https://res.cloudinary.com/cvp/image/upload/v1738062081/pic-4_tqunng.webp",
					"is_primary": false,
				},
				{
					"url":        "https://res.cloudinary.com/cvp/image/upload/v1738062082/pic-5_meexfz.webp",
					"is_primary": false,
				},
			},
		},
		{
			"name":         "Cherudim Gundam Saga Type.GBF",
			"grade_id":     grades[1].ID,
			"condition":    db.GundamConditionMint,
			"manufacturer": "Bandai",
			"scale":        db.GundamScale1144,
			"description":  "Phiên bản màu đen vàng đặc biệt.",
			"price":        int64(610000),
			"images": []map[string]any{
				{
					"url":        "https://res.cloudinary.com/cvp/image/upload/v1738062099/pic-1_qudovv.webp",
					"is_primary": true,
				},
				{
					"url":        "https://res.cloudinary.com/cvp/image/upload/v1738062100/pic-2_y1d61i.webp",
					"is_primary": false,
				},
				{
					"url":        "https://res.cloudinary.com/cvp/image/upload/v1738062101/pic-3_avxia5.webp",
					"is_primary": false,
				},
				{
					"url":        "https://res.cloudinary.com/cvp/image/upload/v1738062102/pic-4_rj4zlq.webp",
					"is_primary": false,
				},
				{
					"url":        "https://res.cloudinary.com/cvp/image/upload/v1738062103/pic-5_krvmgi.webp",
					"is_primary": false,
				},
			},
		},
	}
	for _, gundam := range gundams {
		randomOwnerID := sellers[rand.Intn(len(sellers))].ID
		currentGundam, err := server.dbStore.CreateGundam(context.Background(), db.CreateGundamParams{
			OwnerID:      randomOwnerID,
			Name:         gundam["name"].(string),
			Slug:         util.GenerateRandomSlug(gundam["name"].(string)),
			GradeID:      gundam["grade_id"].(int64),
			Condition:    gundam["condition"].(db.GundamCondition),
			Manufacturer: gundam["manufacturer"].(string),
			Scale:        gundam["scale"].(db.GundamScale),
			Description:  gundam["description"].(string),
			Price:        gundam["price"].(int64),
			Status:       db.GundamStatusAvailable,
		})
		if err != nil {
			log.Error().Err(err).Msg("Failed to create gundam")
			ctx.JSON(500, gin.H{"error": "Failed to create gundams"})
			return
		}
		
		// Create images
		for _, image := range gundam["images"].([]map[string]any) {
			err = server.dbStore.CreateGundamImage(context.Background(), db.CreateGundamImageParams{
				GundamID:  currentGundam.ID,
				Url:       image["url"].(string),
				IsPrimary: image["is_primary"].(bool),
			})
			if err != nil {
				log.Error().Err(err).Msg("Failed to create gundam image")
				ctx.JSON(500, gin.H{"error": "Failed to create gundam images"})
				return
			}
		}
	}
	
	ctx.String(http.StatusOK, "Seed data successfully")
}
