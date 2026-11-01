package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestK3SBasePreservesOperatorSecrets(t *testing.T) {
	root := filepath.Join("deploy", "k3s")
	data, err := os.ReadFile(filepath.Join(root, "kustomization.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var base struct {
		Resources       []string         `yaml:"resources"`
		SecretGenerator []map[string]any `yaml:"secretGenerator"`
	}
	if err := yaml.Unmarshal(data, &base); err != nil {
		t.Fatal(err)
	}
	if len(base.Resources) == 0 || len(base.SecretGenerator) != 0 {
		t.Fatal("public base must declare workloads without generating credentials")
	}
	for _, resource := range base.Resources {
		data, err := os.ReadFile(filepath.Join(root, resource))
		if err != nil {
			t.Fatal(err)
		}
		decoder := yaml.NewDecoder(strings.NewReader(string(data)))
		for {
			var document struct {
				Kind string `yaml:"kind"`
			}
			if err := decoder.Decode(&document); err == io.EOF {
				break
			} else if err != nil {
				t.Fatalf("%s: %v", resource, err)
			}
			if document.Kind == "Secret" {
				t.Fatalf("public base includes %s; applying it can overwrite operator credentials", resource)
			}
		}
	}
	data, err = os.ReadFile(filepath.Join(root, "03-deployment.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"inverter-dashboard-go-gateway", "inverter-dashboard-go-config"} {
		if !strings.Contains(string(data), name) {
			t.Fatalf("operator-managed Secret reference %q was removed", name)
		}
	}
}
