package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type observedBody struct {
	reader   io.Reader
	closed   bool
	closeErr error
}

func (b *observedBody) Read(p []byte) (int, error) { return b.reader.Read(p) }

func (b *observedBody) Close() error {
	b.closed = true
	return b.closeErr
}

type repeatedByteReader struct{ remaining int64 }

func (r *repeatedByteReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := int64(len(p))
	if n > r.remaining {
		n = r.remaining
	}
	for i := int64(0); i < n; i++ {
		p[i] = 'x'
	}
	r.remaining -= n
	return int(n), nil
}

func response(status int, header http.Header, body io.ReadCloser, length int64) *http.Response {
	if header == nil {
		header = make(http.Header)
	}
	return &http.Response{
		StatusCode:    status,
		Header:        header,
		Body:          body,
		ContentLength: length,
	}
}

func clientFor(roundTrip roundTripFunc) *http.Client {
	return &http.Client{Transport: roundTrip}
}

func TestFetchArchiveRetriesOnlyTransientResponses(t *testing.T) {
	var calls int
	var delay []time.Duration
	var transientBodies []*observedBody
	client := clientFor(func(*http.Request) (*http.Response, error) {
		calls++
		if calls < 3 {
			body := &observedBody{reader: strings.NewReader("temporary")}
			transientBodies = append(transientBodies, body)
			status := http.StatusServiceUnavailable
			if calls == 2 {
				status = http.StatusTooManyRequests
			}
			header := make(http.Header)
			header.Set("Retry-After", "0")
			return response(status, header, body, -1), nil
		}
		return response(http.StatusOK, nil, io.NopCloser(strings.NewReader("archive")), 7), nil
	})

	data, err := fetchArchive(context.Background(), client, "https://example.test/archive", func(_ context.Context, d time.Duration) error {
		delay = append(delay, d)
		return nil
	})
	if err != nil {
		t.Fatalf("fetchArchive() error = %v", err)
	}
	if string(data) != "archive" {
		t.Fatalf("archive = %q, want archive", data)
	}
	if calls != 3 {
		t.Fatalf("requests = %d, want 3", calls)
	}
	if len(delay) != 2 || delay[0] != time.Second || delay[1] != 2*time.Second {
		t.Fatalf("retry waits = %v, want [1s 2s]", delay)
	}
	for i, body := range transientBodies {
		if !body.closed {
			t.Errorf("transient response body %d was not closed before retry", i+1)
		}
	}
}

func TestFetchArchiveDoesNotRetryPermanentStatus(t *testing.T) {
	var calls, waits int
	body := &observedBody{reader: strings.NewReader("not found")}
	client := clientFor(func(*http.Request) (*http.Response, error) {
		calls++
		return response(http.StatusNotFound, nil, body, -1), nil
	})
	_, err := fetchArchive(context.Background(), client, "https://example.test/archive", func(context.Context, time.Duration) error {
		waits++
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("fetchArchive() error = %v, want HTTP 404", err)
	}
	if calls != 1 || waits != 0 || !body.closed {
		t.Fatalf("calls=%d waits=%d closed=%t, want 1/0/true", calls, waits, body.closed)
	}
}

func TestFetchArchiveDoesNotRequestWithCanceledContext(t *testing.T) {
	var calls, waits int
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := clientFor(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("unexpected request")
	})
	_, err := fetchArchive(ctx, client, "https://example.test/archive", func(context.Context, time.Duration) error {
		waits++
		return nil
	})
	if !errors.Is(err, context.Canceled) || calls != 0 || waits != 0 {
		t.Fatalf("error=%v calls=%d waits=%d, want canceled and 0/0", err, calls, waits)
	}
}

func TestFetchArchiveReturnsStatusResponseCloseError(t *testing.T) {
	closeErr := errors.New("response close failed")
	body := &observedBody{reader: strings.NewReader("temporary"), closeErr: closeErr}
	var calls, waits int
	client := clientFor(func(*http.Request) (*http.Response, error) {
		calls++
		return response(http.StatusServiceUnavailable, nil, body, -1), nil
	})
	_, err := fetchArchive(context.Background(), client, "https://example.test/archive", func(context.Context, time.Duration) error {
		waits++
		return nil
	})
	if !errors.Is(err, closeErr) || calls != 1 || waits != 0 || !body.closed {
		t.Fatalf("error=%v calls=%d waits=%d closed=%t", err, calls, waits, body.closed)
	}
}

