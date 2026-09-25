package admissionregistration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/cel-go/cel"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
)

const crdPrefix = "/registry/apiextensions.k8s.io/customresourcedefinitions/"

func typeCheckPolicy(policy *admissionregv1.ValidatingAdmissionPolicy) *admissionregv1.TypeChecking {
	return typeCheckPolicyWith(context.Background(), policy, nil)
}

func typeCheckPolicyWith(ctx context.Context, policy *admissionregv1.ValidatingAdmissionPolicy, client *kine.Client) *admissionregv1.TypeChecking {
	schema, hasSchema := matchingCRDSchema(ctx, policy, client)
	env, err := vapTypeEnv(policy)
	if err != nil {
		return &admissionregv1.TypeChecking{}
	}
	check := func(expr string) string {
		if hasSchema {
			return schemaCheckExpr(schema, expr)
		}
		return typeCheckExpr(env, expr)
	}
	return collectTypeWarnings(policy, check)
}

func collectTypeWarnings(policy *admissionregv1.ValidatingAdmissionPolicy, check func(string) string) *admissionregv1.TypeChecking {
	var warnings []admissionregv1.ExpressionWarning
	for i, v := range policy.Spec.Validations {
		if w := check(v.Expression); w != "" {
			warnings = append(warnings, admissionregv1.ExpressionWarning{
				FieldRef: fmt.Sprintf("spec.validations[%d].expression", i),
				Warning:  w,
			})
		}
		if v.MessageExpression == "" {
			continue
		}
		if w := check(v.MessageExpression); w != "" {
			warnings = append(warnings, admissionregv1.ExpressionWarning{
				FieldRef: fmt.Sprintf("spec.validations[%d].messageExpression", i),
				Warning:  w,
			})
		}
	}
	return &admissionregv1.TypeChecking{ExpressionWarnings: warnings}
}

func vapTypeEnv(policy *admissionregv1.ValidatingAdmissionPolicy) (*cel.Env, error) {
	objectType := cel.DynType
	if matchesDeployment(policy) {
		objectType = cel.MapType(cel.StringType, cel.MapType(cel.StringType, cel.IntType))
	}
	return cel.NewEnv(
		cel.Variable("object", objectType),
		cel.Variable("oldObject", cel.DynType),
		cel.Variable("request", cel.DynType),
		cel.Variable("namespaceObject", cel.DynType),
		cel.Variable("authorizer", cel.DynType),
		cel.Variable("params", cel.DynType),
		cel.Variable("variables", cel.DynType),
	)
}

var objectFieldPath = regexp.MustCompile(`\bobject(?:\.[A-Za-z_][A-Za-z0-9_]*)+`)

func schemaCheckExpr(schema schemaNode, expr string) string {
	for _, path := range objectFieldPath.FindAllString(expr, -1) {
		node := schema
		for _, field := range strings.Split(path, ".")[1:] {
			child, ok := node.Properties[field]
			if !ok {
				return fmt.Sprintf("undefined field '%s'", field)
			}
			node = child
		}
		if w := compareWarning(expr, path, node.Type); w != "" {
			return w
		}
	}
	return ""
}

func compareWarning(expr, path, schemaType string) string {
	idx := strings.Index(expr, path)
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(expr[idx+len(path):])
	ops := []struct {
		tok, name string
	}{
		{">=", "_>=_"},
		{"<=", "_<=_"},
		{"==", "_==_"},
		{"!=", "_!=_"},
		{">", "_>_"},
		{"<", "_<_"},
		{"+", "_+_"},
	}
	left := celKind(schemaType)
	for _, op := range ops {
		if !strings.HasPrefix(rest, op.tok) {
			continue
		}
		right := literalKind(strings.TrimSpace(rest[len(op.tok):]))
		if left != "" && right != "" && left != right {
			return fmt.Sprintf("found no matching overload for '%s' applied to '(%s, %s)'", op.name, left, right)
		}
		return ""
	}
	return ""
}

