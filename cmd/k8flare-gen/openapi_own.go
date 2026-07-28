package main

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"

	"github.com/k8flare/k8flare/pkg/apiserver/apidef"
)

// ownGroupOpenAPIV3 builds the OpenAPI v3 document for one of k8flare's own
// API groups -- the ones with no upstream document to copy (see
// ownOpenAPIGroups in openapi.go).
//
// kubectl needs this document to exist for a very specific reason, and the
// reason determines what it has to contain. kubectl's client-side
// validation is a *fallback*: paramVerifyingSchema (k8s.io/kubectl
// pkg/validation/schema.go) first asks whether the server supports
// ?fieldValidation= for the object's GVK, and only validates client-side if
// the answer is "unsupported". queryParamVerifierV3
// (k8s.io/cli-runtime pkg/resource/query_param_verifier_v3.go) answers that
// by looking up the GroupVersion's v3 document and scanning it for a PATCH
// operation carrying a matching x-kubernetes-group-version-kind extension
// and a "fieldValidation" query parameter. A GroupVersion missing from the
// v3 index makes it fall through to the v2 verifier, which fetches
// /openapi/v2 as protobuf -- which our Static Assets copy cannot be (it is
// upstream's JSON swagger.json, served as-is whatever the Accept header
// asks for), so kubectl fails the apply outright with "proto: cannot parse
// invalid wire-format data".
//
// Correction (2026-07-28): that last clause no longer holds -- /openapi/v2
// is now served as real protobuf when asked for it (genOpenAPI writes a
// v2.pb alongside the JSON; the Worker picks by Accept). Generating this v3
// document is still right, because it is what makes kubectl defer to
// server-side validation rather than fall back at all, but a missing v3
// document is now a degradation (v2's permissive schema) instead of a hard
// failure. The v2 fallback was found to be reachable regardless of this
// group -- any GroupVersion in discovery but absent from the v3 index hits
// it, e.g. authentication.k8s.io/v1.
//
// So the load-bearing content here is the operations' GVK extensions and
// their fieldValidation parameters, not the schemas: with those present
// kubectl defers to server-side field validation, which this apiserver
// really does implement and default to Strict (pkg/apiserver/fieldvalidation.go).
// The object schemas are therefore deliberately permissive
// (x-kubernetes-preserve-unknown-fields), because generating a faithful one
// from a hand-written Go type needs upstream's openapi-gen wired into
// cmd/k8flare-gen. That is a real gap only for `kubectl explain`, not for
// validation -- see docs/cluster-api-design.md.
func ownGroupOpenAPIV3(gv schema.GroupVersion) ([]byte, error) {
	doc := &spec3.OpenAPI{
		Version: "3.0.0",
		Info: &spec.Info{
			InfoProps: spec.InfoProps{
				Title:   "k8flare",
				Version: "unversioned",
			},
		},
		Paths:      &spec3.Paths{Paths: map[string]*spec3.Path{}},
		Components: &spec3.Components{Schemas: map[string]*spec.Schema{}},
	}

	for _, def := range apidef.ForGroupVersion(gv) {
		addResourcePaths(doc, def)
	}

	return json.Marshal(doc)
}

// addResourcePaths adds the collection, item and subresource paths for one
// resource, plus the permissive object/list schemas they reference.
func addResourcePaths(doc *spec3.OpenAPI, def apidef.ResourceDef) {
	gv := def.GroupVersion
	objName := schemaName(gv, def.Kind)
	listName := schemaName(gv, def.Kind+"List")
	doc.Components.Schemas[objName] = objectSchema(gv, def.Kind, fmt.Sprintf("%s is a k8flare %s.", def.Kind, def.Kind))
	doc.Components.Schemas[listName] = objectSchema(gv, def.Kind+"List", fmt.Sprintf("%sList is a list of %s.", def.Kind, def.Kind))

	prefix := fmt.Sprintf("/apis/%s/%s", gv.Group, gv.Version)
	if def.Namespaced {
		prefix += "/namespaces/{namespace}"
	}
	collection := fmt.Sprintf("%s/%s", prefix, def.Resource)
	item := collection + "/{name}"

	doc.Paths.Paths[collection] = &spec3.Path{
		PathProps: spec3.PathProps{
			Parameters: scopeParameters(def, false),
			Get:        operation(def, "list", listName, false),
			Post:       operation(def, "post", objName, true),
		},
	}
	doc.Paths.Paths[item] = &spec3.Path{
		PathProps: spec3.PathProps{
			Parameters: scopeParameters(def, true),
			Get:        operation(def, "get", objName, false),
			Put:        operation(def, "put", objName, true),
			Patch:      operation(def, "patch", objName, true),
			Delete:     operation(def, "delete", objName, false),
		},
	}

	for _, sub := range def.Subresources {
		kind := def.Kind
		if sub.Kind != "" {
			kind = sub.Kind
		}
		subSchema := schemaName(gv, kind)
		if _, ok := doc.Components.Schemas[subSchema]; !ok {
			doc.Components.Schemas[subSchema] = objectSchema(gv, kind, kind+" is a k8flare subresource object.")
		}
		subDef := def
		subDef.Kind = kind
		doc.Paths.Paths[item+"/"+sub.Name] = &spec3.Path{
			PathProps: spec3.PathProps{
				Parameters: scopeParameters(def, true),
				Get:        operation(subDef, "get", subSchema, false),
				Put:        operation(subDef, "put", subSchema, true),
				Patch:      operation(subDef, "patch", subSchema, true),
			},
		}
	}
}

