package addons

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"io"
	"sort"
	"strings"

	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

//go:embed manifests/*.yaml
var manifestFS embed.FS

//go:embed addon-crd.yaml
var addonCRD []byte

const defaultLocalStoragePath = "/var/lib/rancher/k3s/storage"

type File struct {
	Name    string
	Content []byte
}

func Packaged() []File {
	entries, err := manifestFS.ReadDir("manifests")
	if err != nil {
		panic(err)
	}
	files := make([]File, 0, len(entries))
	for _, e := range entries {
		content, err := manifestFS.ReadFile("manifests/" + e.Name())
		if err != nil {
			panic(err)
		}
		files = append(files, File{Name: e.Name(), Content: content})
	}
	return files
}

func Vars() map[string]string {
	return map[string]string{
		"%{CLUSTER_DNS}%":                 supervisor.ClusterDNS.String(),
		"%{CLUSTER_DNS_LIST}%":            fmt.Sprintf("[%s]", supervisor.ClusterDNS),
		"%{CLUSTER_DNS_IPFAMILYPOLICY}%":  "SingleStack",
		"%{CLUSTER_DOMAIN}%":              supervisor.ClusterDomain,
		"%{DEFAULT_LOCAL_STORAGE_PATH}%":  defaultLocalStoragePath,
		"%{SYSTEM_DEFAULT_REGISTRY}%":     "",
		"%{SYSTEM_DEFAULT_REGISTRY_RAW}%": "",
	}
}

func Render(files []File, vars map[string]string) []File {
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]File, len(files))
	for i, f := range files {
		content := f.Content
		for _, k := range keys {
			content = bytes.ReplaceAll(content, []byte(k), []byte(vars[k]))
		}
		out[i] = File{Name: f.Name, Content: content}
	}
	return out
}

func ParseDisable(list string) map[string]bool {
	disables := map[string]bool{}
	for _, name := range strings.Split(list, ",") {
		if name = strings.TrimSpace(name); name != "" {
			disables[name] = true
		}
	}
	return disables
}

func Decode(content []byte) ([]*unstructured.Unstructured, error) {
	var objs []*unstructured.Unstructured
	reader := yaml.NewYAMLReader(bufio.NewReaderSize(bytes.NewReader(content), 4096))
	for {
		raw, err := reader.Read()
		if err == io.EOF {
			return objs, nil
		}
		if err != nil {
			return nil, err
		}
		if isEmpty(raw) {
			continue
		}
		jsonDoc, err := yaml.ToJSON(raw)
		if err != nil {
			return nil, err
		}
		obj, _, err := unstructured.UnstructuredJSONScheme.Decode(jsonDoc, nil, nil)
		if err != nil {
			return nil, err
		}
		if list, ok := obj.(*unstructured.UnstructuredList); ok {
			for i := range list.Items {
				objs = append(objs, &list.Items[i])
			}
			continue
		}
		objs = append(objs, obj.(*unstructured.Unstructured))
	}
}

func isEmpty(doc []byte) bool {
	for _, line := range bytes.Split(doc, []byte("\n")) {
		s := bytes.TrimSpace(line)
		if string(s) != "---" && !bytes.HasPrefix(s, []byte("#")) && len(s) != 0 {
			return false
		}
	}
	return true
}
