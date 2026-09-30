package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type edgeCertificate struct {
	Cert     string `json:"cert"`
	Key      string `json:"key"`
	ClientCA string `json:"clientCA"`
}

func runEdgeCertificate(args []string, out io.Writer) error {
	cmd := newCommand("edge-certificate")
	hosts := cmd.fs.String("hosts", "", "comma-separated DNS names and IPs the certificate covers")
	ttl := cmd.fs.Duration("ttl", 365*24*time.Hour, "certificate lifetime")
	dir := cmd.fs.String("out-dir", ".", "where edge.crt, edge.key and client-ca.crt are written")
	if _, err := cmd.parse(args); err != nil {
		return err
	}
	if *hosts == "" {
		return errors.New("--hosts is required")
	}
	if *ttl <= 0 {
		return errors.New("--ttl must be positive")
	}
	c, err := cmd.conn.dial()
	if err != nil {
		return err
	}
	var issued edgeCertificate
	body := map[string]any{"hosts": strings.Split(*hosts, ","), "ttlSeconds": int64(ttl.Seconds())}
	if err := c.call("POST", "/internal/edge-certificate", nil, body, &issued); err != nil {
		return err
	}
	for _, f := range []struct {
		name, content string
		mode          os.FileMode
	}{
		{"edge.crt", issued.Cert, 0o644},
		{"edge.key", issued.Key, 0o600},
		{"client-ca.crt", issued.ClientCA, 0o644},
	} {
		path := filepath.Join(*dir, f.name)
		if err := os.WriteFile(path, []byte(f.content), f.mode); err != nil {
			return err
		}
		fmt.Fprintln(out, path)
	}
	return nil
}
