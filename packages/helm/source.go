package helm

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	helmv1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	"sigs.k8s.io/yaml"
)

const (
	ociChartLayer    = "application/vnd.cncf.helm.chart.content.v1.tar+gzip"
	maxChartBytes    = 32 << 20
	maxIndexBytes    = 64 << 20
	maxManifestBytes = 4 << 20
)

type Source struct {
	HTTP    *http.Client
	Secrets SecretGetter
}

type credentials struct {
	username, password string
}

func (s *Source) Fetch(ctx context.Context, chart *helmv1.HelmChart) ([]byte, error) {
	spec := chart.Spec
	if spec.ChartContent != "" {
		return base64.StdEncoding.DecodeString(spec.ChartContent)
	}
	creds, err := s.credentials(ctx, chart)
	if err != nil {
		return nil, err
	}
	switch {
	case strings.HasPrefix(spec.Chart, "oci://"):
		return s.pullOCI(ctx, spec, creds)
	case strings.Contains(spec.Chart, "://"):
		return s.download(ctx, spec.Chart, creds)
	case spec.Repo == "":
		return nil, fmt.Errorf("chart %q needs spec.repo, a chart URL or spec.chartContent", spec.Chart)
	}
	return s.fromRepository(ctx, spec, creds)
}

func (s *Source) credentials(ctx context.Context, chart *helmv1.HelmChart) (credentials, error) {
	if chart.Spec.AuthSecret == nil {
		return credentials{}, nil
	}
	if s.Secrets == nil {
		return credentials{}, fmt.Errorf("authSecret %s: no secret reader", chart.Spec.AuthSecret.Name)
	}
	data, err := s.Secrets(ctx, chart.Namespace, chart.Spec.AuthSecret.Name)
	if err != nil {
		return credentials{}, fmt.Errorf("authSecret %s: %w", chart.Spec.AuthSecret.Name, err)
	}
	return credentials{username: string(data["username"]), password: string(data["password"])}, nil
}

func (s *Source) get(ctx context.Context, target string, creds credentials, limit int64, headers map[string]string) ([]byte, *http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, nil, err
	}
	if creds.username != "" || creds.password != "" {
		req.SetBasicAuth(creds.username, creds.password)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, resp, err
	}
	if int64(len(body)) > limit {
		return nil, resp, fmt.Errorf("GET %s: response larger than %d bytes", target, limit)
	}
	return body, resp, nil
}

func (s *Source) download(ctx context.Context, target string, creds credentials) ([]byte, error) {
	body, resp, err := s.get(ctx, target, creds, maxChartBytes, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", target, resp.Status)
	}
	return body, nil
}

type repoIndex struct {
	Entries map[string][]struct {
		Version string   `json:"version"`
		URLs    []string `json:"urls"`
	} `json:"entries"`
}

func (s *Source) fromRepository(ctx context.Context, spec helmv1.HelmChartSpec, creds credentials) ([]byte, error) {
	base := strings.TrimSuffix(spec.Repo, "/") + "/"
	raw, resp, err := s.get(ctx, base+"index.yaml", creds, maxIndexBytes, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %sindex.yaml: %s", base, resp.Status)
	}
	var index repoIndex
	if err := yaml.Unmarshal(raw, &index); err != nil {
		return nil, fmt.Errorf("parse %sindex.yaml: %w", base, err)
	}
	name := spec.Chart
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	versions := index.Entries[name]
	if len(versions) == 0 {
		return nil, fmt.Errorf("chart %q not found in repository %s", name, spec.Repo)
	}
	candidates := make([]string, len(versions))
	for i, v := range versions {
		candidates[i] = v.Version
	}
	picked, err := pickVersion(candidates, spec.Version)
	if err != nil {
		return nil, fmt.Errorf("chart %q in %s: %w", name, spec.Repo, err)
	}
	var urls []string
	for _, v := range versions {
		if v.Version == picked {
			urls = v.URLs
			break
		}
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("chart %q %s has no download URL", name, picked)
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	target, err := baseURL.Parse(urls[0])
	if err != nil {
		return nil, err
	}
	if target.Host != baseURL.Host && !spec.AuthPassCredentials {
		creds = credentials{}
	}
	return s.download(ctx, target.String(), creds)
}

func pickVersion(candidates []string, constraint string) (string, error) {
	for _, c := range candidates {
		if c == constraint && constraint != "" {
			return c, nil
		}
	}
	if constraint == "" {
		constraint = "*"
	}
	want, err := semver.NewConstraint(constraint)
	if err != nil {
		return "", fmt.Errorf("invalid version constraint %q: %w", constraint, err)
	}
	type parsed struct {
		raw string
		v   *semver.Version
	}
	var matches []parsed
	for _, c := range candidates {
		v, err := semver.NewVersion(c)
		if err == nil && want.Check(v) {
			matches = append(matches, parsed{c, v})
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no version matches %q", constraint)
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].v.GreaterThan(matches[j].v) })
	return matches[0].raw, nil
}

