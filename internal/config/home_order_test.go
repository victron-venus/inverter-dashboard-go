package config

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestHomeConfigurationRetainsDeclarationOrderAndDisabledEntries(t *testing.T) {
	var document struct {
		Switches yaml.Node `yaml:"switch_entities"`
	}
	err := yaml.Unmarshal([]byte(`switch_entities:
  home_z: {entity: switch.example_z, name: Laundry guard, order: 1}
  home_a: [light.example_a, Example A]
  home_disabled: {entity: switch.disabled, enabled: false}
  home_empty: {label: Missing entity}
`), &document)
	if err != nil {
		t.Fatal(err)
	}
	got, err := switchEntitiesFromYAML(document.Switches)
	if err != nil {
		t.Fatal(err)
	}
	want := []EntityConfig{
		{Key: "home_z", Entity: "switch.example_z", Label: "Laundry guard", Order: 1},
		{Key: "home_a", Entity: "light.example_a", Label: "Example A"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Home configuration = %#v; want %#v", got, want)
	}
}

func TestHomeConfigurationRejectsNonMapping(t *testing.T) {
	_, err := switchEntitiesFromYAML(yaml.Node{Kind: yaml.SequenceNode})
	if err == nil {
		t.Fatal("non-mapping Home list accepted")
	}
}

func TestHomeConfigurationPreservesYAMLEmptyAndAliasForms(t *testing.T) {
	for _, source := range []string{"switch_entities:", "switch_entities: null", "switch_entities: {}"} {
		var doc struct {
			Switches yaml.Node `yaml:"switch_entities"`
		}
		if err := yaml.Unmarshal([]byte(source), &doc); err != nil {
			t.Fatal(err)
		}
		got, err := switchEntitiesFromYAML(doc.Switches)
		if err != nil || len(got) != 0 {
			t.Fatalf("empty list = %v, %v", got, err)
		}
	}
	var doc struct {
		Switches yaml.Node `yaml:"switch_entities"`
	}
	if err := yaml.Unmarshal([]byte("entries: &home\n  home_z: switch.z\n  home_a: light.a\nswitch_entities: *home\n"), &doc); err != nil {
		t.Fatal(err)
	}
	got, err := switchEntitiesFromYAML(doc.Switches)
	if err != nil || len(got) != 2 || got[0].Key != "home_z" || got[1].Key != "home_a" {
		t.Fatalf("alias list = %v, %v", got, err)
	}
}
