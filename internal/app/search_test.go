package app

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/follenfang/wowdoc/internal/query"
)

func TestQueryAndExploreHaveDifferentRecall(t *testing.T) {
	t.Setenv("WOWDOC_HOME", t.TempDir())
	buildValidationSnapshot(t, "retail", "12.1.0", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "120100", "KnownAPI", "number")
	for _, command := range []string{"query", "explore"} {
		var out, stderr bytes.Buffer
		code := RunWowdoc([]string{command, "--source", "wow-ui-source", "--product", "retail", "--ref", "12.1.0", "--topic", "api", "--text", "number nonexistent"}, &out, &stderr)
		if code != 0 {
			t.Fatalf("%s: %s %s", command, out.String(), stderr.String())
		}
		var envelope struct {
			Data query.Response `json:"data"`
		}
		if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if (len(envelope.Data.Results) == 0) != (command == "query") {
			t.Fatalf("%s: %+v", command, envelope.Data.Results)
		}
	}
	var out, stderr bytes.Buffer
	code := RunWowdoc([]string{"query", "--source", "wow-ui-source", "--product", "retail", "--ref", "12.1.0", "--topic", "typo", "--text", "KnownAPI"}, &out, &stderr)
	if code != 2 || !bytes.Contains(out.Bytes(), []byte(`"code":"invalid_topic"`)) {
		t.Fatalf("code=%d %s", code, out.String())
	}
}
