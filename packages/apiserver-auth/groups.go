package auth

import (
	"context"
	"net/http"

	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/group"
)

type authenticatedGroupToken struct {
	tokens authenticator.Token
}

func WithAuthenticatedGroup(tokens authenticator.Token) authenticator.Token {
	return authenticatedGroupToken{tokens: tokens}
}

func (a authenticatedGroupToken) AuthenticateToken(ctx context.Context, token string) (*authenticator.Response, bool, error) {
	resp, ok, err := a.tokens.AuthenticateToken(ctx, token)
	if err != nil || !ok {
		return nil, ok, err
	}
	adder := group.NewAuthenticatedGroupAdder(authenticator.RequestFunc(func(*http.Request) (*authenticator.Response, bool, error) {
		return resp, true, nil
	}))
	return adder.AuthenticateRequest(nil)
}
