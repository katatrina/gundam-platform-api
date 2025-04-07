package ordertracking

import (
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
)

// Định nghĩa map ánh xạ từ trạng thái GHN sang trạng thái tổng quát
var statusGroups = map[string]db.DeliveryOverralStatus{
	// Nhóm trạng thái picking
	"ready_to_pick":         db.DeliveryOverralStatusPicking,
	"picking":               db.DeliveryOverralStatusPicking,
	"money_collect_picking": db.DeliveryOverralStatusPicking,
	"picked":                db.DeliveryOverralStatusPicking,
	
	// Nhóm trạng thái delivering
	"storing":                  db.DeliveryOverralStatusDelivering,
	"transporting":             db.DeliveryOverralStatusDelivering,
	"sorting":                  db.DeliveryOverralStatusDelivering,
	"delivering":               db.DeliveryOverralStatusDelivering,
	"money_collect_delivering": db.DeliveryOverralStatusDelivering,
	
	// Nhóm trạng thái delivered
	"delivered": db.DeliveryOverralStatusDelivered,
	
	// Nhóm trạng thái failed
	"delivery_fail": db.DeliveryOverralStatusFailed,
	"cancel":        db.DeliveryOverralStatusFailed,
	"exception":     db.DeliveryOverralStatusFailed,
	"damage":        db.DeliveryOverralStatusFailed,
	"lost":          db.DeliveryOverralStatusFailed,
	
	// Nhóm trạng thái return
	"waiting_to_return":   db.DeliveryOverralStatusReturn,
	"return":              db.DeliveryOverralStatusReturn,
	"return_transporting": db.DeliveryOverralStatusReturn,
	"return_sorting":      db.DeliveryOverralStatusReturn,
	"returning":           db.DeliveryOverralStatusReturn,
	"return_fail":         db.DeliveryOverralStatusReturn,
	"returned":            db.DeliveryOverralStatusReturn,
}

// Hàm chuyển đổi trạng thái GHN sang trạng thái tổng quát
func mapGHNStatusToOverallStatus(ghnStatus string) db.DeliveryOverralStatus {
	if overallStatus, exists := statusGroups[ghnStatus]; exists {
		return overallStatus
	}
	return db.DeliveryOverralStatusFailed // Mặc định xem như thất bại nếu không khớp với bất kỳ trạng thái nào
}
