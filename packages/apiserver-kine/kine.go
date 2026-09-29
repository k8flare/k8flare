package kine

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/storage"
)

// Client speaks the Cluster Durable Object's key-value protocol
// (packages/cluster-store). Every key is stored under "/registry".
type Client struct {
	HTTP *http.Client
}

const kineBase = "http://cluster.internal"

type KV struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	ModRevision int64  `json:"modRevision"`
}

type kineResponse struct {
	Revision int64  `json:"revision"`
	KV       *KV    `json:"kv"`
	KVs      []KV   `json:"kvs"`
	More     bool   `json:"more"`
	Error    string `json:"error"`
}

var (
	ErrNotFound  = fmt.Errorf("kine: not found")
	ErrConflict  = fmt.Errorf("kine: conflict")
	ErrCompacted = fmt.Errorf("kine: compacted")
)

func (c *Client) call(ctx context.Context, method, path string, query url.Values, body any) (*kineResponse, error) {
	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = bytes.NewReader(data)
	}
	u := kineBase + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, payload)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kine %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	var out kineResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("kine %s %s: status %d: %w", method, path, resp.StatusCode, err)
	}
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		return &out, nil
	case http.StatusNotFound:
		return &out, ErrNotFound
	case http.StatusConflict:
		return &out, ErrConflict
	case http.StatusGone:
		return &out, ErrCompacted
	}
	return nil, fmt.Errorf("kine %s %s: status %d: %s", method, path, resp.StatusCode, out.Error)
}

func (c *Client) Get(ctx context.Context, key string) (*KV, int64, error) {
	out, err := c.call(ctx, http.MethodGet, "/kv", url.Values{"key": {key}}, nil)
	if err != nil {
		return nil, 0, err
	}
	if out.KV == nil {
		return nil, out.Revision, ErrNotFound
	}
	return out.KV, out.Revision, nil
}

func (c *Client) Put(ctx context.Context, key string, value []byte, revision int64) (int64, error) {
	out, err := c.call(ctx, http.MethodPut, "/kv", nil, map[string]any{
		"key": key, "value": base64.StdEncoding.EncodeToString(value), "revision": revision,
	})
	if err != nil {
		return 0, err
	}
	return out.Revision, nil
}

func (c *Client) Delete(ctx context.Context, key string, revision int64) (int64, error) {
	out, err := c.call(ctx, http.MethodDelete, "/kv", nil, map[string]any{"key": key, "revision": revision})
	if err != nil {
		return 0, err
	}
	return out.Revision, nil
}

func (c *Client) List(ctx context.Context, prefix, from string, limit int) ([]KV, int64, bool, error) {
	return c.ListAt(ctx, prefix, from, limit, 0)
}

