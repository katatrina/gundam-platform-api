package db

import (
	"context"
)

// Helper function để lấy thông tin chi tiết Gundam
func (store *SQLStore) GetGundamDetailsByID(ctx context.Context, q *Queries, gundamID int64) (GundamDetails, error) {
	var qTx *Queries
	if q == nil {
		qTx = store.Queries
	} else {
		qTx = q
	}
	
	var detail GundamDetails
	
	gundam, err := qTx.GetGundamByID(ctx, gundamID)
	if err != nil {
		return detail, err
	}
	
	grade, err := qTx.GetGradeByID(ctx, gundam.GradeID)
	if err != nil {
		return detail, err
	}
	
	primaryImageURL, err := qTx.GetGundamPrimaryImageURL(ctx, gundam.ID)
	if err != nil {
		return detail, err
	}
	
	secondaryImageURLs, err := qTx.GetGundamSecondaryImageURLs(ctx, gundam.ID)
	if err != nil {
		return detail, err
	}
	
	accessories, err := qTx.GetGundamAccessories(ctx, gundam.ID)
	if err != nil {
		return detail, err
	}
	
	accessoryDTOs := make([]GundamAccessoryDTO, len(accessories))
	for i, accessory := range accessories {
		accessoryDTOs[i] = ConvertGundamAccessoryToDTO(accessory)
	}
	
	detail = GundamDetails{
		ID:                   gundam.ID,
		OwnerID:              gundam.OwnerID,
		Name:                 gundam.Name,
		Slug:                 gundam.Slug,
		Grade:                grade.DisplayName,
		Series:               gundam.Series,
		PartsTotal:           gundam.PartsTotal,
		Material:             gundam.Material,
		Version:              gundam.Version,
		Quantity:             gundam.Quantity,
		Condition:            string(gundam.Condition),
		ConditionDescription: gundam.ConditionDescription,
		Manufacturer:         gundam.Manufacturer,
		Weight:               gundam.Weight,
		Scale:                string(gundam.Scale),
		Description:          gundam.Description,
		Price:                gundam.Price,
		ReleaseYear:          gundam.ReleaseYear,
		Status:               string(gundam.Status),
		Accessories:          accessoryDTOs,
		PrimaryImageURL:      primaryImageURL,
		SecondaryImageURLs:   secondaryImageURLs,
		CreatedAt:            gundam.CreatedAt,
		UpdatedAt:            gundam.UpdatedAt,
	}
	
	return detail, nil
}

// Hàm hỗ trợ để xác định trạng thái thấp nhất
func GetLowestOrderStatus(status1, status2 OrderStatus) OrderStatus {
	// Định nghĩa thứ tự các trạng thái từ thấp đến cao
	statusOrder := map[OrderStatus]int{
		OrderStatusPending:    1,
		OrderStatusPackaging:  2,
		OrderStatusDelivering: 3,
		OrderStatusDelivered:  4,
		OrderStatusCompleted:  5,
		OrderStatusFailed:     0, // Các trạng thái đặc biệt
		OrderStatusCanceled:   0,
	}
	
	// Trường hợp đặc biệt: nếu một trong hai là failed hoặc canceled
	if status1 == OrderStatusFailed || status1 == OrderStatusCanceled {
		return status1
	}
	if status2 == OrderStatusFailed || status2 == OrderStatusCanceled {
		return status2
	}
	
	// So sánh thông thường
	if statusOrder[status1] <= statusOrder[status2] {
		return status1
	}
	return status2
}
