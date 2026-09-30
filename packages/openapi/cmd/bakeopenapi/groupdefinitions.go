//go:build !js

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	definitionRef  = regexp.MustCompile(`#/definitions/([^"]+)`)
	groupVersionOf = regexp.MustCompile(`const GroupVersion = "([^"]*)"`)
)

const scaleDefinition = "io.k8s.api.autoscaling.v1.Scale"

type definition struct {
	raw    json.RawMessage
	groups []string
}

func writeGroupDefinitions(v2 []byte) error {
	var doc struct {
		Definitions map[string]json.RawMessage `json:"definitions"`
	}
	if err := json.Unmarshal(v2, &doc); err != nil {
		return err
	}
	definitions := map[string]definition{}
	for name, raw := range doc.Definitions {
		var kinds struct {
			GVKs []struct {
				Group string `json:"group"`
			} `json:"x-kubernetes-group-version-kind"`
		}
		if err := json.Unmarshal(raw, &kinds); err != nil {
			return err
		}
		d := definition{raw: raw}
		for _, gvk := range kinds.GVKs {
			d.groups = append(d.groups, gvk.Group)
		}
		definitions[name] = d
	}
	sources, err := filepath.Glob("packages/apiserver-*/zz_generated_group.go")
	if err != nil {
		return err
	}
	for _, source := range sources {
		body, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		match := groupVersionOf.FindSubmatch(body)
		if match == nil {
			continue
		}
		group := ""
		if prefix, _, ok := strings.Cut(string(match[1]), "/"); ok {
			group = prefix
		}
		if err := writeClosure(filepath.Join(filepath.Dir(source), "openapi.json"), definitions, group); err != nil {
			return err
		}
	}
	return nil
}

func writeClosure(path string, definitions map[string]definition, group string) error {
	closure := map[string]json.RawMessage{}
	pending := []string{scaleDefinition}
	for name, d := range definitions {
		for _, g := range d.groups {
			if g == group {
				pending = append(pending, name)
			}
		}
	}
	for len(pending) > 0 {
		name := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		d, ok := definitions[name]
		if !ok {
			continue
		}
		if _, done := closure[name]; done {
			continue
		}
		closure[name] = d.raw
		for _, ref := range definitionRef.FindAllSubmatch(d.raw, -1) {
			pending = append(pending, string(ref[1]))
		}
	}
	body, err := json.Marshal(closure)
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}
