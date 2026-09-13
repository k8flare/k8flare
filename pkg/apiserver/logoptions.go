package apiserver

import (
	"net/url"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/client-go/kubernetes/scheme"
)

// client-go's scheme has no query-parameter conversion for PodLogOptions
// (upstream generates it next to the internal types), so the installer
// could not decode ?follow=true&tailLines=10 without this.
func init() {
	if err := scheme.Scheme.AddConversionFunc((*url.Values)(nil), (*corev1.PodLogOptions)(nil), func(a, b any, _ conversion.Scope) error {
		return convertLogOptions(*a.(*url.Values), b.(*corev1.PodLogOptions))
	}); err != nil {
		panic(err)
	}
}

func convertLogOptions(in url.Values, out *corev1.PodLogOptions) error {
	out.Container = in.Get("container")
	out.Stream = optionalString(in.Get("stream"))
	var err error
	if out.Follow, err = optionalBool(in.Get("follow")); err != nil {
		return err
	}
	if out.Previous, err = optionalBool(in.Get("previous")); err != nil {
		return err
	}
	if out.Timestamps, err = optionalBool(in.Get("timestamps")); err != nil {
		return err
	}
	if out.InsecureSkipTLSVerifyBackend, err = optionalBool(in.Get("insecureSkipTLSVerifyBackend")); err != nil {
		return err
	}
	if out.SinceSeconds, err = optionalInt64(in.Get("sinceSeconds")); err != nil {
		return err
	}
	if out.TailLines, err = optionalInt64(in.Get("tailLines")); err != nil {
		return err
	}
	if out.LimitBytes, err = optionalInt64(in.Get("limitBytes")); err != nil {
		return err
	}
	if v := in.Get("sinceTime"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return err
		}
		out.SinceTime = &metav1.Time{Time: t}
	}
	return nil
}

func optionalString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func optionalBool(v string) (bool, error) {
	if v == "" {
		return false, nil
	}
	return strconv.ParseBool(v)
}

func optionalInt64(v string) (*int64, error) {
	if v == "" {
		return nil, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil, err
	}
	return &n, nil
}
