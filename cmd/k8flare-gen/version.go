package main

import (
	"fmt"
	"regexp"
	"strings"
)

// semverPattern extracts Major.Minor(.Patch) from a `go list -m` version
// string, e.g. "v1.36.2" -> ["v1.36.2", "1", "36"]. k8s.io/kubernetes
// versions follow plain semver (unlike the k3s-io fork's "-k3s1" suffixed
// replace target), so this is deliberately simple rather than a general
// semver parser.
var semverPattern = regexp.MustCompile(`^v(\d+)\.(\d+)(?:\.\d+)?`)

// genVersion writes pkg/apiserver/zz_generated_version.go: the
// Major/Minor/GitVersion GET /version reports, read from go.mod's resolved
// k8s.io/kubernetes version instead of a hand-maintained literal (which had
// drifted -- it hard-coded Minor "34" while go.mod had long since moved to
// 1.36).
func genVersion(root string) error {
	version, err := goListModuleVersion(root, "k8s.io/kubernetes")
	if err != nil {
		return err
	}

	m := semverPattern.FindStringSubmatch(version)
	if m == nil {
		return fmt.Errorf("k8s.io/kubernetes version %q does not look like semver", version)
	}
	major, minor := m[1], m[2]
	gitVersion := strings.TrimSuffix(version, "+incompatible") + "+k8flare"

	src := fmt.Sprintf(`// %s
package apiserver

// kubernetesMajor, kubernetesMinor, and kubernetesGitVersion mirror the
// k8s.io/kubernetes version pinned in go.mod (resolved via 'go list -m'),
// so GET /version's reported version can't silently drift out of sync with
// the actually-vendored API types the way a hand-maintained literal did
// (this project's oldest one hard-coded Minor "34" long after go.mod had
// moved on to 1.36 -- see docs/k8s-version-bump.md).
const (
	kubernetesMajor      = %q
	kubernetesMinor      = %q
	kubernetesGitVersion = %q
)
`, generatedHeader, major, minor, gitVersion)

	return writeGoFile(root+"/pkg/apiserver/zz_generated_version.go", []byte(src))
}
