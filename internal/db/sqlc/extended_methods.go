package db

import (
	"encoding/json"
)

func (ns NullDeliveryOverralStatus) MarshalJSON() ([]byte, error) {
	if !ns.Valid {
		return []byte("null"), nil
	}
	
	return json.Marshal(string(ns.DeliveryOverralStatus))
}