func TestFetchArchiveDoesNotRetryInvalidRetryAfter(t *testing.T) {
	body := &observedBody{reader: strings.NewReader("temporary")}
	header := make(http.Header)
	header.Set("Retry-After", "not a date")
	var calls, waits int
	client := clientFor(func(*http.Request) (*http.Response, error) {
		calls++
		return response(http.StatusServiceUnavailable, header, body, -1), nil
	})
	_, err := fetchArchive(context.Background(), client, "https://example.test/archive", func(context.Context, time.Duration) error {
		waits++
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "invalid Retry-After") || calls != 1 || waits != 0 || !body.closed {
		t.Fatalf("error=%v calls=%d waits=%d closed=%t", err, calls, waits, body.closed)
	}
}

func TestFetchArchiveDoesNotRetryTransportError(t *testing.T) {
	var calls, waits int
	transportErr := errors.New("connection reset")
	client := clientFor(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, transportErr
	})
	_, err := fetchArchive(context.Background(), client, "https://example.test/archive", func(context.Context, time.Duration) error {
		waits++
		return nil
	})
	if !errors.Is(err, transportErr) {
		t.Fatalf("fetchArchive() error = %v, want transport error", err)
	}
	if calls != 1 || waits != 0 {
		t.Fatalf("calls=%d waits=%d, want 1/0", calls, waits)
	}
}

