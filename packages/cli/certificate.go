package main

import (
	"errors"
	"fmt"
	"io"
	"text/tabwriter"
	"time"
)

const certificateRenewWindow = 90 * 24 * time.Hour

type caStatus struct {
	Name     string    `json:"name"`
	Subject  string    `json:"subject"`
	NotAfter time.Time `json:"notAfter"`
	Expired  bool      `json:"expired"`
}

func runCertificate(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: k8flare certificate rotate-ca|check")
	}
	switch args[0] {
	case "rotate-ca":
		return certificateCall("certificate rotate-ca", "POST", "/internal/certificate/rotate-ca", args[1:], out)
	case "check":
		return certificateCall("certificate check", "GET", "/internal/certificate/check", args[1:], out)
	}
	return fmt.Errorf("unknown certificate command %q", args[0])
}

func certificateCall(name, method, path string, args []string, out io.Writer) error {
	cmd := newCommand(name)
	if _, err := cmd.parse(args); err != nil {
		return err
	}
	c, err := cmd.conn.dial()
	if err != nil {
		return err
	}
	var listed struct {
		Items []caStatus `json:"items"`
	}
	if err := c.call(method, path, nil, nil, &listed); err != nil {
		return err
	}
	w := tabwriter.NewWriter(out, 10, 4, 3, ' ', 0)
	fmt.Fprintf(w, "CA\tSUBJECT\tEXPIRES\tSTATUS\n")
	for _, s := range listed.Items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.Name, s.Subject, s.NotAfter.Format(time.RFC3339), s.status())
	}
	return w.Flush()
}

func (s caStatus) status() string {
	switch {
	case s.Expired:
		return "expired"
	case time.Until(s.NotAfter) < certificateRenewWindow:
		return "expiring"
	}
	return "ok"
}
