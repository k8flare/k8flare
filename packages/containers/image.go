package containers

import (
	"encoding/json"
	"fmt"
	"regexp"
)

const (
	DeclaredImagesEnv   = "CONTAINERS_IMAGES"
	declaredImagePrefix = "images/"
	platformImage       = "cloudflare/debian-trixie"
)

var accountRegistryImage = regexp.MustCompile(`^registry\.cloudflare\.com/[^@\s]+@sha256:[0-9a-f]{64}$`)

func ParseDeclaredImages(raw string) []string {
	var names []string
	if raw == "" || json.Unmarshal([]byte(raw), &names) != nil {
		return nil
	}
	return names
}

func ResolveImage(image string, declared []string) (string, error) {
	for _, name := range declared {
		if image == name {
			return declaredImagePrefix + name, nil
		}
	}
	if image == platformImage || accountRegistryImage.MatchString(image) {
		return image, nil
	}
	return "", fmt.Errorf("image %q cannot run on Containers: images must be declared in wrangler.jsonc containers[].images or be digest-pinned in the account registry (registry.cloudflare.com/<account>/<repo>@sha256:<digest>)", image)
}
