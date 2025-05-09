package db

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

func (ns NullDeliveryOverralStatus) MarshalJSON() ([]byte, error) {
	if !ns.Valid {
		return []byte("null"), nil
	}
	
	return json.Marshal(string(ns.DeliveryOverralStatus))
}

// func CreateGundamSnapshot(gundam *Gundam, grade *GundamGrade, primaryImageURL string) ([]byte, error) {
// 	snapshot := GundamSnapshot{
// 		ID:       gundam.ID,
// 		Name:     gundam.Name,
// 		Slug:     gundam.Slug,
// 		Grade:    grade.DisplayName,
// 		Scale:    string(gundam.Scale),
// 		Quantity: gundam.Quantity,
// 		Weight:   gundam.Weight,
// 		ImageURL: primaryImageURL,
// 	}
//
// 	return json.Marshal(snapshot)
// }

// Scan implements the sql.Scanner interface.
func (g *GundamSnapshot) Scan(src interface{}) error {
	if src == nil {
		return nil
	}
	
	switch v := src.(type) {
	case []byte:
		// Parse JSON from database bytes
		return json.Unmarshal(v, g)
	
	case string:
		// Parse JSON from string
		return json.Unmarshal([]byte(v), g)
	
	default:
		return fmt.Errorf("unsupported scan type for GundamSnapshot: %T", src)
	}
}

// Value implements the driver.Valuer interface.
func (g GundamSnapshot) Value() (driver.Value, error) {
	if g == (GundamSnapshot{}) {
		return nil, nil
	}
	
	// Convert to JSON
	data, err := json.Marshal(g)
	if err != nil {
		return nil, err
	}
	
	return data, nil
}

// MarshalJSON implements the json.Marshaler interface.
func (g GundamSnapshot) MarshalJSON() ([]byte, error) {
	type Alias GundamSnapshot
	return json.Marshal(&struct {
		Alias
	}{
		Alias: Alias(g),
	})
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (g *GundamSnapshot) UnmarshalJSON(data []byte) error {
	type Alias GundamSnapshot
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(g),
	}
	
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	
	return nil
}
