//go:build js && wasm

package leanclient

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apimachinerywatch "k8s.io/apimachinery/pkg/watch"
	restclient "k8s.io/client-go/rest"
)

// Watch issues a GET with watch=true and returns a watch.Interface backed
// by apimachinery's own watch.StreamWatcher -- genuine reuse of upstream's
// event-channel/Stop() machinery (CLAUDE.md rule 3) -- fed by
// jsonDecoder[T], the only hand-written piece: it decodes the wire
// envelope (metav1.WatchEvent, whose Object field is a
// runtime.RawExtension with its own non-reflective MarshalJSON/
// UnmarshalJSON) with plain encoding/json, then unmarshals the embedded
// object straight into T, never through a runtime.Scheme/
// serializer.CodecFactory.
func Watch[T any](ctx context.Context, c restclient.Interface, resource, namespace string, opts metav1.ListOptions, newT func() *T) (apimachinerywatch.Interface, error) {
	opts.Watch = true
	req := c.Get().Resource(resource)
	if namespace != "" {
		req = req.Namespace(namespace)
	}
	setListParams(req, opts)

	stream, err := req.Stream(ctx)
	if err != nil {
		return nil, err
	}
	reporter := apierrors.NewClientErrorReporter(http.StatusInternalServerError, "GET", "ClientWatchDecoding")
	return apimachinerywatch.NewStreamWatcher(&jsonDecoder[T]{r: bufio.NewReader(stream), stream: stream, newT: newT}, reporter), nil
}

// jsonDecoder implements apimachinery's watch.Decoder over a stream of
// newline-delimited JSON metav1.WatchEvent objects -- the same wire
// format client-go's own rest/watch.Decoder consumes (see
// k8s.io/client-go/rest/watch/decoder.go), minus that decoder's dependency
// on a runtime.Decoder sourced from a Scheme.
type jsonDecoder[T any] struct {
	r      *bufio.Reader
	stream io.Closer
	newT   func() *T
}

func (d *jsonDecoder[T]) Decode() (apimachinerywatch.EventType, runtime.Object, error) {
	line, err := d.r.ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return "", nil, err
	}
	var event metav1.WatchEvent
	if err := json.Unmarshal(line, &event); err != nil {
		return "", nil, err
	}
	obj := d.newT()
	if err := json.Unmarshal(event.Object.Raw, obj); err != nil {
		return "", nil, err
	}
	return apimachinerywatch.EventType(event.Type), any(obj).(runtime.Object), nil
}

func (d *jsonDecoder[T]) Close() {
	d.stream.Close()
}
