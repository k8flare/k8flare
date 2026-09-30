package main

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

type secretsStatusResponse struct {
	Enabled      bool     `json:"enabled"`
	ActiveKey    string   `json:"activeKey"`
	InactiveKeys []string `json:"inactiveKeys"`
	Total        int      `json:"total"`
	Current      int      `json:"current"`
	Stale        int      `json:"stale"`
}

func runSecretsEncrypt(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: k8flare secrets-encrypt status|reencrypt")
	}
	switch args[0] {
	case "status":
		return secretsStatus(args[1:], out)
	case "reencrypt":
		return secretsReencrypt(args[1:], out)
	}
	return fmt.Errorf("unknown secrets-encrypt command %q", args[0])
}

func secretsStatus(args []string, out io.Writer) error {
	cmd := newCommand("secrets-encrypt status")
	if _, err := cmd.parse(args); err != nil {
		return err
	}
	c, err := cmd.conn.dial()
	if err != nil {
		return err
	}
	var status secretsStatusResponse
	if err := c.call("GET", "/internal/secrets-encrypt/status", nil, nil, &status); err != nil {
		return err
	}
	if !status.Enabled {
		fmt.Fprintln(out, "Encryption Status: Disabled")
	} else {
		fmt.Fprintln(out, "Encryption Status: Enabled")
		fmt.Fprintf(out, "Active Key: %s\n", status.ActiveKey)
		if len(status.InactiveKeys) > 0 {
			fmt.Fprintf(out, "Inactive Keys: %s\n", strings.Join(status.InactiveKeys, ", "))
		}
	}
	fmt.Fprintf(out, "Secrets: %d total, %d current, %d stale\n", status.Total, status.Current, status.Stale)
	return nil
}

func secretsReencrypt(args []string, out io.Writer) error {
	cmd := newCommand("secrets-encrypt reencrypt")
	if _, err := cmd.parse(args); err != nil {
		return err
	}
	c, err := cmd.conn.dial()
	if err != nil {
		return err
	}
	var result struct {
		Rewritten int `json:"rewritten"`
	}
	if err := c.call("POST", "/internal/secrets-encrypt/reencrypt", nil, nil, &result); err != nil {
		return err
	}
	fmt.Fprintf(out, "%d secrets re-encrypted with the active key\n", result.Rewritten)
	return nil
}