type ociManifest struct {
	Layers []struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
	} `json:"layers"`
}

func (s *Source) pullOCI(ctx context.Context, spec helmv1.HelmChartSpec, creds credentials) ([]byte, error) {
	ref, err := url.Parse(spec.Chart)
	if err != nil {
		return nil, err
	}
	scheme := "https"
	if spec.PlainHTTP {
		scheme = "http"
	}
	repository := strings.TrimPrefix(ref.Path, "/")
	registry := &registryClient{source: s, base: scheme + "://" + ref.Host + "/v2/" + repository, creds: creds}
	tag := strings.ReplaceAll(spec.Version, "+", "_")
	if tag == "" || strings.ContainsAny(spec.Version, "<>=~^*|, ") {
		tags, err := registry.tags(ctx)
		if err != nil {
			return nil, err
		}
		picked, err := pickVersion(tags, spec.Version)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", spec.Chart, err)
		}
		tag = picked
	}
	raw, err := registry.get(ctx, "/manifests/"+tag, maxManifestBytes, "application/vnd.oci.image.manifest.v1+json")
	if err != nil {
		return nil, err
	}
	var manifest ociManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest %s:%s: %w", spec.Chart, tag, err)
	}
	for _, layer := range manifest.Layers {
		if layer.MediaType != ociChartLayer {
			continue
		}
		blob, err := registry.get(ctx, "/blobs/"+layer.Digest, maxChartBytes, "")
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(blob)
		if want := "sha256:" + hex.EncodeToString(sum[:]); layer.Digest != want {
			return nil, fmt.Errorf("blob %s: digest mismatch, got %s", layer.Digest, want)
		}
		return blob, nil
	}
	return nil, fmt.Errorf("%s:%s has no helm chart layer", spec.Chart, tag)
}

type registryClient struct {
	source *Source
	base   string
	creds  credentials
	token  string
}

func (r *registryClient) tags(ctx context.Context) ([]string, error) {
	raw, err := r.get(ctx, "/tags/list", maxManifestBytes, "")
	if err != nil {
		return nil, err
	}
	var list struct {
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	for i, t := range list.Tags {
		list.Tags[i] = strings.ReplaceAll(t, "_", "+")
	}
	return list.Tags, nil
}

func (r *registryClient) get(ctx context.Context, path string, limit int64, accept string) ([]byte, error) {
	headers := map[string]string{}
	if accept != "" {
		headers["Accept"] = accept
	}
	for attempt := 0; ; attempt++ {
		creds := r.creds
		if r.token != "" {
			headers["Authorization"] = "Bearer " + r.token
			creds = credentials{}
		}
		body, resp, err := r.source.get(ctx, r.base+path, creds, limit, headers)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			if err := r.authenticate(ctx, resp.Header.Get("WWW-Authenticate")); err != nil {
				return nil, err
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GET %s%s: %s", r.base, path, resp.Status)
		}
		return body, nil
	}
}

func (r *registryClient) authenticate(ctx context.Context, challenge string) error {
	scheme, params, _ := strings.Cut(challenge, " ")
	if !strings.EqualFold(scheme, "Bearer") {
		return fmt.Errorf("registry %s needs unsupported authentication %q", r.base, challenge)
	}
	fields := map[string]string{}
	for _, part := range strings.Split(params, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok {
			fields[k] = strings.Trim(v, `"`)
		}
	}
	realm, err := url.Parse(fields["realm"])
	if err != nil || fields["realm"] == "" {
		return fmt.Errorf("registry %s sent a bearer challenge without a realm", r.base)
	}
	query := realm.Query()
	for _, k := range []string{"service", "scope"} {
		if fields[k] != "" {
			query.Set(k, fields[k])
		}
	}
	realm.RawQuery = query.Encode()
	body, resp, err := r.source.get(ctx, realm.String(), r.creds, maxManifestBytes, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token request %s: %s", realm.Host, resp.Status)
	}
	var token struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &token); err != nil {
		return err
	}
	r.token = token.Token
	if r.token == "" {
		r.token = token.AccessToken
	}
	return nil
}
