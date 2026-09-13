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
	if len(files) == 0 {
		t.Fatalf("no Durable Object database under %s", devState)
	}
	out, err := exec.Command("sqlite3", files[0], "SELECT COUNT(*) FROM kine").Output()
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	if rows == 0 || rows > 1100 {
		t.Fatalf("kine rows after 1101 writes: %d (compaction should keep about %d)", rows, 1000)
	}
	t.Logf("kine rows after compaction: %d", rows)
}
