package homeassistant

import (
 "testing"
)

func TestParseSwitchEntitiesWithStringArrays(t *testing.T) {
 // Test parsing map[string][]string that comes from YAML config
 input := map[string]interface{}{
 "home_recliner": []string{"switch.recliner_recliner", "Recliner"},
 "home_garage": []string{"switch.garage_opener_l", "Garage"},
 }

 result := parseSwitchEntities(input)

 if len(result) != 2 {
 t.Errorf("Expected 2 buttons, got %d", len(result))
 }

 recliner, ok := result["home_recliner"]
 if !ok {
 t.Fatal("home_recliner not found in result")
 }
 if recliner.Entity != "switch.recliner_recliner" {
 t.Errorf("Expected entity 'switch.recliner_recliner', got %s", recliner.Entity)
 }
 if recliner.Label != "Recliner" {
 t.Errorf("Expected label 'Recliner', got %s", recliner.Label)
 }
}
