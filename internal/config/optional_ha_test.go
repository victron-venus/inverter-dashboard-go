package config

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestHAConfigurationLoggingAcceptsEmptyOrShortToken(t *testing.T) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(previous)
	for _, token := range []string{"", "x", "secret-token-value"} {
		logHomeAssistantConfig(&HomeAssistantConfig{Token: token})
	}
	if strings.Contains(output.String(), "secret-token") {
		t.Fatal("credential leaked in configuration log")
	}
}
