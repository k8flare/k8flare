//go:build !js

package apiserver_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestCompaction(t *testing.T) {
	url, _ := startDevURL(t)
	cs := kubernetes.NewForConfigOrDie(&rest.Config{Host: url, BearerToken: devToken, QPS: 1000, Burst: 1000})
	c, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cms := cs.CoreV1().ConfigMaps("default")
	first, err := cms.Create(c, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "compact"}, Data: map[string]string{"n": "0"}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cur := first
	for i := 1; i <= 1100; i++ {
		cur.Data["n"] = strconv.Itoa(i)
		if cur, err = cms.Update(c, cur, metav1.UpdateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := cms.Get(c, "compact", metav1.GetOptions{})
	if err != nil || got.Data["n"] != "1100" {
		t.Fatalf("latest value after compaction: %v %v", got.Data, err)
	}
	w, err := cms.Watch(c, metav1.ListOptions{ResourceVersion: first.ResourceVersion})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	select {
	case ev := <-w.ResultChan():
		status, ok := ev.Object.(*metav1.Status)
		if ev.Type != watch.Error || !ok || status.Reason != metav1.StatusReasonExpired {
			t.Fatalf("watch from a compacted revision: got %s %v", ev.Type, ev.Object)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("no watch event for a compacted revision")
	}
	files, _ := filepath.Glob(filepath.Join(devState, "v3", "do", "*", "*.sqlite"))
	var db string
	for _, f := range files {
		if filepath.Base(f) == "metadata.sqlite" {
			continue
		}
		if exec.Command("sqlite3", f, "SELECT 1 FROM kine LIMIT 1").Run() == nil {
			db = f
			break
		}
	}
	if db == "" {
		t.Fatalf("no kine database under %s", devState)
	}
	out, err := exec.Command("sqlite3", db, "SELECT COUNT(*), (SELECT value FROM meta WHERE key = 'compact_revision'), (SELECT MAX(id) FROM kine) FROM kine").Output()
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Split(strings.TrimSpace(string(out)), "|")
	if len(fields) != 3 {
		t.Fatalf("kine stats: %q", out)
	}
	rows, _ := strconv.Atoi(fields[0])
	compact, _ := strconv.Atoi(fields[1])
	maxID, _ := strconv.Atoi(fields[2])
	if rows == 0 || compact < 1 || rows >= maxID || rows > maxID-compact+500 {
		t.Fatalf("kine rows=%d compact=%d max=%d (want history dropped to about %d retained revisions)", rows, compact, maxID, 1000)
	}
	t.Logf("kine rows=%d compact=%d max=%d", rows, compact, maxID)
}
