//go:build !js

package apiserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	auth "github.com/k8flare/k8flare/packages/apiserver-auth"
	"k8s.io/apiserver/pkg/authentication/user"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
)

type inflightRig struct {
	handler http.Handler
	release chan struct{}
	entered chan struct{}
}

func newInflightRig(nonMutating, mutating int) *inflightRig {
	rig := &inflightRig{release: make(chan struct{}), entered: make(chan struct{}, 16)}
	serving := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test-Hold") == "" {
			return
		}
		rig.entered <- struct{}{}
		<-rig.release
	})
	limited := auth.WithRequestInfo(withInflightLimit(serving, nonMutating, mutating))
	rig.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := &user.DefaultInfo{Name: "someone", Groups: []string{user.AllAuthenticated}}
		if r.Header.Get("X-Test-Masters") != "" {
			u.Groups = append(u.Groups, user.SystemPrivilegedGroup)
		}
		limited.ServeHTTP(w, r.WithContext(genericapirequest.WithUser(r.Context(), u)))
	})
	return rig
}

func (r *inflightRig) hold(method, path string) chan int {
	done := make(chan int, 1)
	go func() {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("X-Test-Hold", "1")
		rec := httptest.NewRecorder()
		r.handler.ServeHTTP(rec, req)
		done <- rec.Code
	}()
	<-r.entered
	return done
}

func (r *inflightRig) code(method, path string, masters bool) (int, string) {
	req := httptest.NewRequest(method, path, nil)
	if masters {
		req.Header.Set("X-Test-Masters", "1")
	}
	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	return rec.Code, rec.Header().Get("Retry-After")
}

func TestReadOnlyRequestsBeyondTheLimitGet429(t *testing.T) {
	rig := newInflightRig(1, 1)
	held := rig.hold(http.MethodGet, "/api/v1/namespaces/default/pods")
	if code, retry := rig.code(http.MethodGet, "/api/v1/namespaces/default/pods", false); code != http.StatusTooManyRequests || retry != "1" {
		t.Fatalf("second read: %d retry-after=%q", code, retry)
	}
	close(rig.release)
	if code := <-held; code != http.StatusOK {
		t.Fatalf("held request: %d", code)
	}
	if code, _ := rig.code(http.MethodGet, "/api/v1/namespaces/default/pods", false); code != http.StatusOK {
		t.Fatalf("read after release: %d", code)
	}
}

func TestMutatingAndReadOnlyLimitsAreSeparate(t *testing.T) {
	rig := newInflightRig(1, 1)
	held := rig.hold(http.MethodGet, "/api/v1/namespaces/default/pods")
	writeHeld := rig.hold(http.MethodPost, "/api/v1/namespaces/default/pods")
	if code, _ := rig.code(http.MethodPost, "/api/v1/namespaces/default/pods", false); code != http.StatusTooManyRequests {
		t.Fatalf("second write: %d", code)
	}
	close(rig.release)
	<-held
	<-writeHeld
}

func TestWatchesAndStreamsDoNotUseSlots(t *testing.T) {
	rig := newInflightRig(1, 1)
	held := rig.hold(http.MethodGet, "/api/v1/namespaces/default/pods")
	for _, path := range []string{
		"/api/v1/namespaces/default/pods?watch=true",
		"/api/v1/namespaces/default/pods/p/exec",
		"/api/v1/namespaces/default/pods/p/log",
		"/api/v1/namespaces/default/pods/p/portforward",
		"/api/v1/namespaces/default/services/s/proxy/x",
	} {
		if code, _ := rig.code(http.MethodGet, path, false); code != http.StatusOK {
			t.Errorf("%s: %d", path, code)
		}
	}
	close(rig.release)
	<-held
}

func TestPrivilegedGroupIsNeverRejected(t *testing.T) {
	rig := newInflightRig(1, 1)
	held := rig.hold(http.MethodGet, "/api/v1/namespaces/default/pods")
	if code, _ := rig.code(http.MethodGet, "/api/v1/namespaces/default/pods", true); code != http.StatusOK {
		t.Fatalf("masters: %d", code)
	}
	close(rig.release)
	<-held
}

func TestZeroLimitsDisableTheFilter(t *testing.T) {
	rig := newInflightRig(0, 0)
	held := rig.hold(http.MethodGet, "/api/v1/pods")
	if code, _ := rig.code(http.MethodGet, "/api/v1/pods", false); code != http.StatusOK {
		t.Fatalf("got %d", code)
	}
	close(rig.release)
	<-held
}
