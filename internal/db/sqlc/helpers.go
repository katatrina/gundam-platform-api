package db

import (
	"context"
	"fmt"
	
	"github.com/katatrina/gundam-BE/internal/delivery"
)

// Helper function để tạo delivery information
func createDeliveryInfo(qTx *Queries, ctx context.Context, arg CreateOrderTxParams) (buyerDelivery, sellerDelivery DeliveryInformation, err error) {
	buyerDelivery, err = qTx.CreateDeliveryInformation(ctx, CreateDeliveryInformationParams{
		UserID:        arg.BuyerID,
		FullName:      arg.BuyerAddress.FullName,
		PhoneNumber:   arg.BuyerAddress.PhoneNumber,
		ProvinceName:  arg.BuyerAddress.ProvinceName,
		DistrictName:  arg.BuyerAddress.DistrictName,
		GhnDistrictID: arg.BuyerAddress.GhnDistrictID,
		WardName:      arg.BuyerAddress.WardName,
		GhnWardCode:   arg.BuyerAddress.GhnWardCode,
		Detail:        arg.BuyerAddress.Detail,
	})
	if err != nil {
		return
	}
	
	sellerDelivery, err = qTx.CreateDeliveryInformation(ctx, CreateDeliveryInformationParams{
		UserID:        arg.SellerID,
		FullName:      arg.SellerAddress.FullName,
		PhoneNumber:   arg.SellerAddress.PhoneNumber,
		ProvinceName:  arg.SellerAddress.ProvinceName,
		DistrictName:  arg.SellerAddress.DistrictName,
		GhnDistrictID: arg.SellerAddress.GhnDistrictID,
		WardName:      arg.SellerAddress.WardName,
		GhnWardCode:   arg.SellerAddress.GhnWardCode,
		Detail:        arg.SellerAddress.Detail,
	})
	return
}

func ConvertToDeliveryCreateOrderRequest(order Order, orderItems []OrderItem, senderAddress, receiverAddress DeliveryInformation) delivery.CreateOrderRequest {
	ghnOrder := delivery.OrderInfo{
		ID:            order.ID.String(),
		Code:          order.Code,
		BuyerID:       order.BuyerID,
		SellerID:      order.SellerID,
		ItemsSubtotal: order.ItemsSubtotal,
		DeliveryFee:   order.DeliveryFee,
		TotalAmount:   order.TotalAmount,
		Status:        string(order.Status),
		PaymentMethod: string(order.PaymentMethod),
	}
	if order.Note != nil {
		ghnOrder.Note = *order.Note
	}
	
	ghnOrderItems := make([]delivery.OrderItemInfo, len(orderItems))
	for i, item := range orderItems {
		ghnOrderItems[i] = delivery.OrderItemInfo{
			OrderID:  item.OrderID.String(),
			Name:     item.Name,
			Price:    item.Price,
			Quantity: item.Quantity,
			Weight:   item.Weight,
		}
	}
	
	ghnSenderAddress := delivery.AddressInfo{
		UserID:        senderAddress.UserID,
		FullName:      senderAddress.FullName,
		PhoneNumber:   senderAddress.PhoneNumber,
		ProvinceName:  senderAddress.ProvinceName,
		DistrictName:  senderAddress.DistrictName,
		GhnDistrictID: senderAddress.GhnDistrictID,
		WardName:      senderAddress.WardName,
		GhnWardCode:   senderAddress.GhnWardCode,
		Detail:        senderAddress.Detail,
	}
	
	ghnReceiverAddress := delivery.AddressInfo{
		UserID:        receiverAddress.UserID,
		FullName:      receiverAddress.FullName,
		PhoneNumber:   receiverAddress.PhoneNumber,
		ProvinceName:  receiverAddress.ProvinceName,
		DistrictName:  receiverAddress.DistrictName,
		GhnDistrictID: receiverAddress.GhnDistrictID,
		WardName:      receiverAddress.WardName,
		GhnWardCode:   receiverAddress.GhnWardCode,
		Detail:        receiverAddress.Detail,
	}
	
	return delivery.CreateOrderRequest{
		Order:           ghnOrder,
		OrderItems:      ghnOrderItems,
		SenderAddress:   ghnSenderAddress,
		ReceiverAddress: ghnReceiverAddress,
	}
}

// Helper function để tạo thông tin địa chỉ giao nhận hàng
func createDeliveryAddresses(qTx *Queries, ctx context.Context, buyerID, sellerID string, shippingAddress UserAddress) (int64, int64, error) {
	// Tạo thông tin giao hàng của người mua
	buyerDelivery, err := qTx.CreateDeliveryInformation(ctx, CreateDeliveryInformationParams{
		UserID:        buyerID,
		FullName:      shippingAddress.FullName,
		PhoneNumber:   shippingAddress.PhoneNumber,
		ProvinceName:  shippingAddress.ProvinceName,
		DistrictName:  shippingAddress.DistrictName,
		GhnDistrictID: shippingAddress.GhnDistrictID,
		WardName:      shippingAddress.WardName,
		GhnWardCode:   shippingAddress.GhnWardCode,
		Detail:        shippingAddress.Detail,
	})
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create buyer delivery information: %w", err)
	}
	
	// Lấy địa chỉ pickup của người bán
	sellerPickupAddress, err := qTx.GetUserPickupAddress(ctx, sellerID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get seller pickup address: %w", err)
	}
	
	// Tạo thông tin giao hàng của người bán
	sellerDelivery, err := qTx.CreateDeliveryInformation(ctx, CreateDeliveryInformationParams{
		UserID:        sellerID,
		FullName:      sellerPickupAddress.FullName,
		PhoneNumber:   sellerPickupAddress.PhoneNumber,
		ProvinceName:  sellerPickupAddress.ProvinceName,
		DistrictName:  sellerPickupAddress.DistrictName,
		GhnDistrictID: sellerPickupAddress.GhnDistrictID,
		WardName:      sellerPickupAddress.WardName,
		GhnWardCode:   sellerPickupAddress.GhnWardCode,
		Detail:        sellerPickupAddress.Detail,
	})
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create seller delivery information: %w", err)
	}
	
	return sellerDelivery.ID, buyerDelivery.ID, nil
}

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
