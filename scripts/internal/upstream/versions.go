package upstream

import (
	_ "embed"
	"fmt"

	"golang.org/x/mod/modfile"
)

//go:embed versions.mod
var versionsMod []byte

func parseVersions(name string, data []byte) (map[string]string, error) {
	file, err := modfile.Parse(name, data, nil)
	if err != nil {
		return nil, err
	}
	versions := map[string]string{}
	for _, req := range file.Require {
		if _, ok := versions[req.Mod.Path]; ok {
			return nil, fmt.Errorf("%s: %s is required twice", name, req.Mod.Path)
		}
		versions[req.Mod.Path] = req.Mod.Version
	}
	return versions, nil
}

func VersionOf(module string) (string, error) {
	versions, err := parseVersions("scripts/internal/upstream/versions.mod", versionsMod)
	if err != nil {
		return "", err
	}
	version, ok := versions[module]
	if !ok {
		return "", fmt.Errorf("scripts/internal/upstream/versions.mod has no require line for %s", module)
	}
	return version, nil
}

func mustVersionOf(module string) string {
	version, err := VersionOf(module)
	if err != nil {
		panic(err)
	}
	return version
}
