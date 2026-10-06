package homeassistant

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/victron-venus/inverter-dashboard-go/internal/config"
)

type serviceCleanupBody struct {
	io.Reader
	closed bool
}

func (b *serviceCleanupBody) Close() error {
	b.closed = true
	return errors.New("response cleanup failed")
}

func TestServiceCleanupPreservesCommandResult(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusServiceUnavailable} {
		body := &serviceCleanupBody{Reader: strings.NewReader("")}
		c := NewClient(&config.HomeAssistantConfig{URL: "http://ha.test", Token: "test", DirectControls: true})
		c.entityObserved = map[string]time.Time{"light.test": time.Now()}
		requests := 0
		c.httpClient = &http.Client{Transport: ownershipTransport(func(_ *http.Request) (*http.Response, error) {
			requests++
			return &http.Response{StatusCode: status, Body: body}, nil
		})}
		err := c.callServiceData("light", "turn_on", "light.test", nil)
		if status == http.StatusOK && err != nil {
			t.Fatalf("accepted command became a failure after cleanup: %v", err)
		}
		if status != http.StatusOK && (err == nil || !strings.Contains(err.Error(), "HTTP 503")) {
			t.Fatalf("cleanup replaced the command status: %v", err)
		}
		if !body.closed || requests != 1 {
			t.Fatalf("closed=%v requests=%d, want closed response and no retry", body.closed, requests)
		}
	}
}
