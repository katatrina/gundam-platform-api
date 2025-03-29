package util

import (
	"fmt"
	
	"github.com/lithammer/shortuuid/v4"
)

const (
	alphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
)

func GenerateOrderID() string {
	uuid := shortuuid.NewWithAlphabet(alphabet)
	return fmt.Sprintf("ORD-%s", uuid[:10])
}
