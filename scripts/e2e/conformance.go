package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const focusNamesPerFlag = 50

type dryRunSuite struct {
	SpecReports []dryRunSpec
}

type dryRunSpec struct {
	ContainerHierarchyTexts []string
	LeafNodeText            string
	State                   string
}

func selectedSpecNames(report []byte) ([]string, error) {
	var suites []dryRunSuite
	if err := json.Unmarshal(report, &suites); err != nil {
		return nil, fmt.Errorf("parse ginkgo json report: %w", err)
	}
	seen := map[string]bool{}
	var names []string
	for _, suite := range suites {
		for _, spec := range suite.SpecReports {
			if spec.State != "passed" || spec.LeafNodeText == "" {
				continue
			}
			name := strings.Join(append(slices.Clone(spec.ContainerHierarchyTexts), spec.LeafNodeText), " ")
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	slices.Sort(names)
	return names, nil
}

func shardSpecNames(names []string, shard, shards int) []string {
	var serial, parallel []string
	for _, name := range names {
		if strings.Contains(name, "[Serial]") {
			serial = append(serial, name)
		} else {
			parallel = append(parallel, name)
		}
	}
	return append(shardOf(parallel, shard, shards), shardOf(serial, shard, shards)...)
}

func anchoredFocusFlags(names []string) []string {
	var flags []string
	for start := 0; start < len(names); start += focusNamesPerFlag {
		end := min(start+focusNamesPerFlag, len(names))
		escaped := make([]string, 0, end-start)
		for _, name := range names[start:end] {
			escaped = append(escaped, "^"+regexp.QuoteMeta(name)+"$")
		}
		flags = append(flags, strings.Join(escaped, "|"))
	}
	return flags
}

func listSelectedSpecs(e2eTest, kubeconfig, focus, skip, reportDir string, nodes int) ([]string, error) {
	absReport, err := filepath.Abs(reportDir)
	if err != nil {
		return nil, err
	}
	absKubeconfig, err := filepath.Abs(kubeconfig)
	if err != nil {
		return nil, err
	}
	jsonPath := filepath.Join(absReport, "dry-run.json")
	args := []string{
		"--no-color",
		"--dry-run",
		"--procs=1",
		"--focus=" + focus,
		"--json-report=" + jsonPath,
	}
	if skip != "" {
		args = append(args, "--skip="+skip)
	}
	args = append(args, e2eTest, "--",
		"--kubeconfig", absKubeconfig,
		"--kube-api-content-type=application/json",
		"--provider=skeleton",
		fmt.Sprintf("--num-nodes=%d", nodes),
		"--disable-log-dump",
		"--report-dir", absReport,
	)
	cmd := exec.Command(filepath.Join(filepath.Dir(e2eTest), "ginkgo"), args...)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ginkgo dry run: %w", err)
	}
	report, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}
	return selectedSpecNames(report)
}
