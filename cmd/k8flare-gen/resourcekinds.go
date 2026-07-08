package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/k8flare/k8flare/pkg/apiserver/apidef"
)

// genResourceKinds writes packages/k8s/src/gen/resource-kinds.gen.ts: the
// resource-plural -> Kind lookup table packages/k8s/src/url-mapping.ts uses
// to build synthetic objects (e.g. watch bookmarks) that must decode as the
// correct concrete type. Replaces the hand-written RESOURCE_KINDS map that
// used to live directly in url-mapping.ts, which had already caused two
// separate watch-bookmark bugs (a resource added to the Go apiserver but
// forgotten here) before this generator existed.
func genResourceKinds(root string) error {
	kinds := make(map[string]string, len(apidef.Table))
	for _, def := range apidef.Table {
		kinds[def.Resource] = def.Kind
	}

	names := make([]string, 0, len(kinds))
	for resource := range kinds {
		names = append(names, resource)
	}
	sort.Strings(names)

	var b strings.Builder
	fmt.Fprintf(&b, "// %s\n\n", generatedHeader)
	b.WriteString("/**\n")
	b.WriteString(" * Maps a resource's plural name to its Kind. Covers every resource the Go\n")
	b.WriteString(" * apiserver serves (from pkg/apiserver/apidef.Table).\n")
	b.WriteString(" */\n")
	b.WriteString("export const RESOURCE_KINDS: Record<string, string> = {\n")
	for _, resource := range names {
		fmt.Fprintf(&b, "  %s: %q,\n", tsKey(resource), kinds[resource])
	}
	b.WriteString("};\n")

	return writeFile(root+"/packages/k8s/src/gen/resource-kinds.gen.ts", []byte(b.String()))
}

// tsKey returns key as a bare object-literal key when it's a valid
// identifier (matches every actual resource name today, e.g. "configmaps"),
// or a quoted string otherwise -- defensive, in case a future resource name
// contains a character that isn't.
func tsKey(key string) string {
	for i, r := range key {
		isLetter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || r == '$'
		isDigit := r >= '0' && r <= '9'
		if isLetter || (i > 0 && isDigit) {
			continue
		}
		return fmt.Sprintf("%q", key)
	}
	return key
}
