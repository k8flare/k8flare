package main

import (
	"slices"
	"strings"
	"testing"
)

const dryRunReport = `[{"SuiteDescription":"Suite","SpecReports":[
{"ContainerHierarchyTexts":["[sig-a] Thing"],"LeafNodeText":"should work [Conformance]","LeafNodeType":"It","State":"passed"},
{"ContainerHierarchyTexts":["[sig-a] Thing"],"LeafNodeText":"should work [Conformance]","LeafNodeType":"It","State":"passed"},
{"ContainerHierarchyTexts":["[sig-b] Other [Serial]"],"LeafNodeText":"does (a|b) [Conformance]","LeafNodeType":"It","State":"passed"},
{"ContainerHierarchyTexts":["[sig-c]"],"LeafNodeText":"not focused","LeafNodeType":"It","State":"skipped"},
{"ContainerHierarchyTexts":[],"LeafNodeText":"","LeafNodeType":"SynchronizedBeforeSuite","State":"passed"}
]}]`

func TestSelectedSpecNamesKeepsFocusedSpecsSortedAndUnique(t *testing.T) {
	got, err := selectedSpecNames([]byte(dryRunReport))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"Suite [sig-a] Thing should work [Conformance]",
		"Suite [sig-b] Other [Serial] does (a|b) [Conformance]",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSelectedSpecNamesRejectsGarbage(t *testing.T) {
	if _, err := selectedSpecNames([]byte("not json")); err == nil {
		t.Fatal("want an error")
	}
}

func TestSpecShardsPartitionAndSpreadSerialSpecs(t *testing.T) {
	var names []string
	for i := range 40 {
		names = append(names, "spec "+string(rune('a'+i%26))+string(rune('a'+i/26)))
	}
	for i := range 12 {
		names = append(names, "serial [Serial] "+string(rune('a'+i)))
	}
	slices.Sort(names)
	var covered []string
	for shard := range 4 {
		part := shardSpecNames(names, shard, 4)
		serial := 0
		for _, n := range part {
			if strings.Contains(n, "[Serial]") {
				serial++
			}
		}
		if serial != 3 {
			t.Fatalf("shard %d has %d serial specs, want 3", shard, serial)
		}
		covered = append(covered, part...)
	}
	slices.Sort(covered)
	if !slices.Equal(covered, names) {
		t.Fatal("shards do not cover the specs exactly once")
	}
}

func TestAnchoredFocusFlagsEscapeAndChunk(t *testing.T) {
	names := []string{"a (b|c) [x].", "d"}
	flags := anchoredFocusFlags(names)
	if len(flags) != 1 || flags[0] != `^a \(b\|c\) \[x\]\.$|^d$` {
		t.Fatalf("got %q", flags)
	}
	many := make([]string, focusNamesPerFlag*2+1)
	for i := range many {
		many[i] = "n"
	}
	if got := len(anchoredFocusFlags(many)); got != 3 {
		t.Fatalf("got %d flags, want 3", got)
	}
	if anchoredFocusFlags(nil) != nil {
		t.Fatal("no names must give no flags")
	}
}
