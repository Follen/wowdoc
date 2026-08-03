package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/follenfang/wowdoc/internal/catalog"
	"github.com/follenfang/wowdoc/internal/indexer"
)

func TestWowdataInitDefaultsToTenHotTags(t *testing.T) {
	cmd := dataInitCommand()
	flag := cmd.Flags().Lookup("hot-tags")
	if flag == nil {
		t.Fatal("hot-tags flag is missing")
	}
	if flag.DefValue != "10" {
		t.Fatalf("hot-tags default=%s, want 10", flag.DefValue)
	}
}

func TestInitWorkerBudgetIsGloballyBounded(t *testing.T) {
	if got := initWorkerBudget(0); got < 4 || got > 8 {
		t.Fatalf("default worker budget=%d, want 4..8", got)
	}
	if got := initWorkerBudget(64); got != 8 {
		t.Fatalf("capped worker budget=%d, want 8", got)
	}
	if got := initWorkerBudget(2); got != 2 {
		t.Fatalf("explicit worker budget=%d, want 2", got)
	}
}

func TestInitJobsRunBranchesInParallelAndRefsSerially(t *testing.T) {
	jobs := []initJob{
		{source: catalog.Source{ID: "source"}, product: catalog.Product{ID: "a"}, refs: []initRef{{commit: "a1", requested: "latest"}, {commit: "a2", requested: "tag-a"}}},
		{source: catalog.Source{ID: "source"}, product: catalog.Product{ID: "b"}, refs: []initRef{{commit: "b1", requested: "latest"}, {commit: "b2", requested: "tag-b"}}},
		{source: catalog.Source{ID: "source"}, product: catalog.Product{ID: "c"}, refs: []initRef{{commit: "c1", requested: "latest"}, {commit: "c2", requested: "tag-c"}}},
	}
	var active, maxActive, maxWorkers int32
	var mu sync.Mutex
	activeProducts := map[string]int{}
	var serialViolation bool
	built, failures, branchWorkers := runInitJobs(context.Background(), jobs, 8, func(job initJob, ref initRef, workers int) (indexer.Stats, error) {
		for {
			previous := atomic.LoadInt32(&maxWorkers)
			if int32(workers) <= previous || atomic.CompareAndSwapInt32(&maxWorkers, previous, int32(workers)) {
				break
			}
		}
		mu.Lock()
		activeProducts[job.product.ID]++
		if activeProducts[job.product.ID] > 1 {
			serialViolation = true
		}
		mu.Unlock()
		current := atomic.AddInt32(&active, 1)
		for {
			previous := atomic.LoadInt32(&maxActive)
			if current <= previous || atomic.CompareAndSwapInt32(&maxActive, previous, current) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&active, -1)
		mu.Lock()
		activeProducts[job.product.ID]--
		mu.Unlock()
		return indexer.Stats{SnapshotID: job.product.ID + "-" + ref.commit}, nil
	})
	if len(failures) != 0 || len(built) != 6 {
		t.Fatalf("built=%d failures=%#v", len(built), failures)
	}
	if branchWorkers != 3 || maxActive < 2 {
		t.Fatalf("branchWorkers=%d maxActive=%d", branchWorkers, maxActive)
	}
	if int(maxWorkers)*branchWorkers > 8 {
		t.Fatalf("workers=%d branchWorkers=%d exceed budget", maxWorkers, branchWorkers)
	}
	if serialViolation {
		t.Fatal("refs from the same branch overlapped")
	}
}

func TestInitJobsKeepOtherBranchesAfterFailure(t *testing.T) {
	jobs := []initJob{
		{source: catalog.Source{ID: "source"}, product: catalog.Product{ID: "bad"}, refs: []initRef{{commit: "bad1", requested: "latest"}, {commit: "bad2", requested: "older"}}},
		{source: catalog.Source{ID: "source"}, product: catalog.Product{ID: "good"}, refs: []initRef{{commit: "good1", requested: "latest"}, {commit: "good2", requested: "older"}}},
	}
	var called sync.Map
	built, failures, _ := runInitJobs(context.Background(), jobs, 4, func(job initJob, ref initRef, workers int) (indexer.Stats, error) {
		called.Store(ref.commit, true)
		if ref.commit == "bad1" {
			return indexer.Stats{}, errors.New("fixture failure")
		}
		return indexer.Stats{SnapshotID: ref.commit}, nil
	})
	if len(failures) != 1 || len(built) != 2 {
		t.Fatalf("built=%#v failures=%#v", built, failures)
	}
	if _, ok := called.Load("bad2"); ok {
		t.Fatal("failed branch continued to an older ref")
	}
	if _, ok := called.Load("good2"); !ok {
		t.Fatal("independent branch did not finish")
	}
}

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
