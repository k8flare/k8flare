package admission

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	jsonpatch "github.com/evanphx/json-patch"
	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	admissionv1 "k8s.io/api/admission/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

const (
	workerURLPrefix = "https://k8flare.com/worker/"
	workerAnnot     = "k8flare.com/worker"
	hooksBase       = "https://hooks.internal/hook/"
	tunnelDialBase  = "https://nodetunnel.internal/dial/"
)

func (h *Handler) callWebhook(ctx context.Context, req admit.Request, annotations map[string]string, name string, cfg admissionregv1.WebhookClientConfig, timeout *int32, failure *admissionregv1.FailurePolicyType, obj map[string]any) (map[string]any, error) {
	seconds := 10
	if timeout != nil && *timeout > 0 {
		seconds = int(*timeout)
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
	defer cancel()
	review := admissionReview(req, obj)
	body, err := json.Marshal(review)
	if err != nil {
		return nil, failClosed(failure, err)
	}
	httpReq, err := h.webhookRequest(ctx, annotations, cfg, body)
	if err != nil {
		return nil, failClosed(failure, err)
	}
	q := httpReq.URL.Query()
	q.Set("timeout", strconv.Itoa(seconds)+"s")
	httpReq.URL.RawQuery = q.Encode()
	client := h.hooks
	switch httpReq.URL.Host {
	case "nodetunnel.internal":
		client = h.tunnel
	case "hooks.internal":
		client = h.hooks
	default:
		client = h.outbound
	}
	if client == nil {
		return nil, failClosed(failure, fmt.Errorf("webhook %s: no client", name))
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, failClosed(failure, fmt.Errorf("webhook %s: %w", name, err))
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, failClosed(failure, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, failClosed(failure, fmt.Errorf("webhook %s: HTTP %d: %s", name, resp.StatusCode, string(data)))
	}
	var out admissionv1.AdmissionReview
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, failClosed(failure, err)
	}
	if out.Response == nil {
		return nil, failClosed(failure, fmt.Errorf("webhook %s: empty response", name))
	}
	if !out.Response.Allowed {
		return nil, fmt.Errorf("admission webhook %q denied the request: %s", name, denyMessage(out.Response.Result))
	}
	if len(out.Response.Patch) == 0 || obj == nil {
		return obj, nil
	}
	patched, err := applyPatch(obj, out.Response.Patch)
	if err != nil {
		return nil, failClosed(failure, fmt.Errorf("webhook %s: %w", name, err))
	}
	return patched, nil
}

func (h *Handler) webhookRequest(ctx context.Context, annotations map[string]string, cfg admissionregv1.WebhookClientConfig, body []byte) (*http.Request, error) {
	if name := strings.TrimSpace(annotations[workerAnnot]); name != "" {
		return hookRequest(ctx, name, body)
	}
	if cfg.URL != nil && strings.HasPrefix(*cfg.URL, workerURLPrefix) {
		return hookRequest(ctx, strings.TrimPrefix(*cfg.URL, workerURLPrefix), body)
	}
	if cfg.URL != nil && *cfg.URL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, *cfg.URL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}
	if cfg.Service == nil {
		return nil, fmt.Errorf("webhook clientConfig has neither url nor service")
	}
	return h.serviceRequest(ctx, *cfg.Service, cfg.CABundle, body)
}

