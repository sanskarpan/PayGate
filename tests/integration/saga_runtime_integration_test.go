//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/auth"
	"github.com/sanskarpan/PayGate/internal/idempotency"
	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/saga"
)

func TestIntegrationSagaTimeoutRecordsDeadLetterAndCompensation(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	env := buildGenericSagaEnv(t, db)

	var compensated atomic.Int32
	env.sagaSvc.RegisterHandler("test.timeout", func(ctx context.Context, cmd saga.Command) (map[string]any, error) {
		time.Sleep(200 * time.Millisecond)
		return map[string]any{"command_id": cmd.CommandID, "status": "late_success"}, nil
	})
	env.sagaSvc.RegisterCompensationHandler("generic_timeout", func(ctx context.Context, inst saga.Instance) error {
		compensated.Add(1)
		return nil
	})
	env.sagaSvc.SetLeaseTTLForTest(25 * time.Millisecond)

	merchantID, authHeader := createSagaMerchant(t, ctx, env)
	timeoutAt := time.Now().Add(40 * time.Millisecond)
	instance, err := env.sagaSvc.StartCommandSaga(ctx, saga.CreateCommandSagaInput{
		MerchantID: merchantID,
		SagaType:   "generic_timeout",
		TimeoutAt:  &timeoutAt,
		InitialStep: saga.CreateStepInput{
			StepName:    "run_timeout_command",
			StepKind:    saga.StepKindCommand,
			CommandName: "test.timeout",
			InputPayload: map[string]any{
				"kind": "timeout",
			},
			MaxAttempts: 1,
		},
	})
	if err != nil {
		t.Fatalf("start saga: %v", err)
	}

	go saga.NewWorker(env.sagaSvc, 10*time.Millisecond, nil).Start(context.Background())

	waitFor(t, 5*time.Second, func() bool {
		current, err := env.sagaSvc.Get(ctx, merchantID, instance.ID)
		return err == nil && current.Status == saga.StatusAborted
	})
	if compensated.Load() == 0 {
		t.Fatal("expected compensation handler to run")
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/sagas/"+instance.ID+"/dead-letters", nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get dead letters: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var deadLetters map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &deadLetters); err != nil {
		t.Fatalf("decode dead letters: %v", err)
	}
	items := deadLetters["items"].([]any)
	if len(items) == 0 {
		t.Fatal("expected at least one dead letter")
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/sagas/"+instance.ID+"/dispatches", nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get dispatches: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var dispatches map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &dispatches); err != nil {
		t.Fatalf("decode dispatches: %v", err)
	}
	dispatchItems := dispatches["items"].([]any)
	if len(dispatchItems) == 0 {
		t.Fatal("expected at least one dispatch record")
	}
	firstDispatch := dispatchItems[0].(map[string]any)
	if firstDispatch["status"] != "dispatched" && firstDispatch["status"] != "nacked" && firstDispatch["status"] != "acked" {
		t.Fatalf("unexpected dispatch status: %#v", firstDispatch["status"])
	}
}

