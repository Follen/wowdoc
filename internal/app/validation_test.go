package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/follenfang/wowdoc/internal/catalog"
	"github.com/follenfang/wowdoc/internal/home"
	"github.com/follenfang/wowdoc/internal/indexer"
	"github.com/follenfang/wowdoc/internal/store"
	"github.com/follenfang/wowdoc/internal/validator"
)

func TestValidateWithoutTOCKeepsRecursiveLuaBehavior(t *testing.T) {
	t.Setenv("WOWDOC_HOME", t.TempDir())
	addon := t.TempDir()
	writeValidationFile(t, addon, "Loaded.lua", "local valid = true\n")
	writeValidationFile(t, addon, "tests/Broken.lua", "local =\n")
	writeValidationFile(t, addon, "Addon.toc", "Loaded.lua\n")

	var stdout, stderr bytes.Buffer
	exit := RunWowdoc([]string{"validate", "--path", addon, "--source", "wow-ui-source", "--product", "retail", "--ref", "12.1.0"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			CheckedLua  int              `json:"checkedLua"`
			Valid       bool             `json:"valid"`
			Diagnostics []map[string]any `json:"diagnostics"`
			Ref         string           `json:"ref"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK || envelope.Data.CheckedLua != 2 || envelope.Data.Valid || len(envelope.Data.Diagnostics) != 1 || envelope.Data.Ref != "12.1.0" {
		t.Fatalf("unexpected envelope: %s", stdout.String())
	}
}

func TestValidateTOCUsesExactClosureAndStableEvidence(t *testing.T) {
	homeRoot := t.TempDir()
	t.Setenv("WOWDOC_HOME", homeRoot)
	buildValidationSnapshot(t, "retail", "12.1.0", "1111111111111111111111111111111111111111", "120100", "KnownAPI", "number")
	addon := t.TempDir()
	writeValidationFile(t, addon, "Addon.toc", "## Interface: 120100\nLayout.xml\n")
	writeValidationFile(t, addon, "Layout.xml", "<Ui>\n  <Script file=\"Code.lua\"/>\n</Ui>\n")
	writeValidationFile(t, addon, "Code.lua", "C_Test.KnownAPI(1)\nframe:RegisterEvent(\"KNOWN_EVENT\")\nframe:RegisterEvent(dynamicEvent)\n")
	writeValidationFile(t, addon, "tests/Broken.lua", "local =\n")

	var stdout, stderr bytes.Buffer
	exit := RunWowdoc([]string{"validate", "--path", addon, "--toc", "addon.TOC", "--source", "wow-ui-source", "--product", "retail", "--ref", "12.1.0"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	var envelope struct {
		OK   bool             `json:"ok"`
		Data validator.Result `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	got := envelope.Data
	if !envelope.OK || !got.Valid || got.CheckedLua != 1 || got.CheckedXML != 1 {
		t.Fatalf("unexpected result: %s", stdout.String())
	}
	if got.RequestedRef != "12.1.0" || got.MatchedTag != "12.1.0" || got.ResolvedCommit != "1111111111111111111111111111111111111111" {
		t.Fatalf("version evidence=%#v", got)
	}
	if got.Diagnostics == nil || len(got.Diagnostics) != 0 || got.Unresolved == nil || len(got.Unresolved) != 1 {
		t.Fatalf("diagnostics/unresolved=%#v/%#v", got.Diagnostics, got.Unresolved)
	}
	if len(got.LoadClosure) != 2 || got.LoadClosure[0].Path != "Layout.xml" || got.LoadClosure[0].LoadOrder != 0 || got.LoadClosure[1].Path != "Code.lua" || got.LoadClosure[1].LoadOrder != 1 {
		t.Fatalf("closure=%#v", got.LoadClosure)
	}
	if got.LoadClosure[1].LoadedBy[0].File != "Layout.xml" || got.LoadClosure[1].LoadedBy[0].Kind != "script" {
		t.Fatalf("loadedBy=%#v", got.LoadClosure[1].LoadedBy)
	}
	if len(got.Facts) < 3 || got.Coverage.Checked < 3 {
		t.Fatalf("facts/coverage=%#v/%#v", got.Facts, got.Coverage)
	}
}

func TestValidateMatrixMergesThreeClientTargets(t *testing.T) {
	t.Setenv("WOWDOC_HOME", t.TempDir())
	buildValidationSnapshot(t, "retail", "12.1.0", "2222222222222222222222222222222222222222", "120100", "KnownAPI", "number")
	buildValidationSnapshot(t, "classic", "5.5.4", "3333333333333333333333333333333333333333", "50504", "KnownAPI", "string")
	buildValidationSnapshot(t, "titan", "3.80.2", "4444444444444444444444444444444444444444", "30802", "OtherAPI", "number")

	root := t.TempDir()
	addon := filepath.Join(root, "Addon With Spaces")
	writeValidationFile(t, addon, "Shared.lua", "C_Test.KnownAPI(1)\n")
	writeValidationFile(t, addon, "Retail.lua", "local retail = true\n")
	writeValidationFile(t, addon, "Classic.lua", "local classic = true\n")
	writeValidationFile(t, addon, "Titan.lua", "local titan = true\n")
	writeValidationFile(t, addon, "Retail.toc", "## Interface: 120100\nShared.lua\nRetail.lua\n")
	writeValidationFile(t, addon, "Classic.toc", "## Interface: 50504\nShared.lua\nClassic.lua\n")
	writeValidationFile(t, addon, "Titan.toc", "## Interface: 30802\nShared.lua\nTitan.lua\n")
	config := `{"path":"Addon With Spaces","targets":[{"id":"retail","toc":"Retail.toc","product":"retail","ref":"12.1.0"},{"id":"classic","toc":"Classic.toc","product":"classic","ref":"5.5.4"},{"id":"titan","toc":"Titan.toc","product":"titan","ref":"3.80.2"}]}`
	configPath := filepath.Join(root, "wowdoc.matrix.json")
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	exit := RunWowdoc([]string{"validate-matrix", "--config", configPath}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	var envelope struct {
		OK   bool                   `json:"ok"`
		Data validator.MatrixResult `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	got := envelope.Data
	if !envelope.OK || got.Valid || len(got.Targets) != 3 {
		t.Fatalf("matrix=%s", stdout.String())
	}
	if got.Targets[0].ID != "retail" || got.Targets[1].ID != "classic" || got.Targets[2].ID != "titan" {
		t.Fatalf("target order=%#v", got.Targets)
	}
	if len(got.Summary.SharedFiles) != 1 || got.Summary.SharedFiles[0] != "Shared.lua" {
		t.Fatalf("shared files=%#v", got.Summary.SharedFiles)
	}
	if len(got.Summary.TargetOnlyFiles["retail"]) != 1 || got.Summary.TargetOnlyFiles["retail"][0] != "Retail.lua" {
		t.Fatalf("retail files=%#v", got.Summary.TargetOnlyFiles)
	}
	if len(got.Summary.APIs.Differences) != 1 || got.Summary.APIs.Differences[0].Name != "C_Test.KnownAPI" {
		t.Fatalf("API summary=%#v", got.Summary.APIs)
	}
	values := got.Summary.APIs.Differences[0].Targets
	if values["retail"].Signature == values["classic"].Signature || values["titan"].Exists {
		t.Fatalf("API values=%#v", values)
	}
	if len(got.Targets[2].Diagnostics) == 0 || got.Targets[2].Diagnostics[0].Code != "api_not_found" {
		t.Fatalf("titan diagnostics=%#v", got.Targets[2].Diagnostics)
	}
}

func buildValidationSnapshot(t *testing.T, product, tag, commit, interfaceValue, apiName, argumentType string) {
	t.Helper()
	layout, err := home.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if err = layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	fixture := t.TempDir()
	generated := `APIDocumentation:AddDocumentationTable({
  Name = "C_Test",
  Type = "System",
  Functions = {
    {
      Name = "` + apiName + `",
      Type = "Function",
      Arguments = {
        {Name = "value", Type = "` + argumentType + `", Nilable = false},
      },
    },
  },
  Events = {
    {
      Name = "KNOWN_EVENT",
      Type = "Event",
      Payload = {
        {Name = "payload", Type = "string", Nilable = false},
      },
    },
  },
})
`
	writeValidationFile(t, fixture, "Interface/AddOns/Blizzard_APIDocumentationGenerated/GeneratedDocumentation.lua", generated)
	writeValidationFile(t, fixture, "Client.toc", "## Interface: "+interfaceValue+"\n")
	stats, err := indexer.Build(context.Background(), indexer.BuildOptions{Layout: layout, SourceID: "wow-ui-source", ProductID: product, Commit: commit, RequestedRef: tag, Tag: tag, Input: indexer.DirectoryInput{Root: fixture}, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	cat, err := store.OpenCatalog(layout)
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	if err = cat.Seed(catalog.Sources()); err != nil {
		t.Fatal(err)
	}
	source, _ := catalog.FindSource("wow-ui-source")
	productConfig, ok := catalog.FindProduct(source, product)
	if !ok {
		t.Fatalf("unknown product fixture %s", product)
	}
	if err = cat.PublishRef("wow-ui-source", productConfig, commit, []store.TagRecord{{Name: tag, Commit: commit, CommittedAt: time.Now().Unix()}}); err != nil {
		t.Fatal(err)
	}
	if err = cat.SaveSnapshot(store.SnapshotRecord{ID: stats.SnapshotID, SourceID: "wow-ui-source", ProductID: product, Commit: commit, RequestedRef: tag, Tag: tag, Status: "ready", DBPath: stats.DBPath, ManifestPath: stats.ManifestPath, IndexedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
}

func writeValidationFile(t *testing.T, root, name, data string) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}