func TestFetchArchiveStopsAfterThirdTransientResponse(t *testing.T) {
	var calls, waits int
	client := clientFor(func(*http.Request) (*http.Response, error) {
		calls++
		return response(http.StatusBadGateway, nil, io.NopCloser(strings.NewReader("temporary")), -1), nil
	})
	_, err := fetchArchive(context.Background(), client, "https://example.test/archive", func(context.Context, time.Duration) error {
		waits++
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("fetchArchive() error = %v, want terminal HTTP 502", err)
	}
	if calls != 3 || waits != 2 {
		t.Fatalf("calls=%d waits=%d, want 3/2", calls, waits)
	}
}

func TestFetchArchiveReturnsBodyReadErrorWithoutRetry(t *testing.T) {
	readErr := errors.New("body read failed")
	body := &observedBody{reader: errorReader{err: readErr}}
	var calls, waits int
	client := clientFor(func(*http.Request) (*http.Response, error) {
		calls++
		return response(http.StatusOK, nil, body, -1), nil
	})
	_, err := fetchArchive(context.Background(), client, "https://example.test/archive", func(context.Context, time.Duration) error {
		waits++
		return nil
	})
	if !errors.Is(err, readErr) {
		t.Fatalf("fetchArchive() error = %v, want body error", err)
	}
	if calls != 1 || waits != 0 || !body.closed {
		t.Fatalf("calls=%d waits=%d closed=%t, want 1/0/true", calls, waits, body.closed)
	}
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }

func TestRetryDelayParsesRetryAfterAndEnforcesLimit(t *testing.T) {
	now := time.Date(2026, time.November, 19, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		header  string
		minimum time.Duration
		want    time.Duration
		wantErr bool
	}{
		{name: "missing uses exponential minimum", minimum: 2 * time.Second, want: 2 * time.Second},
		{name: "delta seconds", header: "7", minimum: time.Second, want: 7 * time.Second},
		{name: "delta seconds honors minimum", header: "1", minimum: 2 * time.Second, want: 2 * time.Second},
		{name: "http date", header: now.Add(12 * time.Second).Format(http.TimeFormat), minimum: time.Second, want: 12 * time.Second},
		{name: "past date honors minimum", header: now.Add(-time.Minute).Format(http.TimeFormat), minimum: 3 * time.Second, want: 3 * time.Second},
		{name: "sixty seconds allowed", header: "60", minimum: time.Second, want: 60 * time.Second},
		{name: "over deadline rejected", header: "61", minimum: time.Second, wantErr: true},
		{name: "malformed rejected", header: "later", minimum: time.Second, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := retryDelay(tt.header, now, tt.minimum)
			if tt.wantErr {
				if err == nil {
					t.Fatal("retryDelay() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("retryDelay() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("retryDelay() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFetchArchiveStopsWhenRetryWaitHitsDeadline(t *testing.T) {
	body := &observedBody{reader: strings.NewReader("temporary")}
	var calls int
	client := clientFor(func(*http.Request) (*http.Response, error) {
		calls++
		return response(http.StatusServiceUnavailable, nil, body, -1), nil
	})
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Minute))
	defer cancel()
	_, err := fetchArchive(ctx, client, "https://example.test/archive", func(context.Context, time.Duration) error {
		return context.DeadlineExceeded
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("fetchArchive() error = %v, want deadline exceeded", err)
	}
	if calls != 1 || !body.closed {
		t.Fatalf("calls=%d closed=%t, want 1/true", calls, body.closed)
	}
}

func TestReadArchiveClosesBodiesAndEnforcesSize(t *testing.T) {
	t.Run("successful body", func(t *testing.T) {
		body := &observedBody{reader: strings.NewReader("archive")}
		data, err := readArchive(context.Background(), response(http.StatusOK, nil, body, 7))
		if err != nil || string(data) != "archive" {
			t.Fatalf("readArchive() = %q, %v", data, err)
		}
		if !body.closed {
			t.Fatal("successful response body was not closed")
		}
	})

	t.Run("declared oversize closes without reading", func(t *testing.T) {
		body := &observedBody{reader: strings.NewReader("unused")}
		_, err := readArchive(context.Background(), response(http.StatusOK, nil, body, maxArchive+1))
		if err == nil || !strings.Contains(err.Error(), "size limit") || !body.closed {
			t.Fatalf("readArchive() error=%v closed=%t", err, body.closed)
		}
	})

	t.Run("streamed oversize closes", func(t *testing.T) {
		body := &observedBody{reader: &repeatedByteReader{remaining: maxArchive + 1}}
		_, err := readArchive(context.Background(), response(http.StatusOK, nil, body, -1))
		if err == nil || !strings.Contains(err.Error(), "size limit") || !body.closed {
			t.Fatalf("readArchive() error=%v closed=%t", err, body.closed)
		}
	})

	t.Run("close error returned", func(t *testing.T) {
		closeErr := errors.New("body close failed")
		body := &observedBody{reader: strings.NewReader("archive"), closeErr: closeErr}
		_, err := readArchive(context.Background(), response(http.StatusOK, nil, body, 7))
		if !errors.Is(err, closeErr) || !body.closed {
			t.Fatalf("readArchive() error=%v closed=%t, want close error", err, body.closed)
		}
	})
}

func TestReadArchiveHonorsDeadlineAndCloses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	body := &observedBody{reader: cancelOnEOFReader{cancel: cancel}}
	_, err := readArchive(ctx, response(http.StatusOK, nil, body, -1))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("readArchive() error = %v, want canceled context", err)
	}
	if !body.closed {
		t.Fatal("response body was not closed after deadline/cancellation")
	}
}

type cancelOnEOFReader struct{ cancel context.CancelFunc }

func (r cancelOnEOFReader) Read([]byte) (int, error) {
	r.cancel()
	return 0, io.EOF
}

func TestDownloadClientKeepsTLSAndRedirectRestrictions(t *testing.T) {
	client := newDownloadClient()
	if client.Timeout != 60*time.Second {
		t.Fatalf("client timeout = %v, want 60s", client.Timeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.TLSClientConfig == nil || transport.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("transport TLS profile = %#v, want TLS 1.2 minimum", client.Transport)
	}
	if transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("TLS certificate verification must remain enabled")
	}

	request := func(raw string) *http.Request {
		t.Helper()
		parsed, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Request{URL: parsed}
	}
	previous := make([]*http.Request, 9)
	for _, tt := range []struct {
		name string
		url  string
		via  []*http.Request
		want bool
	}{
		{name: "https allowed", url: "https://example.test/path", via: previous},
		{name: "http downgrade denied", url: "http://example.test/path", via: previous, want: true},
		{name: "userinfo denied", url: "https://user@example.test/path", via: previous, want: true},
		{name: "eleventh hop denied", url: "https://example.test/path", via: append(append([]*http.Request(nil), previous...), request("https://example.test/one")), want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := client.CheckRedirect(request(tt.url), tt.via)
			if (err != nil) != tt.want {
				t.Fatalf("CheckRedirect() error = %v, want error %t", err, tt.want)
			}
		})
	}
}

func TestFetchArchiveRejectsMalformedURLBeforeRequest(t *testing.T) {
	var calls int
	client := clientFor(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, fmt.Errorf("unexpected request")
	})
	_, err := fetchArchive(context.Background(), client, "http://%zz", func(context.Context, time.Duration) error { return nil })
	if err == nil || calls != 0 {
		t.Fatalf("fetchArchive() error=%v calls=%d, want request construction error and no transport", err, calls)
	}
}

func TestWaitForRetryHonorsContextAndDeadline(t *testing.T) {
	t.Run("already canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := waitForRetry(ctx, time.Hour); !errors.Is(err, context.Canceled) {
			t.Fatalf("waitForRetry() error = %v, want canceled", err)
		}
	})

	t.Run("delay cannot fit deadline", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second))
		defer cancel()
		if err := waitForRetry(ctx, time.Minute); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("waitForRetry() error = %v, want deadline exceeded", err)
		}
	})

	t.Run("short delay completes", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := waitForRetry(ctx, time.Millisecond); err != nil {
			t.Fatalf("waitForRetry() error = %v, want nil", err)
		}
	})

	t.Run("cancellation interrupts timer", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		timer := time.AfterFunc(10*time.Millisecond, cancel)
		defer timer.Stop()
		if err := waitForRetry(ctx, time.Hour); !errors.Is(err, context.Canceled) {
			t.Fatalf("waitForRetry() error = %v, want canceled", err)
		}
	})
}
