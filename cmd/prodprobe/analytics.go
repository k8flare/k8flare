package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const graphQLEndpoint = "https://api.cloudflare.com/client/v4/graphql"

const invocationsQuery = `query K8flareProbeInvocations($accountTag: string!, $scriptName: string!, $start: Time!, $end: Time!) {
  viewer {
    accounts(filter: {accountTag: $accountTag}) {
      workersInvocationsAdaptive(
        limit: 10000
        filter: {scriptName: $scriptName, datetime_geq: $start, datetime_leq: $end}
      ) {
        sum { requests errors }
      }
      durableObjectsInvocationsAdaptiveGroups(
        limit: 10000
        filter: {scriptName: $scriptName, datetime_geq: $start, datetime_leq: $end}
      ) {
        sum { requests errors }
      }
    }
  }
}`

type invocations struct {
	WorkerRequests int64
	WorkerErrors   int64
	DORequests     int64
	DOErrors       int64
}

func (i invocations) String() string {
	return fmt.Sprintf("worker %d requests / %d errors, durable objects %d requests / %d errors",
		i.WorkerRequests, i.WorkerErrors, i.DORequests, i.DOErrors)
}

func (i invocations) total() int64 { return i.WorkerRequests + i.DORequests }

type sumNode struct {
	Sum struct {
		Requests int64 `json:"requests"`
		Errors   int64 `json:"errors"`
	} `json:"sum"`
}

type graphQLResponse struct {
	Data *struct {
		Viewer *struct {
			Accounts []struct {
				Workers []sumNode `json:"workersInvocationsAdaptive"`
				DO      []sumNode `json:"durableObjectsInvocationsAdaptiveGroups"`
			} `json:"accounts"`
		} `json:"viewer"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func parseInvocations(body []byte) (invocations, error) {
	var resp graphQLResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return invocations{}, fmt.Errorf("decode GraphQL response: %w (body: %s)", err, truncate(body))
	}
	if len(resp.Errors) > 0 {
		msgs := make([]string, 0, len(resp.Errors))
		for _, e := range resp.Errors {
			msgs = append(msgs, e.Message)
		}
		return invocations{}, fmt.Errorf("GraphQL errors: %s", strings.Join(msgs, "; "))
	}
	if resp.Data == nil || resp.Data.Viewer == nil {
		return invocations{}, fmt.Errorf("GraphQL response carried no data (body: %s)", truncate(body))
	}
	if len(resp.Data.Viewer.Accounts) == 0 {
		return invocations{}, fmt.Errorf("GraphQL returned no account: wrong -account, or the API token lacks Account Analytics:Read")
	}
	var out invocations
	for _, acct := range resp.Data.Viewer.Accounts {
		for _, n := range acct.Workers {
			out.WorkerRequests += n.Sum.Requests
			out.WorkerErrors += n.Sum.Errors
		}
		for _, n := range acct.DO {
			out.DORequests += n.Sum.Requests
			out.DOErrors += n.Sum.Errors
		}
	}
	return out, nil
}

func truncate(body []byte) string {
	const max = 400
	if len(body) > max {
		return string(body[:max]) + "..."
	}
	return string(body)
}

type analyticsClient struct {
	accountID  string
	apiToken   string
	scriptName string
	http       *http.Client
	endpoint   string
}

func newAnalyticsClient(accountID, apiToken, scriptName string) *analyticsClient {
	return &analyticsClient{
		accountID:  accountID,
		apiToken:   apiToken,
		scriptName: scriptName,
		http:       &http.Client{Timeout: 60 * time.Second},
		endpoint:   graphQLEndpoint,
	}
}

func (c *analyticsClient) invocations(ctx context.Context, start, end time.Time) (invocations, error) {
	payload, err := json.Marshal(map[string]any{
		"query": invocationsQuery,
		"variables": map[string]any{
			"accountTag": c.accountID,
			"scriptName": c.scriptName,
			"start":      start.UTC().Format(time.RFC3339),
			"end":        end.UTC().Format(time.RFC3339),
		},
	})
	if err != nil {
		return invocations{}, err
	}
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return invocations{}, ctx.Err()
			case <-time.After(time.Duration(attempt) * 5 * time.Second):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
		if err != nil {
			return invocations{}, err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("GraphQL HTTP %d: %s", resp.StatusCode, truncate(body))
			continue
		}
		return parseInvocations(body)
	}
	return invocations{}, lastErr
}
