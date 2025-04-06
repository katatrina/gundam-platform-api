package delivery

import (
	"context"
)

const (
	// GHNBaseURL is the base URL for GHN API
	GHNBaseURL = "https://dev-online-gateway.ghn.vn/shiip/public-api/v2"
)

type IDeliveryProvider interface {
	CreateOrder(ctx context.Context, request CreateOrderRequest) (*CreateOrderResponse, error)
}

type GHNService struct {
	Token  string
	ShopID string
}

func NewGHNService(token, shopID string) IDeliveryProvider {
	return &GHNService{
		Token:  token,
		ShopID: shopID,
	}
}