func hookRequest(ctx context.Context, name string, body []byte) (*http.Request, error) {
	name = strings.Trim(name, "/")
	if name == "" || strings.Contains(name, "/") {
		return nil, fmt.Errorf("invalid worker name %q", name)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hooksBase+name, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func (h *Handler) serviceRequest(ctx context.Context, svc admissionregv1.ServiceReference, ca []byte, body []byte) (*http.Request, error) {
	target, node, err := h.resolveService(ctx, svc)
	if err != nil {
		return nil, err
	}
	path := "/"
	if svc.Path != nil && *svc.Path != "" {
		path = *svc.Path
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
	}
	url := tunnelDialBase + node + "/" + target.host + "/" + target.port + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Dial-TLS", "1")
	req.Header.Set("X-Dial-ServerName", svc.Name+"."+svc.Namespace+".svc")
	if len(ca) == 0 {
		return nil, fmt.Errorf("x509: certificate signed by unknown authority")
	}
	req.Header.Set("X-Dial-CA", base64.StdEncoding.EncodeToString(ca))
	return req, nil
}

type dialTarget struct {
	host string
	port string
}

func (h *Handler) resolveService(ctx context.Context, svc admissionregv1.ServiceReference) (dialTarget, string, error) {
	port := int32(443)
	if svc.Port != nil && *svc.Port != 0 {
		port = *svc.Port
	}
	ep, ok, err := h.store.endpoints(ctx, svc.Namespace, svc.Name)
	if err != nil {
		return dialTarget{}, "", err
	}
	if ok {
		for _, subset := range ep.Subsets {
			for _, addr := range subset.Addresses {
				p := port
				for _, sp := range subset.Ports {
					if sp.Port != 0 {
						p = sp.Port
						break
					}
				}
				node := ""
				if addr.NodeName != nil {
					node = *addr.NodeName
				}
				if node == "" && addr.TargetRef != nil && addr.TargetRef.Kind == "Pod" {
					pod, found, err := h.store.pod(ctx, addr.TargetRef.Namespace, addr.TargetRef.Name)
					if err != nil {
						return dialTarget{}, "", err
					}
					if found {
						node = pod.Spec.NodeName
					}
				}
				if addr.IP != "" && node != "" {
					return dialTarget{host: addr.IP, port: strconv.Itoa(int(p))}, node, nil
				}
			}
		}
	}
	slices, err := h.store.endpointSlices(ctx, svc.Namespace)
	if err != nil {
		return dialTarget{}, "", err
	}
	for _, slice := range slices {
		if slice.Labels["kubernetes.io/service-name"] == svc.Name {
			for _, endpoint := range slice.Endpoints {
				if len(endpoint.Addresses) == 0 {
					continue
				}
				node := ""
				if endpoint.NodeName != nil {
					node = *endpoint.NodeName
				}
				if node == "" && endpoint.TargetRef != nil && endpoint.TargetRef.Kind == "Pod" {
					pod, found, err := h.store.pod(ctx, endpoint.TargetRef.Namespace, endpoint.TargetRef.Name)
					if err != nil {
						return dialTarget{}, "", err
					}
					if found {
						node = pod.Spec.NodeName
					}
				}
				p := port
				if len(slice.Ports) > 0 && slice.Ports[0].Port != nil {
					p = *slice.Ports[0].Port
				}
				if node != "" {
					return dialTarget{host: endpoint.Addresses[0], port: strconv.Itoa(int(p))}, node, nil
				}
			}
		}
	}
	return dialTarget{}, "", fmt.Errorf("no ready endpoints for service %s/%s", svc.Namespace, svc.Name)
}

func admissionReview(req admit.Request, obj map[string]any) admissionv1.AdmissionReview {
	gvk := metav1.GroupVersionKind{Group: req.Kind.Group, Version: req.Kind.Version, Kind: req.Kind.Kind}
	gvr := metav1.GroupVersionResource{Group: req.Resource.Group, Version: req.Resource.Version, Resource: req.Resource.Resource}
	var object, old runtime.RawExtension
	if obj != nil {
		object.Raw, _ = json.Marshal(obj)
	}
	if req.OldObject != nil {
		old.Raw, _ = json.Marshal(req.OldObject)
	}
	return admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{APIVersion: "admission.k8s.io/v1", Kind: "AdmissionReview"},
		Request: &admissionv1.AdmissionRequest{
			UID:         types.UID(req.Name + "/" + req.Operation),
			Kind:        gvk,
			Resource:    gvr,
			SubResource: req.Subresource,
			Name:        req.Name,
			Namespace:   req.Namespace,
			Operation:   admissionv1.Operation(req.Operation),
			Object:      object,
			OldObject:   old,
			DryRun:      &req.DryRun,
			UserInfo:    userInfo(req.User),
		},
	}
}

func userInfo(u admit.User) authenticationv1.UserInfo {
	extra := map[string]authenticationv1.ExtraValue{}
	for k, v := range u.Extra {
		extra[k] = v
	}
	return authenticationv1.UserInfo{Username: u.Username, UID: u.UID, Groups: u.Groups, Extra: extra}
}

func applyPatch(obj map[string]any, patch []byte) (map[string]any, error) {
	current, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	decoded := patch
	if !bytes.HasPrefix(bytes.TrimSpace(patch), []byte("[")) {
		decoded, err = decodePatch(patch)
		if err != nil {
			return nil, err
		}
	}
	p, err := jsonpatch.DecodePatch(decoded)
	if err != nil {
		return nil, err
	}
	out, err := p.Apply(current)
	if err != nil {
		return nil, err
	}
	var next map[string]any
	if err := json.Unmarshal(out, &next); err != nil {
		return nil, err
	}
	return next, nil
}

func denyMessage(status *metav1.Status) string {
	if status == nil {
		return "without explanation"
	}
	if status.Message != "" {
		return status.Message
	}
	if status.Reason != "" {
		return string(status.Reason)
	}
	return "without explanation"
}

func decodePatch(patch []byte) ([]byte, error) {
	if decoded, err := base64.StdEncoding.DecodeString(string(patch)); err == nil && bytes.HasPrefix(bytes.TrimSpace(decoded), []byte("[")) {
		return decoded, nil
	}
	return patch, nil
}
