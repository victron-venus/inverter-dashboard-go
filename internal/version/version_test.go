package version

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGetCurrent(t *testing.T) {
	// GetCurrent should return a non-empty string (either from VERSION or "dev")
	if GetCurrent() == "" {
		t.Error("GetCurrent() returned empty string")
	}
}

func TestCheckLatest(t *testing.T) {
	// Mock http.Do
	oldClient := http.DefaultClient
	defer func() { http.DefaultClient = oldClient }()

	mockTransport := &mockRoundTripper{
		mockResp: &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte("2.0.0"))),
			Header:     make(http.Header),
		},
	}
	http.DefaultClient = &http.Client{Transport: mockTransport}

	latest, err := CheckLatest("https://example.com")
	if err != nil {
		t.Errorf("CheckLatest error: %v", err)
	}
	if latest != "2.0.0" {
		t.Errorf("CheckLatest() = %q, want \"2.0.0\"", latest)
	}
	// Ensure the request was made to the correct URL
	if !mockTransport.requested {
		t.Error("CheckLatest did not make a request")
	}
	if got, want := mockTransport.lastURL, "https://example.com/VERSION"; got != want {
		t.Errorf("CheckLatest requested %q, want %q", got, want)
	}
}

func TestCheckLatestError(t *testing.T) {
	oldClient := http.DefaultClient
	defer func() { http.DefaultClient = oldClient }()

	mockTransport := &mockRoundTripper{
		mockErr: fmt.Errorf("network error"),
	}
	http.DefaultClient = &http.Client{Transport: mockTransport}

	_, err := CheckLatest("https://example.com")
	if err == nil {
		t.Error("CheckLatest expected error, got nil")
	}
	if !strings.Contains(err.Error(), "network error") {
		t.Errorf("CheckLatest error = %v, want error containing \"network error\"", err)
	}
}

func TestCheckLatestNonOKStatus(t *testing.T) {
	oldClient := http.DefaultClient
	defer func() { http.DefaultClient = oldClient }()

	mockTransport := &mockRoundTripper{
		mockResp: &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewReader([]byte(""))),
			Header:     make(http.Header),
		},
	}
	http.DefaultClient = &http.Client{Transport: mockTransport}

	_, err := CheckLatest("https://example.com")
	if err == nil {
		t.Error("CheckLatest expected error for non-200 status, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected status code: 404") {
		t.Errorf("CheckLatest error = %v, want status error", err)
	}
}

func TestGetLatestCachedAndSetLatestCached(t *testing.T) {
	// Clear cache
	SetLatestCached("")
	if got := GetLatestCached(); got != "" {
		t.Errorf("GetLatestCached() = %q, want empty string", got)
	}
	SetLatestCached("3.0.0")
	if got := GetLatestCached(); got != "3.0.0" {
		t.Errorf("GetLatestCached() = %q, want \"3.0.0\"", got)
	}
}

type mockRoundTripper struct {
	mockResp *http.Response
	mockErr  error
	requested bool
	lastURL   string
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.requested = true
	m.lastURL = req.URL.String()
	if m.mockErr != nil {
		return nil, m.mockErr
	}
	return m.mockResp, nil
}
