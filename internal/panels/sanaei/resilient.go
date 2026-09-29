package sanaei

import (
	"context"
	"errors"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 4,
		BaseDelay:   120 * time.Millisecond,
		MaxDelay:    1500 * time.Millisecond,
	}
}

type ResilientSession struct {
	Client *APIClient
	Policy RetryPolicy

	loginMu sync.Mutex
}

func retryableStatus(code int) bool {
	switch code {
	case http.StatusRequestTimeout,
		http.StatusTooEarly,
		http.StatusTooManyRequests:
		return true
	}

	return code >= 500
}

func retryableError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	var ne net.Error

	if errors.As(err, &ne) {
		return true
	}

	msg := strings.ToLower(err.Error())

	return strings.Contains(msg, "eof") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection refused")
}

func retryableErrorWithContext(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	// A transport/client timeout is transient while the parent operation
	// still has budget. Parent cancellation/deadline always wins.
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return retryableError(err)
}

func (s *ResilientSession) login(
	ctx context.Context,
) error {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()

	return s.Client.Login(ctx)
}

func (s *ResilientSession) delay(
	ctx context.Context,
	attempt int,
) error {
	p := s.Policy

	if p.BaseDelay <= 0 {
		p.BaseDelay = 120 * time.Millisecond
	}

	if p.MaxDelay <= 0 {
		p.MaxDelay = 1500 * time.Millisecond
	}

	delay := p.BaseDelay

	for i := 1; i < attempt; i++ {
		delay *= 2

		if delay >= p.MaxDelay {
			delay = p.MaxDelay
			break
		}
	}

	jitterMax := delay / 4

	var jitter time.Duration

	if jitterMax > 0 {
		jitter = time.Duration(
			rand.Int63n(
				int64(jitterMax) + 1,
			),
		)
	}

	timer := time.NewTimer(
		delay + jitter,
	)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()

	case <-timer.C:
		return nil
	}
}

func (s *ResilientSession) Do(
	ctx context.Context,
	req SessionRequest,
) (SessionResponse, error) {
	if s == nil ||
		s.Client == nil ||
		s.Client.HTTP == nil {

		return SessionResponse{},
			ErrSessionRequest
	}

	policy := s.Policy

	if policy.MaxAttempts < 1 {
		policy = DefaultRetryPolicy()
	}

	var lastErr error

	relogged := false

	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {

		resp, err :=
			(DirectSessionExecutor{
				Client: s.Client,
			}).Do(
				ctx,
				req,
			)

		if err == nil &&
			resp.StatusCode >= 200 &&
			resp.StatusCode < 300 {

			return resp, nil
		}

		/*
		   Session expired.

		   One re-login is allowed.
		   It does not consume a retry attempt.
		*/
		if err == nil &&
			(resp.StatusCode ==
				http.StatusUnauthorized ||
				resp.StatusCode ==
					http.StatusForbidden) &&
			!relogged {

			if loginErr :=
				s.login(ctx); loginErr == nil {

				relogged = true
				attempt--
				continue
			} else {
				lastErr = loginErr
			}

		} else if err == nil &&
			!retryableStatus(
				resp.StatusCode,
			) {

			/*
			   Real client/validation error.

			   Do not waste time retrying it.
			*/
			return resp, nil

		} else if err != nil {

			if !retryableErrorWithContext(ctx, err) {
				return resp, err
			}

			lastErr = err

		} else {
			lastErr = ErrSessionRequest
		}

		if attempt ==
			policy.MaxAttempts {

			break
		}

		if err :=
			s.delay(
				ctx,
				attempt,
			); err != nil {

			return SessionResponse{},
				err
		}
	}

	if lastErr == nil {
		lastErr = ErrSessionRequest
	}

	return SessionResponse{},
		lastErr
}

func ReadAllBounded(
	reader io.Reader,
	max int64,
) ([]byte, error) {
	if max <= 0 {
		max = 64 << 20
	}

	limited :=
		io.LimitReader(
			reader,
			max+1,
		)

	data, err :=
		io.ReadAll(limited)

	if err != nil {
		return nil, err
	}

	if int64(len(data)) > max {
		return nil,
			ErrSessionRequest
	}

	return data, nil
}
