package main

import (
	"errors"
	"net"
	"net/url"
	"testing"

	coldcli "coldmic/internal/cli"
)

func TestMapErrorToExitCode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want int
	}{
		{name: "http not found", err: coldcli.HTTPError{StatusCode: 404}, want: exitNotFound},
		{name: "http conflict", err: coldcli.HTTPError{StatusCode: 409}, want: exitConflict},
		{name: "http other", err: coldcli.HTTPError{StatusCode: 500}, want: exitGeneric},
		{name: "url error", err: &url.Error{Op: "Get", URL: "http://127.0.0.1:4317", Err: errors.New("refused")}, want: exitDaemonOffline},
		{name: "op error", err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("refused")}, want: exitDaemonOffline},
		{name: "generic", err: errors.New("boom"), want: exitGeneric},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := mapErrorToExitCode(tc.err)
			if got != tc.want {
				t.Fatalf("got %d want %d", got, tc.want)
			}
		})
	}
}
