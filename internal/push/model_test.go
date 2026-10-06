package push

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestEventKeyCompactUTF8Vectors(t *testing.T) {
	// Expected values generated independently with Python hashlib over
	// json.dumps(array,ensure_ascii=False,separators=(',',':')).encode('utf-8').
	vectors := []struct{ id, want string }{
		{"slot", "ea205cdf3e8f1325f787d44f733e38d5585167f1e33022ebacfc04eb1976440f"},
		{"<>&é😀", "1e7d609845453c100d3ed333ee7d1e9fd8d336bf68c0005f886ad40b0f9b4535"},
		{"x y z", "f6f31108ee2e8278af79179416bfdf8554d523094ecd8ad09e932445dee82f98"},
		{"literal\\u2028", "49672d38d0e187cb463ecec2db7fb5e5bd900dccfe5fb4f5debd14e712d48526"},
	}
	for _, v := range vectors {
		if got := eventKey("native", "victron", v.id, 1791226800000); got != v.want {
			t.Fatal(v.id, got, v.want)
		}
	}
}
func TestPayloadIsBoundedIncludingMultibyteAndHTMLEscapes(t *testing.T) {
	for _, body := range []string{strings.Repeat("😀", 1000), strings.Repeat("<", 1000), strings.Repeat("a", 2000)} {
		p := makePayload("native", "victron", "slot", strings.Repeat("é", 200), body, time.Now(), time.Now())
		raw, _ := json.Marshal(p)
		if len(raw) > maxPayloadBytes || len([]rune(p.Title)) > 120 || len([]rune(p.Body)) > 1000 {
			t.Fatal("payload bounds")
		}
	}
}
