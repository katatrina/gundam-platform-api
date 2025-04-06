package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func (s *GHNService) GetOrderDetails(ctx context.Context, orderCode string) (*GetOrderDetailsResponse, error) {
	url := GHNBaseURL + "/shipping-order/detail"
	
	requestBody := map[string]string{
		"order_code": orderCode,
	}
	
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("error marshalling request body: %w", err)
	}
	
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Token", s.Token)
	
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GHN API error: status code %d, body: %s", resp.StatusCode, string(body))
	}
	
	var response GetOrderDetailsResponse
	if err = json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("error unmarshalling response: %w", err)
	}
	
	return &response, nil
}
