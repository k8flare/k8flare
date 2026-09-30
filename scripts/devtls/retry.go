package main

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net/http"
)

const (
	droppedByProxyWorker = "Error: Network connection lost."
	droppedRetries       = 2
	replayableBody       = 16 << 20
)

type retryDropped struct {
	base http.RoundTripper
}

type replayBody struct {
	io.Reader
	io.Closer
}

func (t retryDropped) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return t.send(req, nil)
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, replayableBody+1))
	if err != nil {
		req.Body.Close()
		return nil, err
	}
	if len(body) > replayableBody {
		once := req.Clone(req.Context())
		once.Body = replayBody{io.MultiReader(bytes.NewReader(body), req.Body), req.Body}
		return t.base.RoundTrip(once)
	}
	req.Body.Close()
	return t.send(req, body)
}

func (t retryDropped) send(req *http.Request, body []byte) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		try := req.Clone(req.Context())
		if body != nil {
			try.Body = io.NopCloser(bytes.NewReader(body))
			try.ContentLength = int64(len(body))
		}
		resp, err := t.base.RoundTrip(try)
		if errors.Is(err, io.EOF) && attempt < droppedRetries {
			log.Printf("devtls: %s %s lost its connection before wrangler dev answered, retrying", req.Method, req.URL.Path)
			continue
		}
		if err != nil || resp.StatusCode != http.StatusInternalServerError || attempt == droppedRetries {
			return resp, err
		}
		head := make([]byte, len(droppedByProxyWorker))
		n, _ := io.ReadFull(resp.Body, head)
		if string(head[:n]) != droppedByProxyWorker {
			resp.Body = replayBody{io.MultiReader(bytes.NewReader(head[:n]), resp.Body), resp.Body}
			return resp, nil
		}
		resp.Body.Close()
		log.Printf("devtls: %s %s lost its connection inside wrangler dev, retrying", req.Method, req.URL.Path)
	}
}
