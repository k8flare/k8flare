package admit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

const ValidateEndpoint = "https://admission.internal/validate"

type ValidateRequest struct {
	Resource    string         `json:"resource"`
	Subresource string         `json:"subresource"`
	Update      bool           `json:"update"`
	Object      map[string]any `json:"object"`
	OldObject   map[string]any `json:"oldObject"`
}

type ValidateResponse struct {
	Errors field.ErrorList `json:"errors,omitempty"`
}

func Validate(ctx context.Context, client *http.Client, in ValidateRequest) field.ErrorList {
	body, err := json.Marshal(in)
	if err != nil {
		return field.ErrorList{field.InternalError(nil, err)}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ValidateEndpoint, bytes.NewReader(body))
	if err != nil {
		return field.ErrorList{field.InternalError(nil, err)}
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return field.ErrorList{field.InternalError(nil, fmt.Errorf("validate: %w", err))}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return field.ErrorList{field.InternalError(nil, fmt.Errorf("validate: %w", err))}
	}
	if resp.StatusCode != http.StatusOK {
		return field.ErrorList{field.InternalError(nil, fmt.Errorf("validate: HTTP %d: %s", resp.StatusCode, data))}
	}
	var out ValidateResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return field.ErrorList{field.InternalError(nil, fmt.Errorf("validate: %w", err))}
	}
	return out.Errors
}
