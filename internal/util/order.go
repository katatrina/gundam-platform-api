package util

import (
	"github.com/lithammer/shortuuid/v4"
)

func GenerateOrderID() string {
	alphabet := "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
	uuid := shortuuid.NewWithAlphabet(alphabet)
	return uuid[:14]
}
