package admission

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apiserver/pkg/authorization/authorizer"
)

const sarPath = "https://apiserver.internal/apis/authorization.k8s.io/v1/subjectaccessreviews"

type sarAuthorizer struct {
	client *http.Client
	token  string
}

func (a *sarAuthorizer) Authorize(ctx context.Context, attr authorizer.Attributes) (authorizer.Decision, string, error) {
	if a == nil || a.client == nil || attr.GetUser() == nil {
		return authorizer.DecisionNoOpinion, "", nil
	}
	review := authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			User:   attr.GetUser().GetName(),
			UID:    attr.GetUser().GetUID(),
			Groups: attr.GetUser().GetGroups(),
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Verb:     attr.GetVerb(),
				Group:    attr.GetAPIGroup(),
				Version:  attr.GetAPIVersion(),
				Resource: attr.GetResource(),
				Name:     attr.GetName(),
			},
		},
	}
	if extra := attr.GetUser().GetExtra(); len(extra) > 0 {
		review.Spec.Extra = make(map[string]authorizationv1.ExtraValue, len(extra))
		for k, v := range extra {
			review.Spec.Extra[k] = v
		}
	}
	body, err := json.Marshal(review)
	if err != nil {
		return authorizer.DecisionNoOpinion, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sarPath, bytes.NewReader(body))
	if err != nil {
		return authorizer.DecisionNoOpinion, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return authorizer.DecisionNoOpinion, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return authorizer.DecisionNoOpinion, "", fmt.Errorf("subjectaccessreview HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&review); err != nil {
		return authorizer.DecisionNoOpinion, "", err
	}
	if review.Status.Allowed {
		return authorizer.DecisionAllow, "", nil
	}
	if review.Status.Denied {
		return authorizer.DecisionDeny, review.Status.Reason, nil
	}
	return authorizer.DecisionNoOpinion, review.Status.Reason, nil
}
