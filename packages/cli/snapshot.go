package main

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"text/tabwriter"
)

func runSnapshot(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: k8flare snapshot save|list|restore")
	}
	switch args[0] {
	case "save":
		return snapshotSave(args[1:], out)
	case "list":
		return snapshotList(args[1:], out)
	case "restore":
		return snapshotRestore(args[1:], out)
	}
	return fmt.Errorf("unknown snapshot command %q", args[0])
}

func snapshotSave(args []string, out io.Writer) error {
	cmd := newCommand("snapshot save")
	if _, err := cmd.parse(args); err != nil {
		return err
	}
	c, err := cmd.conn.dial()
	if err != nil {
		return err
	}
	var saved struct {
		Key string `json:"key"`
	}
	if err := c.call("POST", "/internal/snapshots", nil, nil, &saved); err != nil {
		return err
	}
	fmt.Fprintln(out, saved.Key)
	return nil
}

func snapshotList(args []string, out io.Writer) error {
	cmd := newCommand("snapshot list")
	if _, err := cmd.parse(args); err != nil {
		return err
	}
	c, err := cmd.conn.dial()
	if err != nil {
		return err
	}
	var listed struct {
		Items []struct {
			Key      string `json:"key"`
			Size     int64  `json:"size"`
			Uploaded string `json:"uploaded"`
		} `json:"items"`
	}
	if err := c.call("GET", "/internal/snapshots", nil, nil, &listed); err != nil {
		return err
	}
	w := tabwriter.NewWriter(out, 10, 4, 3, ' ', 0)
	fmt.Fprintf(w, "KEY\tSIZE\tUPLOADED\n")
	for _, s := range listed.Items {
		fmt.Fprintf(w, "%s\t%d\t%s\n", s.Key, s.Size, s.Uploaded)
	}
	return w.Flush()
}

func snapshotRestore(args []string, out io.Writer) error {
	cmd := newCommand("snapshot restore")
	force := cmd.fs.Bool("force", false, "replace the contents of a cluster that already has data")
	keys, err := cmd.parse(args)
	if err != nil {
		return err
	}
	if len(keys) != 1 {
		return errors.New("usage: k8flare snapshot restore [--force] <snapshot-key>")
	}
	c, err := cmd.conn.dial()
	if err != nil {
		return err
	}
	var restored struct {
		Written int `json:"written"`
		Removed int `json:"removed"`
	}
	if err := c.call("POST", "/internal/snapshots/restore", nil, map[string]any{"key": keys[0], "force": *force}, &restored); err != nil {
		return err
	}
	fmt.Fprintf(out, "restored %s: %d keys written, %d keys removed\n", keys[0], restored.Written, restored.Removed)
	return nil
}

func runRestore(args []string, out io.Writer) error {
	cmd := newCommand("restore")
	to := cmd.fs.String("to", "", "point in time to restore to, RFC 3339 or a Unix timestamp")
	if _, err := cmd.parse(args); err != nil {
		return err
	}
	if *to == "" {
		return errors.New("usage: k8flare restore --to <time>")
	}
	c, err := cmd.conn.dial()
	if err != nil {
		return err
	}
	var restored struct {
		To string `json:"to"`
	}
	if err := c.call("POST", "/internal/restore", url.Values{"to": {*to}}, nil, &restored); err != nil {
		return err
	}
	fmt.Fprintf(out, "cluster state restored to %s; the datastore restarts and is briefly unavailable\n", restored.To)
	return nil
}
