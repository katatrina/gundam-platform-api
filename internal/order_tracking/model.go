package ordertracking

import (
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
)

// Định nghĩa map ánh xạ từ trạng thái GHN sang trạng thái tổng quát
var statusGroups = map[string]db.DeliveryOverralStatus{
	// Nhóm trạng thái picking (Đang lấy hàng)
	"ready_to_pick":         db.DeliveryOverralStatusPicking, // Đơn hàng mới đang chờ lấy hàng ✅
	"picking":               db.DeliveryOverralStatusPicking, // Shipper đang đi lấy hàng
	"money_collect_picking": db.DeliveryOverralStatusPicking, // Đang thu tiền khi lấy hàng
	"picked":                db.DeliveryOverralStatusPicking, // Đã lấy hàng xong
	
	// Nhóm trạng thái delivering (Đang giao hàng)
	"storing":                  db.DeliveryOverralStatusDelivering, // Đang lưu kho
	"transporting":             db.DeliveryOverralStatusDelivering, // Đang vận chuyển
	"sorting":                  db.DeliveryOverralStatusDelivering, // Đang phân loại
	"delivering":               db.DeliveryOverralStatusDelivering, // Đang giao hàng ✅
	"money_collect_delivering": db.DeliveryOverralStatusDelivering, // Đang thu tiền khi giao hàng
	
	// Nhóm trạng thái delivered (Đã giao hàng thành công)
	"delivered": db.DeliveryOverralStatusDelivered, // Đã giao hàng thành công ✅
	
	// Nhóm trạng thái failed (Giao hàng thất bại)
	"delivery_fail": db.DeliveryOverralStatusFailed, // Giao hàng thất bại ✅
	"cancel":        db.DeliveryOverralStatusFailed, // Đơn hàng bị hủy
	"exception":     db.DeliveryOverralStatusFailed, // Gặp vấn đề bất thường
	"damage":        db.DeliveryOverralStatusFailed, // Hàng bị hư hỏng
	"lost":          db.DeliveryOverralStatusFailed, // Hàng bị mất
	
	// Nhóm trạng thái return (Trả hàng) - Chỉ dành cho đơn thông thường và đơn đấu giá
	"waiting_to_return":   db.DeliveryOverralStatusReturn, // Đang chờ trả hàng
	"return":              db.DeliveryOverralStatusReturn, // Trả hàng
	"return_transporting": db.DeliveryOverralStatusReturn, // Đang vận chuyển trả hàng
	"return_sorting":      db.DeliveryOverralStatusReturn, // Đang phân loại trả hàng
	"returning":           db.DeliveryOverralStatusReturn, // Đang trả hàng
	"return_fail":         db.DeliveryOverralStatusReturn, // Trả hàng thất bại
	"returned":            db.DeliveryOverralStatusReturn, // Đã trả hàng
}

// Hàm chuyển đổi trạng thái GHN sang trạng thái tổng quát
func mapGHNStatusToOverallStatus(ghnStatus string) db.DeliveryOverralStatus {
	if overallStatus, exists := statusGroups[ghnStatus]; exists {
		return overallStatus
	}
	return db.DeliveryOverralStatusFailed // Mặc định xem như thất bại nếu không khớp với bất kỳ trạng thái nào
}