func celKind(schemaType string) string {
	switch schemaType {
	case "integer":
		return "int"
	case "number":
		return "double"
	case "string":
		return "string"
	case "boolean":
		return "bool"
	default:
		return ""
	}
}

func literalKind(lit string) string {
	if strings.HasPrefix(lit, "'") || strings.HasPrefix(lit, `"`) {
		return "string"
	}
	switch {
	case lit == "true" || lit == "false":
		return "bool"
	case lit == "":
		return ""
	default:
		end := 0
		for end < len(lit) && (lit[end] == '-' || lit[end] >= '0' && lit[end] <= '9') {
			end++
		}
		if end == 0 {
			return ""
		}
		if _, err := strconv.ParseInt(lit[:end], 10, 64); err == nil {
			return "int"
		}
		return ""
	}
}

func matchingCRDSchema(ctx context.Context, policy *admissionregv1.ValidatingAdmissionPolicy, client *kine.Client) (schemaNode, bool) {
	if client == nil || policy.Spec.MatchConstraints == nil {
		return schemaNode{}, false
	}
	crds, err := listCRDs(ctx, client)
	if err != nil {
		return schemaNode{}, false
	}
	for _, rule := range policy.Spec.MatchConstraints.ResourceRules {
		for _, crd := range crds {
			if !matchOne(rule.APIGroups, crd.Spec.Group) || !matchOne(rule.Resources, crd.Spec.Names.Plural) {
				continue
			}
			if schema, ok := storageSchema(crd); ok {
				return schema, true
			}
		}
	}
	return schemaNode{}, false
}

type crdLite struct {
	Spec struct {
		Group string `json:"group"`
		Names struct {
			Plural string `json:"plural"`
		} `json:"names"`
		Versions []struct {
			Storage bool `json:"storage"`
			Schema  struct {
				OpenAPIV3Schema schemaNode `json:"openAPIV3Schema"`
			} `json:"schema"`
		} `json:"versions"`
	} `json:"spec"`
}

type schemaNode struct {
	Type       string                `json:"type"`
	Properties map[string]schemaNode `json:"properties"`
}

func listCRDs(ctx context.Context, client *kine.Client) ([]crdLite, error) {
	kvs, _, _, err := client.List(ctx, crdPrefix, "", 0)
	if err != nil {
		return nil, err
	}
	out := make([]crdLite, 0, len(kvs))
	for _, kv := range kvs {
		data, err := base64.StdEncoding.DecodeString(kv.Value)
		if err != nil {
			continue
		}
		var crd crdLite
		if json.Unmarshal(data, &crd) != nil {
			continue
		}
		out = append(out, crd)
	}
	return out, nil
}

func storageSchema(crd crdLite) (schemaNode, bool) {
	for _, v := range crd.Spec.Versions {
		if v.Storage && v.Schema.OpenAPIV3Schema.Type != "" {
			return v.Schema.OpenAPIV3Schema, true
		}
	}
	for _, v := range crd.Spec.Versions {
		if v.Schema.OpenAPIV3Schema.Type != "" {
			return v.Schema.OpenAPIV3Schema, true
		}
	}
	return schemaNode{}, false
}

func matchesDeployment(policy *admissionregv1.ValidatingAdmissionPolicy) bool {
	if policy.Spec.MatchConstraints == nil {
		return false
	}
	for _, rule := range policy.Spec.MatchConstraints.ResourceRules {
		if !matchOne(rule.APIGroups, "apps") || !matchOne(rule.APIVersions, "v1") || !matchOne(rule.Resources, "deployments") {
			continue
		}
		return true
	}
	return false
}

func matchOne(want []string, got string) bool {
	for _, w := range want {
		if w == "*" || w == got {
			return true
		}
	}
	return false
}

func typeCheckExpr(env *cel.Env, expr string) string {
	_, iss := env.Compile(expr)
	if iss == nil || iss.Err() == nil {
		return ""
	}
	return iss.Err().Error()
}
