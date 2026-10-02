package admission

import (
	"context"
	"fmt"
	"strings"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	"github.com/k8flare/k8flare/packages/kubeversion"
	versionutil "k8s.io/apimachinery/pkg/util/version"
	"k8s.io/component-base/version"
	ndf "k8s.io/component-helpers/nodedeclaredfeatures"
)

var nodeFeatureVersion = parseComponentVersion()

func parseComponentVersion() *versionutil.Version {
	parsed, err := versionutil.ParseSemantic(version.Get().String())
	if err != nil {
		parsed, _ = versionutil.ParseSemantic(kubeversion.Major + "." + kubeversion.Minor + ".0")
	}
	return parsed
}

func validateNodeDeclaredFeatures(ctx context.Context, s *store, req *admit.Request) error {
	if req.Resource.Resource != "pods" || req.Object == nil {
		return nil
	}
	if req.Subresource != "" && req.Subresource != "resize" {
		return nil
	}
	if req.Operation != "" && req.Operation != "UPDATE" {
		return nil
	}
	pod, err := decodePod(req.Object)
	if err != nil {
		return err
	}
	if pod.Spec.NodeName == "" {
		return nil
	}
	old, err := decodePod(req.OldObject)
	if err != nil {
		return err
	}
	if old == nil || old.Generation == pod.Generation {
		return nil
	}
	reqs, err := ndf.DefaultFramework.InferForPodUpdate(&ndf.PodInfo{Spec: &old.Spec, Status: &old.Status}, &ndf.PodInfo{Spec: &pod.Spec, Status: &pod.Status}, nodeFeatureVersion)
	if err != nil {
		return fmt.Errorf("failed to infer pod capability requirements: %w", err)
	}
	if reqs.IsEmpty() {
		return nil
	}
	if s == nil {
		return fmt.Errorf("node %q not found", pod.Spec.NodeName)
	}
	node, ok, err := s.node(ctx, pod.Spec.NodeName)
	if err != nil {
		return fmt.Errorf("failed to get node %q: %w", pod.Spec.NodeName, err)
	}
	if !ok {
		return fmt.Errorf("node %q not found", pod.Spec.NodeName)
	}
	result, err := ndf.DefaultFramework.MatchNode(reqs, &node)
	if err != nil {
		return fmt.Errorf("failed to match pod requirements against node %q: %w", node.Name, err)
	}
	if result.IsMatch {
		return nil
	}
	return fmt.Errorf("pod update requires features %s which are not available on node %q", strings.Join(result.UnsatisfiedRequirements, ", "), node.Name)
}
