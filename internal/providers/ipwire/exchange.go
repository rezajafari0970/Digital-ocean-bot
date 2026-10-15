package ipwire

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

// Reply contains transient bytes only. Never persist/log it; DecodeReply returns
// sanitized evidence. Complete is false for any transport/read/size failure.
type Reply struct {
	Status   int
	Body     []byte
	Header   http.Header
	Complete bool
}

// Exchange is a low-level one-call HTTP primitive, not account or paid authority.
// The integration must supply an already fenced, non-retrying account transport
// and its bound retained token. There is intentionally no default transport,
// cookie jar, redirect handling, retry loop, environment routing or GetBody.
// No production caller uses this primitive until that account adapter is accepted.
func Exchange(ctx context.Context, p Prepared, base http.RoundTripper, token []byte) Reply {
	if base == nil || len(token) == 0 || len(token) > 16384 || strings.ContainsAny(string(token), "\r\n") {
		return Reply{}
	}
	canonical, e := Prepare(p.Spec())
	if e != nil || canonical.Digest() != p.Digest() {
		return Reply{}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// NopCloser prevents NewRequest from synthesizing a replayable GetBody.
	body := p.Body()
	req, e := http.NewRequestWithContext(ctx, p.Method(), p.URL(), io.NopCloser(bytes.NewReader(body)))
	if e != nil {
		return Reply{}
	}
	req.ContentLength = int64(len(body))
	req.Header.Set("Authorization", "Bearer "+string(token))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	response, e := base.RoundTrip(req)
	// Do not return transport errors: they may include URLs, credentials or proxy data.
	if response == nil {
		return Reply{}
	}
	if response.Body != nil {
		defer response.Body.Close()
	}
	out := Reply{Status: response.StatusCode, Header: make(http.Header)}
	if out.Status < 100 || out.Status > 599 {
		out.Status = 0
	}
	// Only bounded Retry-After is relevant; never retain provider headers/cookies.
	if retry := response.Header.Get("Retry-After"); len(retry) <= 128 {
		out.Header.Set("Retry-After", retry)
	}
	if e != nil || response.Body == nil {
		return out
	}
	out.Body, e = io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	out.Complete = e == nil && len(out.Body) <= 1<<20
	return out
}
func (p Prepared) DecodeReply(reply Reply, now time.Time) Result {
	status := reply.Status
	if status < 0 || status > 599 {
		status = 0
	}
	// Decode still retains independently validated candidate hints and cooldown.
	out := p.Decode(status, reply.Body, reply.Header, now)
	if !reply.Complete {
		out.Outcome = "UNKNOWN"
		out.ResourceID = ""
		out.Address = ""
		out.Prefix = ""
		out.ServerID = ""
		out.Region = ""
		out.Contract = ""
		if status != 429 {
			out.ErrorClass = "NETWORK"
		}
	}
	return out
}
