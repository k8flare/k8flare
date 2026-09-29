package main

import (
	"slices"
	"testing"
)

func TestShardsCoverTheSetOnceBetweenThem(t *testing.T) {
	names := sets["required"]
	var covered []string
	for shard := range 3 {
		part := shardOf(names, shard, 3)
		if len(part) == 0 {
			t.Fatalf("shard %d is empty", shard)
		}
		covered = append(covered, part...)
	}
	slices.Sort(covered)
	want := slices.Clone(names)
	slices.Sort(want)
	if !slices.Equal(covered, want) {
		t.Fatalf("shards covered %q, want %q", covered, want)
	}
}

func TestOneShardIsTheWholeSet(t *testing.T) {
	names := sets["required"]
	if got := shardOf(names, 0, 1); !slices.Equal(got, names) {
		t.Fatalf("got %q", got)
	}
}