// operation builds one OpenAPI operation. The x-kubernetes-group-version-kind
// extension and, for mutating operations, the fieldValidation query
// parameter are what kubectl's verifier actually reads -- see
// ownGroupOpenAPIV3's doc comment.
func operation(def apidef.ResourceDef, action, schemaRef string, mutating bool) *spec3.Operation {
	gv := def.GroupVersion
	op := &spec3.Operation{
		OperationProps: spec3.OperationProps{
			OperationId: fmt.Sprintf("%s%s%s", action, gv.Version, def.Kind),
			Tags:        []string{gv.Group + "_" + gv.Version},
			Responses: &spec3.Responses{
				ResponsesProps: spec3.ResponsesProps{
					StatusCodeResponses: map[int]*spec3.Response{
						200: {
							ResponseProps: spec3.ResponseProps{
								Description: "OK",
								Content: map[string]*spec3.MediaType{
									"application/json": {
										MediaTypeProps: spec3.MediaTypeProps{Schema: refSchema(schemaRef)},
									},
								},
							},
						},
					},
				},
			},
		},
		VendorExtensible: spec.VendorExtensible{
			Extensions: spec.Extensions{
				"x-kubernetes-group-version-kind": gvkExtension(gv, def.Kind),
				"x-kubernetes-action":             action,
			},
		},
	}
	if mutating {
		op.Parameters = mutatingParameters()
		op.RequestBody = &spec3.RequestBody{
			RequestBodyProps: spec3.RequestBodyProps{
				Required: true,
				Content: map[string]*spec3.MediaType{
					"application/json": {
						MediaTypeProps: spec3.MediaTypeProps{Schema: refSchema(schemaRef)},
					},
				},
			},
		}
	}
	return op
}

// mutatingParameters are the query parameters real kube-apiserver accepts on
// create/update/patch. fieldValidation is the one kubectl's verifier looks
// for; dryRun and fieldManager are listed because this apiserver accepts
// them too and an OpenAPI document that omits accepted parameters is
// misleading.
func mutatingParameters() []*spec3.Parameter {
	names := []struct{ name, desc string }{
		{"fieldValidation", "fieldValidation instructs the server on how to handle objects in the request containing unknown or duplicate fields: Ignore, Warn, or Strict. Defaults to Strict."},
		{"dryRun", "When present, indicates that modifications should not be persisted."},
		{"fieldManager", "fieldManager is a name associated with the actor or entity that is making these changes."},
	}
	params := make([]*spec3.Parameter, 0, len(names))
	for _, n := range names {
		params = append(params, &spec3.Parameter{
			ParameterProps: spec3.ParameterProps{
				Name:        n.name,
				In:          "query",
				Description: n.desc,
				Schema:      &spec.Schema{SchemaProps: spec.SchemaProps{Type: []string{"string"}}},
			},
		})
	}
	return params
}

// scopeParameters are the path parameters for a resource's URL: {namespace}
// for namespaced resources, plus {name} on item paths.
func scopeParameters(def apidef.ResourceDef, item bool) []*spec3.Parameter {
	var params []*spec3.Parameter
	if def.Namespaced {
		params = append(params, pathParameter("namespace", "object name and auth scope, such as for teams and projects"))
	}
	if item {
		params = append(params, pathParameter("name", "name of the "+def.Kind))
	}
	return params
}

func pathParameter(name, desc string) *spec3.Parameter {
	return &spec3.Parameter{
		ParameterProps: spec3.ParameterProps{
			Name:        name,
			In:          "path",
			Description: desc,
			Required:    true,
			Schema:      &spec.Schema{SchemaProps: spec.SchemaProps{Type: []string{"string"}}},
		},
	}
}

// objectSchema is the permissive stand-in schema described in
// ownGroupOpenAPIV3's doc comment: it identifies the type by GVK but
// declares no properties, so nothing is rejected for being unknown.
func objectSchema(gv schema.GroupVersion, kind, description string) *spec.Schema {
	return &spec.Schema{
		SchemaProps: spec.SchemaProps{
			Description: description,
			Type:        []string{"object"},
		},
		VendorExtensible: spec.VendorExtensible{
			Extensions: spec.Extensions{
				"x-kubernetes-group-version-kind":      []map[string]string{gvkExtension(gv, kind)},
				"x-kubernetes-preserve-unknown-fields": true,
			},
		},
	}
}

func gvkExtension(gv schema.GroupVersion, kind string) map[string]string {
	return map[string]string{"group": gv.Group, "version": gv.Version, "kind": kind}
}

func refSchema(name string) *spec.Schema {
	return &spec.Schema{
		SchemaProps: spec.SchemaProps{Ref: spec.MustCreateRef("#/components/schemas/" + name)},
	}
}

// schemaName renders the reverse-DNS component key upstream uses, e.g.
// "com.k8flare.v1alpha1.Cluster" for k8flare.com/v1alpha1 Cluster.
func schemaName(gv schema.GroupVersion, kind string) string {
	parts := splitDots(gv.Group)
	reversed := ""
	for i := len(parts) - 1; i >= 0; i-- {
		reversed += parts[i] + "."
	}
	return reversed + gv.Version + "." + kind
}

func splitDots(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}
