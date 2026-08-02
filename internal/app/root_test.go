package app

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestMachineReadableNotInitializedError(t *testing.T) {
	t.Setenv("WOWDOC_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := RunWowdoc([]string{"query", "--source", "elvui", "--product", "main", "--text", "E:Initialize"}, &stdout, &stderr)
	if exit != 3 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string   `json:"code"`
			Next []string `json:"nextSteps"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.OK || envelope.Error.Code != "not_initialized" || len(envelope.Error.Next) == 0 {
		t.Fatalf("unexpected envelope: %#v", envelope)
	}
}
