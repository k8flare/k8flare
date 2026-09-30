package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"
)

type tokenRecord struct {
	ID          string     `json:"id"`
	Description string     `json:"description"`
	ExpiresAt   *time.Time `json:"expiresAt"`
	Token       string     `json:"token"`
}

func runToken(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: k8flare token create|list|delete|rotate")
	}
	switch args[0] {
	case "create":
		return tokenCreate(args[1:], out)
	case "list":
		return tokenList(args[1:], out)
	case "delete":
		return tokenDelete(args[1:], out)
	case "rotate":
		return tokenRotate(args[1:], out)
	}
	return fmt.Errorf("unknown token command %q", args[0])
}

func tokenCreate(args []string, out io.Writer) error {
	cmd := newCommand("token create")
	ttl := cmd.fs.Duration("ttl", 24*time.Hour, "time before the token expires; 0 never expires")
	description := cmd.fs.String("description", "", "what the token is for")
	if _, err := cmd.parse(args); err != nil {
		return err
	}
	if *ttl < 0 {
		return errors.New("--ttl must not be negative")
	}
	c, err := cmd.conn.dial()
	if err != nil {
		return err
	}
	caHash, err := c.serverCAHash()
	if err != nil {
		return err
	}
	var created tokenRecord
	body := map[string]any{"description": *description, "ttlSeconds": int64(ttl.Seconds())}
	if err := c.call("POST", "/internal/tokens", nil, body, &created); err != nil {
		return err
	}
	fmt.Fprintln(out, k10Token(caHash, created.Token))
	return nil
}

func tokenList(args []string, out io.Writer) error {
	cmd := newCommand("token list")
	if _, err := cmd.parse(args); err != nil {
		return err
	}
	c, err := cmd.conn.dial()
	if err != nil {
		return err
	}
	var listed struct {
		Items []tokenRecord `json:"items"`
	}
	if err := c.call("GET", "/internal/tokens", nil, nil, &listed); err != nil {
		return err
	}
	w := tabwriter.NewWriter(out, 10, 4, 3, ' ', 0)
	fmt.Fprintf(w, "ID\tEXPIRES\tDESCRIPTION\n")
	for _, t := range listed.Items {
		expires := "<never>"
		if t.ExpiresAt != nil {
			expires = t.ExpiresAt.Format(time.RFC3339)
		}
		description := t.Description
		if description == "" {
			description = "<none>"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", t.ID, expires, description)
	}
	return w.Flush()
}

func (c *client) serverCAHash() (string, error) {
	pem, err := c.do("GET", "/cacerts", nil, nil)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(pem)
	return hex.EncodeToString(sum[:]), nil
}

func k10Token(caHash, token string) string {
	return "K10" + caHash + "::" + token
}

func tokenID(arg string) string {
	id, _, _ := strings.Cut(arg, ".")
	return id
}

func tokenDelete(args []string, out io.Writer) error {
	cmd := newCommand("token delete")
	ids, err := cmd.parse(args)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return errors.New("usage: k8flare token delete <id|token>...")
	}
	c, err := cmd.conn.dial()
	if err != nil {
		return err
	}
	for _, arg := range ids {
		id := tokenID(arg)
		if err := c.call("DELETE", "/internal/tokens/"+id, nil, nil, nil); err != nil {
			return err
		}
		fmt.Fprintf(out, "token %s deleted\n", id)
	}
	return nil
}

func tokenRotate(args []string, out io.Writer) error {
	cmd := newCommand("token rotate")
	ids, err := cmd.parse(args)
	if err != nil {
		return err
	}
	if len(ids) != 1 {
		return errors.New("usage: k8flare token rotate <id|token>")
	}
	c, err := cmd.conn.dial()
	if err != nil {
		return err
	}
	caHash, err := c.serverCAHash()
	if err != nil {
		return err
	}
	var rotated tokenRecord
	if err := c.call("POST", "/internal/tokens/"+tokenID(ids[0])+"/rotate", nil, nil, &rotated); err != nil {
		return err
	}
	fmt.Fprintln(out, k10Token(caHash, rotated.Token))
	return nil
}
