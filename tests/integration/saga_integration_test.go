//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sanskarpan/PayGate/internal/saga"
)

func TestIntegrationSagaReplayCompletesFailedCommand(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	env := buildGenericSagaEnv(t, db)

	var failFirst atomic.Int32
	failFirst.Store(1)
	env.sagaSvc.RegisterHandler("test.command", func(ctx context.Context, cmd saga.Command) (map[string]any, error) {
		if failFirst.CompareAndSwap(1, 0) {
			return nil, errors.New("simulated command failure")
		}
		return map[string]any{"command_id": cmd.CommandID, "status": "completed"}, nil
	})
	env.sagaSvc.RegisterCompensationHandler("generic_replay", func(ctx context.Context, inst saga.Instance) error {
		return errors.New("force replayable terminal failure")
	})

	merchantID, authHeader := createSagaMerchant(t, ctx, env)
	instance, err := env.sagaSvc.StartCommandSaga(ctx, saga.CreateCommandSagaInput{
		MerchantID: merchantID,
		SagaType:   "generic_replay",
		InitialStep: saga.CreateStepInput{
			StepName:    "run_test_command",
			StepKind:    saga.StepKindCommand,
			CommandName: "test.command",
			InputPayload: map[string]any{
				"kind": "replay",
			},
			MaxAttempts: 1,
		},
	})
	if err != nil {
		t.Fatalf("start saga: %v", err)
	}

	workerCtx, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()
	go saga.NewWorker(env.sagaSvc, 10*time.Millisecond, nil).Start(workerCtx)

	waitFor(t, 5*time.Second, func() bool {
		current, err := env.sagaSvc.Get(ctx, merchantID, instance.ID)
		return err == nil && current.Status == saga.StatusFailed
	})

	getReq := httptest.NewRequest(http.MethodGet, "/v1/sagas/"+instance.ID, nil)
	getReq.Header.Set("Authorization", authHeader)
	getRec := httptest.NewRecorder()
	env.mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get saga: expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	var sagaResp map[string]any
	if err := json.Unmarshal(getRec.Body.Bytes(), &sagaResp); err != nil {
		t.Fatalf("decode saga response: %v", err)
	}
	if sagaResp["status"] != string(saga.StatusFailed) {
		t.Fatalf("expected failed saga before replay, got %v", sagaResp["status"])
	}

	replayReq := httptest.NewRequest(http.MethodPost, "/v1/sagas/"+instance.ID+"/replay", nil)
	replayReq.Header.Set("Authorization", authHeader)
	replayRec := httptest.NewRecorder()
	env.mux.ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusAccepted {
		t.Fatalf("replay saga: expected 202, got %d body=%s", replayRec.Code, replayRec.Body.String())
	}

	waitFor(t, 5*time.Second, func() bool {
		current, err := env.sagaSvc.Get(ctx, merchantID, instance.ID)
		return err == nil && current.Status == saga.StatusCompleted && current.ReplayCount == 1
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/sagas/"+instance.ID+"/dispatches", nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get dispatches: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var dispatches map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &dispatches); err != nil {
		t.Fatalf("decode dispatches: %v", err)
	}
	items := dispatches["items"].([]any)
	if len(items) < 2 {
		t.Fatalf("expected at least 2 dispatch records, got %d", len(items))
	}
}
