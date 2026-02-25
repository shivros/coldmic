package main

import (
	"errors"
	"net"
	"net/url"

	coldcli "coldmic/internal/cli"
)

func mapErrorToExitCode(err error) int {
	var httpErr coldcli.HTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.StatusCode {
		case 404:
			return exitNotFound
		case 409:
			return exitConflict
		default:
			return exitGeneric
		}
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return exitDaemonOffline
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return exitDaemonOffline
	}

	return exitGeneric
}