func (c *Client) ListAt(ctx context.Context, prefix, from string, limit int, revision int64) ([]KV, int64, bool, error) {
	q := url.Values{"prefix": {prefix}}
	if from != "" {
		q.Set("from", from)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if revision > 0 {
		q.Set("revision", strconv.FormatInt(revision, 10))
	}
	out, err := c.call(ctx, http.MethodGet, "/list", q, nil)
	if err != nil {
		return nil, 0, false, err
	}
	return out.KVs, out.Revision, out.More, nil
}

func (c *Client) CompactRevision(ctx context.Context) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, kineBase+"/stats", nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var out struct {
		CompactRevision int64 `json:"compactRevision"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("kine GET /stats: status %d", resp.StatusCode)
	}
	return out.CompactRevision, nil
}

func (c *Client) Revision(ctx context.Context) (int64, error) {
	out, err := c.call(ctx, http.MethodGet, "/revision", nil, nil)
	if err != nil {
		return 0, err
	}
	return out.Revision, nil
}

// Storage is the storage.Interface genericregistry.Store runs on.
type Storage struct {
	client  *Client
	codec   runtime.Codec
	newFunc func() runtime.Object
}

var (
	_         storage.Interface = (*Storage)(nil)
	versioner storage.Versioner = storage.APIObjectVersioner{}
)

const registryPrefix = "/registry"

func NewStorage(client *Client, codec runtime.Codec, newFunc func() runtime.Object) *Storage {
	return &Storage{client: client, codec: codec, newFunc: newFunc}
}

func (s *Storage) Versioner() storage.Versioner { return versioner }

func (s *Storage) decodeInto(data []byte, rev int64, into runtime.Object) error {
	if _, _, err := s.codec.Decode(data, nil, into); err != nil {
		return err
	}
	return versioner.UpdateObject(into, uint64(rev))
}

func (s *Storage) decodeKV(kv *KV, into runtime.Object) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(kv.Value)
	if err != nil {
		return nil, err
	}
	return data, s.decodeInto(data, kv.ModRevision, into)
}

func (s *Storage) encode(obj runtime.Object) ([]byte, error) {
	if err := versioner.PrepareObjectForStorage(obj); err != nil {
		return nil, err
	}
	return runtime.Encode(s.codec, obj)
}

func (s *Storage) Create(ctx context.Context, key string, obj, out runtime.Object, _ uint64) error {
	data, err := s.encode(obj)
	if err != nil {
		return err
	}
	rev, err := s.client.Put(ctx, registryPrefix+key, data, 0)
	if err == ErrConflict {
		return storage.NewKeyExistsError(key, 0)
	}
	if err != nil {
		return err
	}
	if out != nil {
		return s.decodeInto(data, rev, out)
	}
	return nil
}

func (s *Storage) Get(ctx context.Context, key string, opts storage.GetOptions, objPtr runtime.Object) error {
	kv, _, err := s.client.Get(ctx, registryPrefix+key)
	if err == ErrNotFound {
		if opts.IgnoreNotFound {
			return runtime.SetZeroValue(objPtr)
		}
		return storage.NewKeyNotFoundError(key, 0)
	}
	if err != nil {
		return err
	}
	_, err = s.decodeKV(kv, objPtr)
	return err
}

func compactedContinue(continueKey, keyPrefix string) error {
	newToken, err := storage.EncodeContinue(continueKey, keyPrefix, -1)
	if err != nil {
		return apierrors.NewResourceExpired("The provided continue parameter is too old to display a consistent list result. You can start a new list without the continue parameter.")
	}
	status := apierrors.NewResourceExpired("The provided continue parameter is too old to display a consistent list result. You can start a new list without the continue parameter, or use the continue token in this response to retrieve the remainder of the results. Continuing with the provided token results in an inconsistent list - objects that were created, modified, or deleted between the time the first chunk was returned and now may show up in the list.")
	status.ErrStatus.ListMeta.Continue = newToken
	return status
}

func (s *Storage) GetList(ctx context.Context, key string, opts storage.ListOptions, listObj runtime.Object) error {
	prefix := registryPrefix + key
	if opts.Recursive && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	at, from, err := storage.ValidateListOptions(prefix, versioner, opts)
	if err != nil {
		return err
	}
	startFrom := from
	limit := opts.Predicate.Limit
	var items []runtime.Object
	var rev int64
	next := ""
	collect := func(kvs []KV) error {
		for i := range kvs {
			if limit > 0 && int64(len(items)) >= limit {
				next = kvs[i].Key
				return nil
			}
			obj := s.newFunc()
			if _, err := s.decodeKV(&kvs[i], obj); err != nil {
				return err
			}
			if ok, err := opts.Predicate.Matches(obj); err != nil {
				return err
			} else if ok {
				items = append(items, obj)
			}
		}
		return nil
	}
	if opts.Recursive {
		for next == "" {
			page, r, more, err := s.client.ListAt(ctx, prefix, from, 500, at)
			if err == ErrCompacted {
				if from != "" {
					return compactedContinue(from, prefix)
				}
				return apierrors.NewResourceExpired("The resourceVersion for the provided list is too old.")
			}
			if err != nil {
				return err
			}
			rev = r
			if at == 0 {
				at = r
			}
			if err := collect(page); err != nil {
				return err
			}
			if !more || len(page) == 0 {
				break
			}
			from = page[len(page)-1].Key + "\x00"
		}
	} else {
		kv, r, err := s.client.Get(ctx, prefix)
		rev = r
		if err == nil {
			if err := collect([]KV{*kv}); err != nil {
				return err
			}
		} else if err != ErrNotFound {
			return err
		}
	}
	if err := meta.SetList(listObj, items); err != nil {
		return err
	}
	listRV := rev
	if at > 0 {
		listRV = at
	}
	continueToken := ""
	var remaining *int64
	if next != "" {
		var err error
		if continueToken, err = storage.EncodeContinue(next, prefix, listRV); err != nil {
			return err
		}
		if opts.Predicate.Empty() {
			rest, _, _, err := s.client.ListAt(ctx, prefix, startFrom, 0, listRV)
			if err != nil {
				return err
			}
			n := int64(len(rest) - len(items))
			if n < 0 {
				n = 0
			}
			remaining = &n
		}
	}
	return versioner.UpdateList(listObj, uint64(listRV), continueToken, remaining)
}

func (s *Storage) GuaranteedUpdate(ctx context.Context, key string, destination runtime.Object, ignoreNotFound bool, preconditions *storage.Preconditions, tryUpdate storage.UpdateFunc, _ runtime.Object) error {
	fullKey := registryPrefix + key
	for {
		current := s.newFunc()
		var rev int64
		var currentData []byte
		kv, _, err := s.client.Get(ctx, fullKey)
		switch {
		case err == nil:
			if currentData, err = s.decodeKV(kv, current); err != nil {
				return err
			}
			rev = kv.ModRevision
		case err == ErrNotFound:
			if !ignoreNotFound {
				return storage.NewKeyNotFoundError(key, 0)
			}
			if err := runtime.SetZeroValue(current); err != nil {
				return err
			}
		default:
			return err
		}
		if preconditions != nil {
			if err := preconditions.Check(key, current); err != nil {
				return err
			}
		}
		ret, _, err := tryUpdate(current, storage.ResponseMeta{ResourceVersion: uint64(rev)})
		if err != nil {
			return err
		}
		data, err := s.encode(ret)
		if err != nil {
			return err
		}
		if rev != 0 && bytes.Equal(data, currentData) {
			return s.decodeInto(data, rev, destination)
		}
		newRev, err := s.client.Put(ctx, fullKey, data, rev)
		if err == ErrConflict || err == ErrNotFound {
			continue
		}
		if err != nil {
			return err
		}
		return s.decodeInto(data, newRev, destination)
	}
}

func (s *Storage) Delete(ctx context.Context, key string, out runtime.Object, preconditions *storage.Preconditions, validateDeletion storage.ValidateObjectFunc, _ runtime.Object, _ storage.DeleteOptions) error {
	fullKey := registryPrefix + key
	for {
		kv, _, err := s.client.Get(ctx, fullKey)
		if err == ErrNotFound {
			return storage.NewKeyNotFoundError(key, 0)
		}
		if err != nil {
			return err
		}
		if _, err := s.decodeKV(kv, out); err != nil {
			return err
		}
		if preconditions != nil {
			if err := preconditions.Check(key, out); err != nil {
				return err
			}
		}
		if validateDeletion != nil {
			if err := validateDeletion(ctx, out); err != nil {
				return err
			}
		}
		_, err = s.client.Delete(ctx, fullKey, kv.ModRevision)
		if err == ErrConflict {
			continue
		}
		if err == ErrNotFound {
			return storage.NewKeyNotFoundError(key, 0)
		}
		return err
	}
}

// WatchDialer opens the Cluster DO's event stream. It is set by the wasm
// entrypoint; host builds have none.
var WatchDialer func(ctx context.Context, rawURL string) (<-chan []byte, func(), error)

var Resident func() bool

type kineEvent struct {
	Rev   int64  `json:"rev"`
	Type  string `json:"type"`
	Key   string `json:"key"`
	Value string `json:"value"`
	Prev  string `json:"prev"`
}

func (s *Storage) Watch(ctx context.Context, key string, opts storage.ListOptions) (watch.Interface, error) {
	if WatchDialer == nil {
		return nil, fmt.Errorf("watch is not available in this build")
	}
	prefix := registryPrefix + key
	q := url.Values{}
	if opts.Recursive {
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
	} else {
		q.Set("exact", "1")
	}
	q.Set("prefix", prefix)
	rv, err := versioner.ParseResourceVersion(opts.ResourceVersion)
	if err != nil {
		return nil, err
	}
	q.Set("since", strconv.FormatUint(rv, 10))
	initial := rv == 0 || (opts.SendInitialEvents != nil && *opts.SendInitialEvents)
	if initial {
		q.Set("initial", "1")
	}
	ctx, cancel := context.WithCancel(ctx)
	dial := func(since int64, initial bool) (<-chan []byte, func(), error) {
		q.Set("since", strconv.FormatInt(since, 10))
		if initial {
			q.Set("initial", "1")
		} else {
			q.Del("initial")
		}
		return WatchDialer(ctx, kineBase+"/watch?"+q.Encode())
	}
	println("kine: watch dial start", prefix, "since", strconv.FormatUint(rv, 10))
	dialStart := time.Now()
	msgs, closeFn, err := redial(ctx, func() (<-chan []byte, func(), error) { return dial(int64(rv), initial) })
	if err != nil {
		cancel()
		println("kine: watch dial failed", prefix+":", err.Error())
		return nil, err
	}
	println("kine: watch dial ok", prefix, "in", time.Since(dialStart).Round(time.Millisecond).String())
	events := make(chan watch.Event, 64)
	w := watch.NewProxyWatcher(events)
	go func() {
		defer cancel()
		defer close(events)
		lastRev := int64(rv)
		snapshotDone := !initial
		for {
			select {
			case <-w.StopChan():
				closeFn()
				return
			case <-time.After(watchIdleTimeout):
				closeFn()
				println("kine: watch end", prefix, "reason=idle-watchdog since", strconv.FormatInt(lastRev, 10))
				expire(events, w, "no watch progress; relist")
				return
			case msg, ok := <-msgs:
				if !ok {
					closeFn()
					if ctx.Err() != nil {
						return
					}
					if Resident != nil && !Resident() {
						expired := apierrors.NewResourceExpired("watch socket closed outside a resident window; relist")
						select {
						case events <- watch.Event{Type: watch.Error, Object: &expired.ErrStatus}:
						case <-w.StopChan():
						}
						return
					}
					println("kine: watch socket closed, redialing", prefix, "since", lastRev)
					msgs, closeFn, err = redial(ctx, func() (<-chan []byte, func(), error) { return dial(lastRev, !snapshotDone) })
					if err != nil {
						println("kine: watch end", prefix, "reason=redial-exhausted:", err.Error())
						expire(events, w, "watch source is gone; relist")
						return
					}
					continue
				}
				var ev kineEvent
				if err := json.Unmarshal(msg, &ev); err != nil {
					continue
				}
				if ev.Rev > lastRev {
					lastRev = ev.Rev
				}
				if ev.Type == "compacted" {
					closeFn()
					expired := apierrors.NewResourceExpired(fmt.Sprintf("resource version %d is older than the compacted revision %d", rv, ev.Rev))
					select {
					case events <- watch.Event{Type: watch.Error, Object: &expired.ErrStatus}:
					case <-w.StopChan():
					}
					return
				}
				if ev.Type == "progress" {
					if opts.Predicate.AllowWatchBookmarks {
						bookmark := s.newFunc()
						if versioner.UpdateObject(bookmark, uint64(ev.Rev)) == nil {
							select {
							case events <- watch.Event{Type: watch.Bookmark, Object: bookmark}:
							case <-w.StopChan():
								closeFn()
								return
							}
						}
					}
					continue
				}
				if ev.Type == "snapshot-end" {
					snapshotDone = true
					if opts.SendInitialEvents == nil || !*opts.SendInitialEvents || !opts.Predicate.AllowWatchBookmarks {
						continue
					}
					bookmark := s.newFunc()
					if m, err := meta.Accessor(bookmark); err == nil {
						m.SetAnnotations(map[string]string{metav1.InitialEventsAnnotationKey: "true"})
					}
					if err := versioner.UpdateObject(bookmark, uint64(ev.Rev)); err != nil {
						continue
					}
					select {
					case events <- watch.Event{Type: watch.Bookmark, Object: bookmark}:
					case <-w.StopChan():
						closeFn()
						return
					}
					continue
				}
				out, ok := s.watchEvent(ev, opts.Predicate)
				if !ok {
					continue
				}
				select {
				case events <- out:
				case <-w.StopChan():
					closeFn()
					return
				}
			}
		}
	}()
	return w, nil
}

func (s *Storage) decodeValue(b64 string, rev int64) (runtime.Object, bool) {
	if b64 == "" {
		return nil, false
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, false
	}
	obj := s.newFunc()
	if err := s.decodeInto(data, rev, obj); err != nil {
		return nil, false
	}
	return obj, true
}

// watchEvent applies upstream's filtering rule: an object that stops
// matching the predicate is reported as deleted, one that starts matching
// as added.
func (s *Storage) watchEvent(ev kineEvent, pred storage.SelectionPredicate) (watch.Event, bool) {
	cur, hasCur := s.decodeValue(ev.Value, ev.Rev)
	prev, hasPrev := s.decodeValue(ev.Prev, ev.Rev)
	matches := func(obj runtime.Object, ok bool) bool {
		if !ok {
			return false
		}
		m, err := pred.Matches(obj)
		return err == nil && m
	}
	curMatch, prevMatch := matches(cur, hasCur), matches(prev, hasPrev)
	switch ev.Type {
	case "deleted":
		if curMatch {
			return watch.Event{Type: watch.Deleted, Object: cur}, true
		}
	case "created":
		if curMatch {
			return watch.Event{Type: watch.Added, Object: cur}, true
		}
	case "modified":
		switch {
		case curMatch && prevMatch:
			return watch.Event{Type: watch.Modified, Object: cur}, true
		case curMatch:
			return watch.Event{Type: watch.Added, Object: cur}, true
		case prevMatch:
			return watch.Event{Type: watch.Deleted, Object: cur}, true
		}
	}
	return watch.Event{}, false
}

func (s *Storage) GetCurrentResourceVersion(ctx context.Context) (uint64, error) {
	rev, err := s.client.Revision(ctx)
	return uint64(rev), err
}

func (s *Storage) Stats(context.Context) (storage.Stats, error)        { return storage.Stats{}, nil }
func (s *Storage) ReadinessCheck() error                               { return nil }
func (s *Storage) RequestWatchProgress(context.Context) error          { return nil }
func (s *Storage) EnableResourceSizeEstimation(storage.KeysFunc) error { return nil }
func (s *Storage) CompactRevision() int64                              { return 0 }

const (
	watchIdleTimeout = 90 * time.Second
	redialAttempts   = 5
	redialDelay      = 500 * time.Millisecond
)

func redial(ctx context.Context, dial func() (<-chan []byte, func(), error)) (<-chan []byte, func(), error) {
	var err error
	for attempt := 0; attempt < redialAttempts; attempt++ {
		var msgs <-chan []byte
		var closeFn func()
		if msgs, closeFn, err = dial(); err == nil {
			return msgs, closeFn, nil
		}
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(redialDelay << attempt):
		}
	}
	return nil, nil, err
}

func expire(events chan watch.Event, w *watch.ProxyWatcher, message string) {
	status := apierrors.NewResourceExpired(message)
	select {
	case events <- watch.Event{Type: watch.Error, Object: &status.ErrStatus}:
	case <-w.StopChan():
	}
}
