package admission

import (
	"context"
	"fmt"
	"reflect"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/kubernetes/pkg/certauthorization"
)

func applyCertificateApproval(ctx context.Context, authz authorizer.Authorizer, req *admit.Request) error {
	if authz == nil || req.Resource.Resource != "certificatesigningrequests" || req.Subresource != "approval" {
		return nil
	}
	if req.Operation != "" && req.Operation != "UPDATE" {
		return nil
	}
	signer := csrSignerName(req.OldObject)
	if signer == "" {
		signer = csrSignerName(req.Object)
	}
	if signer == "" {
		return nil
	}
	if !certauthorization.IsAuthorizedForSignerName(ctx, authz, userFrom(req.User), "approve", signer) {
		return fmt.Errorf("user not permitted to approve requests with signerName %q", signer)
	}
	return nil
}

func applyCertificateSigning(ctx context.Context, authz authorizer.Authorizer, req *admit.Request) error {
	if authz == nil || req.Resource.Resource != "certificatesigningrequests" || req.Subresource != "status" {
		return nil
	}
	if req.Operation != "" && req.Operation != "UPDATE" {
		return nil
	}
	if !csrStatusChanged(req.OldObject, req.Object) {
		return nil
	}
	signer := csrSignerName(req.OldObject)
	if signer == "" {
		signer = csrSignerName(req.Object)
	}
	if signer == "" {
		return nil
	}
	if !certauthorization.IsAuthorizedForSignerName(ctx, authz, userFrom(req.User), "sign", signer) {
		return fmt.Errorf("user not permitted to sign requests with signerName %q", signer)
	}
	return nil
}

func csrSignerName(obj map[string]any) string {
	if obj == nil {
		return ""
	}
	spec, _ := obj["spec"].(map[string]any)
	name, _ := spec["signerName"].(string)
	return name
}

func csrStatusChanged(oldObj, obj map[string]any) bool {
	if obj == nil {
		return false
	}
	if oldObj == nil {
		return true
	}
	oldStatus, _ := oldObj["status"].(map[string]any)
	status, _ := obj["status"].(map[string]any)
	return !reflect.DeepEqual(oldStatus["certificate"], status["certificate"]) || !reflect.DeepEqual(oldStatus["conditions"], status["conditions"])
}

func userFrom(u admit.User) user.Info {
	return &user.DefaultInfo{Name: u.Username, UID: u.UID, Groups: u.Groups, Extra: u.Extra}
}