func TestIntegrationSagaOperatorAbortRecordsAction(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	env := buildGenericSagaEnv(t, db)
	env.sagaSvc.RegisterHandler("test.abort", func(ctx context.Context, cmd saga.Command) (map[string]any, error) {
		time.Sleep(300 * time.Millisecond)
		return map[string]any{"command_id": cmd.CommandID, "status": "late_success"}, nil
	})
	env.sagaSvc.RegisterCompensationHandler("generic_abort", func(ctx context.Context, inst saga.Instance) error {
		return nil
	})
	env.sagaSvc.SetLeaseTTLForTest(50 * time.Millisecond)

	merchantID, authHeader := createSagaMerchant(t, ctx, env)
	instance, err := env.sagaSvc.StartCommandSaga(ctx, saga.CreateCommandSagaInput{
		MerchantID: merchantID,
		SagaType:   "generic_abort",
		InitialStep: saga.CreateStepInput{
			StepName:    "run_abort_command",
			StepKind:    saga.StepKindCommand,
			CommandName: "test.abort",
			MaxAttempts: 1,
		},
	})
	if err != nil {
		t.Fatalf("start saga: %v", err)
	}
	go saga.NewWorker(env.sagaSvc, 10*time.Millisecond, nil).Start(context.Background())

	req := httptest.NewRequest(http.MethodPost, "/v1/sagas/"+instance.ID+"/override", strings.NewReader(`{"action":"abort","reason":"manual operator stop"}`))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("override saga: expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}

	waitFor(t, 5*time.Second, func() bool {
		current, err := env.sagaSvc.Get(ctx, merchantID, instance.ID)
		return err == nil && current.Status == saga.StatusAborted
	})

	req = httptest.NewRequest(http.MethodGet, "/v1/sagas/"+instance.ID+"/actions", nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get actions: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var actions map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &actions); err != nil {
		t.Fatalf("decode actions: %v", err)
	}
	if len(actions["items"].([]any)) == 0 {
		t.Fatal("expected recorded operator action")
	}
}

func TestIntegrationSagaDuplicateLeaseRunsHandlerTwiceButCompletesOnce(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	env := buildGenericSagaEnv(t, db)
	var executions atomic.Int32
	env.sagaSvc.RegisterHandler("test.duplicate", func(ctx context.Context, cmd saga.Command) (map[string]any, error) {
		executions.Add(1)
		time.Sleep(90 * time.Millisecond)
		return map[string]any{"command_id": cmd.CommandID, "status": "completed"}, nil
	})
	env.sagaSvc.SetLeaseTTLForTest(25 * time.Millisecond)

	merchantID, authHeader := createSagaMerchant(t, ctx, env)
	instance, err := env.sagaSvc.StartCommandSaga(ctx, saga.CreateCommandSagaInput{
		MerchantID: merchantID,
		SagaType:   "generic_duplicate",
		InitialStep: saga.CreateStepInput{
			StepName:    "run_duplicate_command",
			StepKind:    saga.StepKindCommand,
			CommandName: "test.duplicate",
			MaxAttempts: 2,
		},
	})
	if err != nil {
		t.Fatalf("start saga: %v", err)
	}
	go saga.NewWorker(env.sagaSvc, 10*time.Millisecond, nil).Start(context.Background())
	go saga.NewWorker(env.sagaSvc, 10*time.Millisecond, nil).Start(context.Background())

	waitFor(t, 5*time.Second, func() bool {
		current, err := env.sagaSvc.Get(ctx, merchantID, instance.ID)
		return err == nil && current.Status == saga.StatusCompleted
	})
	if executions.Load() < 2 {
		t.Fatalf("expected duplicate handler execution, got %d", executions.Load())
	}

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
	if len(dispatches["items"].([]any)) < 2 {
		t.Fatal("expected multiple dispatch attempts under duplicate lease")
	}
}

func TestIntegrationSagaWorkerRestartRetriesCommand(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	env := buildGenericSagaEnv(t, db)
	var attempts atomic.Int32
	env.sagaSvc.RegisterHandler("test.restart", func(ctx context.Context, cmd saga.Command) (map[string]any, error) {
		attempt := attempts.Add(1)
		if attempt == 1 {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return map[string]any{"command_id": cmd.CommandID, "status": "completed"}, nil
	})
	env.sagaSvc.SetLeaseTTLForTest(25 * time.Millisecond)

	merchantID, authHeader := createSagaMerchant(t, ctx, env)
	instance, err := env.sagaSvc.StartCommandSaga(ctx, saga.CreateCommandSagaInput{
		MerchantID: merchantID,
		SagaType:   "generic_restart",
		InitialStep: saga.CreateStepInput{
			StepName:    "run_restart_command",
			StepKind:    saga.StepKindCommand,
			CommandName: "test.restart",
			MaxAttempts: 2,
		},
	})
	if err != nil {
		t.Fatalf("start saga: %v", err)
	}

	workerCtx, cancelWorker := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		saga.NewWorker(env.sagaSvc, 10*time.Millisecond, nil).Start(workerCtx)
	}()

	waitFor(t, 5*time.Second, func() bool {
		return attempts.Load() >= 1
	})
	cancelWorker()
	<-done

	restartCtx, cancelRestart := context.WithCancel(context.Background())
	defer cancelRestart()
	go saga.NewWorker(env.sagaSvc, 10*time.Millisecond, nil).Start(restartCtx)

	waitFor(t, 5*time.Second, func() bool {
		current, err := env.sagaSvc.Get(ctx, merchantID, instance.ID)
		return err == nil && current.Status == saga.StatusCompleted
	})
	if attempts.Load() < 2 {
		t.Fatalf("expected worker restart to trigger a retry, got %d attempts", attempts.Load())
	}

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
	if len(dispatches["items"].([]any)) < 2 {
		t.Fatal("expected at least two dispatch attempts after restart")
	}
}

type genericSagaEnv struct {
	mux         *http.ServeMux
	merchantSvc *merchant.Service
	sagaSvc     *saga.Service
}

func buildGenericSagaEnv(t *testing.T, db *pgxpool.Pool) genericSagaEnv {
	t.Helper()

	merchantRepo := merchant.NewPostgresRepository(db)
	merchantSvc := merchant.NewService(merchantRepo, merchant.WithSessionSecret("integration-dashboard-secret"))
	authMw := auth.NewMiddleware(merchantSvc)
	idemMw := idempotency.NewMiddleware(idempotency.NewStore(db, nil))
	sagaSvc := saga.NewService(saga.NewPostgresRepository(db), nil)

	merchantHandler := merchant.NewHandler(merchantSvc)
	sagaHandler := saga.NewHandler(sagaSvc)

	protected := func(scope merchant.APIKeyScope, next http.Handler) http.Handler {
		return authMw.RequireScope(scope, idemMw.Wrap(next))
	}

	mux := http.NewServeMux()
	merchantHandler.RegisterRoutes(mux)
	sagaHandler.RegisterRoutesWithAuth(mux, protected)

	return genericSagaEnv{
		mux:         mux,
		merchantSvc: merchantSvc,
		sagaSvc:     sagaSvc,
	}
}

func createSagaMerchant(t *testing.T, ctx context.Context, env genericSagaEnv) (string, string) {
	t.Helper()

	slug := strings.NewReplacer("/", "-", " ", "-", "_", "-").Replace(strings.ToLower(t.Name()))
	email := fmt.Sprintf("%s-%d@test.com", slug, time.Now().UnixNano())
	createdMerchant, err := env.merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Saga Runtime Merchant", Email: email, BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	key, err := env.merchantSvc.CreateAPIKey(ctx, createdMerchant.ID, merchant.CreateAPIKeyInput{
		Mode: merchant.APIKeyModeTest, Scope: merchant.APIKeyScopeAdmin,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	return createdMerchant.ID, basicAuth(key.KeyID, key.KeySecret)
}

func waitFor(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}
