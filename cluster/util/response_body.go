package util

import (
	"io"
)

// maxDrainBytes bounds how much of an unread response body DrainAndClose will
// consume. Draining lets the transport reuse the connection; past this size it
// is cheaper to let the connection close.
const maxDrainBytes = 64 << 10

// DrainAndClose discards any unread part of an HTTP response body and closes
// it. Every response returned by ServiceClient.DoRequest must be passed here.
//
// The operator clients share a transport with MaxConnsPerHost set to 2, so a
// body that is left unclosed holds one of only two connections for the life of
// the process. Two such responses and every later request to that operator
// blocks forever waiting for a connection.
func DrainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(body, maxDrainBytes))
	_ = body.Close()
}
