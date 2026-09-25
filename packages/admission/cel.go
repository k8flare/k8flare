package admission

import (
	"fmt"
	"sync"

	"github.com/google/cel-go/cel"
	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
)

var (
	celOnce sync.Once
	celEnv  *cel.Env
	celErr  error
)

func env() (*cel.Env, error) {
	celOnce.Do(func() {
		celEnv, celErr = cel.NewEnv(
			cel.Variable("object", cel.DynType),
			cel.Variable("oldObject", cel.DynType),
			cel.Variable("request", cel.DynType),
			cel.Variable("namespaceObject", cel.DynType),
			cel.Variable("authorizer", cel.DynType),
			cel.Variable("params", cel.DynType),
			cel.Variable("variables", cel.DynType),
		)
	})
	return celEnv, celErr
}

func asCEL(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = asCEL(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = asCEL(val)
		}
		return out
	case float64:
		if t == float64(int64(t)) {
			return int64(t)
		}
		return t
	default:
		return v
	}
}

func celVars(req admit.Request) map[string]any {
	object := any(req.Object)
	if req.Object == nil {
		object = nil
	} else {
		object = asCEL(req.Object)
	}
	old := any(req.OldObject)
	if req.OldObject == nil {
		old = nil
	} else {
		old = asCEL(req.OldObject)
	}
	return map[string]any{
		"object":    object,
		"oldObject": old,
		"request": map[string]any{
			"kind":        map[string]any{"group": req.Kind.Group, "version": req.Kind.Version, "kind": req.Kind.Kind},
			"resource":    map[string]any{"group": req.Resource.Group, "version": req.Resource.Version, "resource": req.Resource.Resource},
			"subResource": req.Subresource,
			"name":        req.Name,
			"namespace":   req.Namespace,
			"operation":   req.Operation,
			"userInfo":    map[string]any{"username": req.User.Username, "uid": req.User.UID, "groups": req.User.Groups},
			"dryRun":      req.DryRun,
		},
	}
}

func evalBool(expr string, vars map[string]any) (bool, error) {
	e, err := env()
	if err != nil {
		return false, err
	}
	ast, iss := e.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return false, iss.Err()
	}
	prg, err := e.Program(ast)
	if err != nil {
		return false, err
	}
	out, _, err := prg.Eval(vars)
	if err != nil {
		return false, err
	}
	b, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("expression %q returned %T", expr, out.Value())
	}
	return b, nil
}

func evalAny(expr string, vars map[string]any) (any, error) {
	e, err := env()
	if err != nil {
		return nil, err
	}
	ast, iss := e.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return nil, iss.Err()
	}
	prg, err := e.Program(ast)
	if err != nil {
		return nil, err
	}
	out, _, err := prg.Eval(vars)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, nil
	}
	return out.Value(), nil
}

func evalVariables(defs []admissionregv1.Variable, vars map[string]any) map[string]any {
	out := map[string]any{}
	vars["variables"] = out
	for _, d := range defs {
		v, err := evalAny(d.Expression, vars)
		if err != nil {
			continue
		}
		out[d.Name] = v
	}
	return out
}

func evalString(expr string, vars map[string]any) (string, error) {
	e, err := env()
	if err != nil {
		return "", err
	}
	ast, iss := e.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return "", iss.Err()
	}
	prg, err := e.Program(ast)
	if err != nil {
		return "", err
	}
	out, _, err := prg.Eval(vars)
	if err != nil {
		return "", err
	}
	s, ok := out.Value().(string)
	if !ok {
		return "", fmt.Errorf("expression %q returned %T", expr, out.Value())
	}
	return s, nil
}

func compileExpr(expr string) error {
	e, err := env()
	if err != nil {
		return err
	}
	_, iss := e.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return fmt.Errorf("compilation failed: %w", iss.Err())
	}
	return nil
}
