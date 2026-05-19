package saga

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/common/idgen"
)

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) CreateCommandSaga(ctx context.Context, in CreateCommandSagaInput) (Instance, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Instance{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	sagaID := idgen.New("saga")
	stepID := idgen.New("sstp")
	commandID := idgen.New("cmd")
	if in.InitialStep.MaxAttempts <= 0 {
		in.InitialStep.MaxAttempts = 1
	}

	inputPayload, err := marshalPayload(in.InputPayload)
	if err != nil {
		return Instance{}, err
	}
	contextPayload, err := marshalPayload(in.ContextPayload)
	if err != nil {
		return Instance{}, err
	}
	stepInput, err := marshalPayload(in.InitialStep.InputPayload)
	if err != nil {
		return Instance{}, err
	}

	_, err = tx.Exec(ctx, `
INSERT INTO paygate_sagas.saga_instances
    (id, merchant_id, saga_type, status, correlation_id, causation_id, input_payload, context_payload, current_step_index, deadline_at, timeout_at)
VALUES
    ($1, $2, $3, $4, $5, $6, $7, $8, 0, $9, $10)
`, sagaID, in.MerchantID, in.SagaType, StatusRunning, in.CorrelationID, in.CausationID, inputPayload, contextPayload, in.DeadlineAt, in.TimeoutAt)
	if err != nil {
		return Instance{}, fmt.Errorf("insert saga instance: %w", err)
	}

	_, err = tx.Exec(ctx, `
INSERT INTO paygate_sagas.saga_steps
    (id, saga_id, step_index, step_name, step_kind, status, command_name, command_id, reply_topic, input_payload, max_attempts)
VALUES
    ($1, $2, 0, $3, $4, $5, $6, $7, $8, $9, $10)
`, stepID, sagaID, in.InitialStep.StepName, in.InitialStep.StepKind, StepStatusPending, in.InitialStep.CommandName, commandID, in.InitialStep.ReplyTopic, stepInput, in.InitialStep.MaxAttempts)
	if err != nil {
		return Instance{}, fmt.Errorf("insert saga step: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Instance{}, err
	}
	return r.Get(ctx, in.MerchantID, sagaID)
}

func (r *PostgresRepository) Get(ctx context.Context, merchantID, sagaID string) (Instance, error) {
	row := r.db.QueryRow(ctx, `
SELECT id, merchant_id, saga_type, status, correlation_id, causation_id,
       input_payload, context_payload, current_step_index, failure_code, failure_reason,
       leased_by, last_leased_at, replay_count, deadline_at, timeout_at,
       started_at, completed_at, created_at, updated_at
FROM paygate_sagas.saga_instances
WHERE id = $1 AND merchant_id = $2
`, sagaID, merchantID)
	instance, err := scanInstance(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Instance{}, ErrSagaNotFound
	}
	if err != nil {
		return Instance{}, err
	}

	steps, err := r.listSteps(ctx, sagaID)
	if err != nil {
		return Instance{}, err
	}
	instance.Steps = steps
	return instance, nil
}

func (r *PostgresRepository) List(ctx context.Context, merchantID string, limit int) ([]Instance, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, saga_type, status, correlation_id, causation_id,
       input_payload, context_payload, current_step_index, failure_code, failure_reason,
       leased_by, last_leased_at, replay_count, deadline_at, timeout_at,
       started_at, completed_at, created_at, updated_at
FROM paygate_sagas.saga_instances
WHERE merchant_id = $1
ORDER BY created_at DESC
LIMIT $2
`, merchantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Instance
	for rows.Next() {
		instance, err := scanInstance(rows)
		if err != nil {
			return nil, err
		}
		steps, err := r.listSteps(ctx, instance.ID)
		if err != nil {
			return nil, err
		}
		instance.Steps = steps
		out = append(out, instance)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) LeaseNextCommandStep(ctx context.Context, leaseOwner string, leaseTTL time.Duration) (Step, bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Step{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	leaseSeconds := int(leaseTTL.Seconds())
	if leaseSeconds <= 0 {
		leaseSeconds = 30
	}

	row := tx.QueryRow(ctx, `
WITH candidate AS (
    SELECT ss.id
    FROM paygate_sagas.saga_steps ss
    JOIN paygate_sagas.saga_instances si ON si.id = ss.saga_id
    WHERE ss.step_kind = 'command'
      AND (
            (ss.status = 'pending' AND ss.next_retry_at <= NOW())
         OR (ss.status = 'in_progress' AND ss.leased_at <= NOW() - make_interval(secs => $2))
      )
      AND si.status IN ('pending', 'running')
    ORDER BY si.created_at ASC, ss.step_index ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
UPDATE paygate_sagas.saga_steps ss
SET status = 'in_progress',
    leased_by = $1,
    leased_at = NOW(),
    attempt_count = ss.attempt_count + 1,
    updated_at = NOW()
FROM candidate c, paygate_sagas.saga_instances si
WHERE ss.id = c.id
  AND si.id = ss.saga_id
RETURNING ss.id, ss.saga_id, si.merchant_id, ss.step_index, ss.step_name, ss.step_kind,
          ss.status, ss.command_name, ss.command_id, ss.reply_topic, ss.input_payload,
          ss.output_payload, ss.error_code, ss.error_message, ss.next_retry_at,
          ss.leased_by, ss.leased_at, ss.completed_at, ss.attempt_count, ss.max_attempts,
          ss.created_at, ss.updated_at
`, leaseOwner, leaseSeconds)

	var step Step
	err = scanStep(row, &step)
	if errors.Is(err, pgx.ErrNoRows) {
		return Step{}, false, nil
	}
	if err != nil {
		return Step{}, false, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_sagas.saga_instances
SET last_leased_at = NOW(), leased_by = $2, updated_at = NOW()
WHERE id = $1
`, step.SagaID, leaseOwner); err != nil {
		return Step{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Step{}, false, err
	}
	return step, true, nil
}

func (r *PostgresRepository) LeaseTimedOutSaga(ctx context.Context, leaseOwner string, leaseTTL time.Duration) (Instance, bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Instance{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	leaseSeconds := int(leaseTTL.Seconds())
	if leaseSeconds <= 0 {
		leaseSeconds = 30
	}

	row := tx.QueryRow(ctx, `
WITH candidate AS (
    SELECT id
    FROM paygate_sagas.saga_instances
    WHERE timeout_at IS NOT NULL
      AND timeout_at <= NOW()
      AND status IN ('pending', 'running', 'waiting', 'compensating')
      AND (
            last_leased_at IS NULL
         OR last_leased_at <= NOW() - make_interval(secs => $2)
      )
    ORDER BY timeout_at ASC, created_at ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
UPDATE paygate_sagas.saga_instances si
SET leased_by = $1,
    last_leased_at = NOW(),
    updated_at = NOW()
FROM candidate c
WHERE si.id = c.id
RETURNING si.id, si.merchant_id, si.saga_type, si.status, si.correlation_id, si.causation_id,
          si.input_payload, si.context_payload, si.current_step_index, si.failure_code, si.failure_reason,
          si.leased_by, si.last_leased_at, si.replay_count, si.deadline_at, si.timeout_at,
          si.started_at, si.completed_at, si.created_at, si.updated_at
`, leaseOwner, leaseSeconds)

	instance, err := scanInstance(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Instance{}, false, nil
	}
	if err != nil {
		return Instance{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Instance{}, false, err
	}
	instance.Steps, err = r.listSteps(ctx, instance.ID)
	if err != nil {
		return Instance{}, false, err
	}
	return instance, true, nil
}

func (r *PostgresRepository) CompleteStep(ctx context.Context, in CompleteStepInput) (Instance, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Instance{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	outputPayload, err := marshalPayload(in.CommandOutput)
	if err != nil {
		return Instance{}, err
	}

	var sagaID string
	var merchantID string
	var sagaStatus Status
	if err := tx.QueryRow(ctx, `
SELECT ss.saga_id, si.merchant_id, si.status
FROM paygate_sagas.saga_steps ss
JOIN paygate_sagas.saga_instances si ON si.id = ss.saga_id
WHERE ss.id = $1
FOR UPDATE
`, in.StepID).Scan(&sagaID, &merchantID, &sagaStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Instance{}, ErrSagaNotFound
		}
		return Instance{}, err
	}

	if sagaStatus == StatusAborted || sagaStatus == StatusFailed {
		if _, err := tx.Exec(ctx, `
UPDATE paygate_sagas.saga_steps
SET status = CASE WHEN status = 'completed' THEN status ELSE 'cancelled' END,
    leased_by = NULL,
    leased_at = NULL,
    updated_at = NOW()
WHERE id = $1
`, in.StepID); err != nil {
			return Instance{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Instance{}, err
		}
		return r.Get(ctx, merchantID, sagaID)
	}

	if _, err := tx.Exec(ctx, `
UPDATE paygate_sagas.saga_steps
SET status = 'completed',
    output_payload = $2,
    error_code = NULL,
    error_message = NULL,
    completed_at = NOW(),
    updated_at = NOW()
WHERE id = $1
`, in.StepID, outputPayload); err != nil {
		return Instance{}, err
	}

	var incomplete int
	if err := tx.QueryRow(ctx, `
SELECT COUNT(*)
FROM paygate_sagas.saga_steps
WHERE saga_id = $1 AND status NOT IN ('completed', 'compensated', 'cancelled')
`, sagaID).Scan(&incomplete); err != nil {
		return Instance{}, err
	}

	if incomplete == 0 {
		if _, err := tx.Exec(ctx, `
UPDATE paygate_sagas.saga_instances
SET status = 'completed',
    completed_at = NOW(),
    leased_by = NULL,
    last_leased_at = NULL,
    failure_code = NULL,
    failure_reason = NULL,
    updated_at = NOW()
WHERE id = $1
`, sagaID); err != nil {
			return Instance{}, err
		}
	} else {
		if _, err := tx.Exec(ctx, `
UPDATE paygate_sagas.saga_instances
SET status = 'running',
    leased_by = NULL,
    last_leased_at = NULL,
    updated_at = NOW()
WHERE id = $1
`, sagaID); err != nil {
			return Instance{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Instance{}, err
	}
	return r.Get(ctx, merchantID, sagaID)
}

func (r *PostgresRepository) FailStep(ctx context.Context, in FailStepInput) (Instance, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Instance{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var sagaID string
	var merchantID string
	var attemptCount int
	var maxAttempts int
	if err := tx.QueryRow(ctx, `
SELECT ss.saga_id, si.merchant_id, ss.attempt_count, ss.max_attempts
FROM paygate_sagas.saga_steps ss
JOIN paygate_sagas.saga_instances si ON si.id = ss.saga_id
WHERE ss.id = $1
FOR UPDATE
`, in.StepID).Scan(&sagaID, &merchantID, &attemptCount, &maxAttempts); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Instance{}, ErrSagaNotFound
		}
		return Instance{}, err
	}

	terminal := in.Terminal || attemptCount >= maxAttempts
	if terminal {
		if _, err := tx.Exec(ctx, `
UPDATE paygate_sagas.saga_steps
SET status = 'failed',
    error_code = $2,
    error_message = $3,
    leased_by = NULL,
    leased_at = NULL,
    updated_at = NOW()
WHERE id = $1
`, in.StepID, in.ErrorCode, in.ErrorMessage); err != nil {
			return Instance{}, err
		}
		if _, err := tx.Exec(ctx, `
UPDATE paygate_sagas.saga_instances
SET status = 'failed',
    failure_code = $2,
    failure_reason = $3,
    leased_by = NULL,
    last_leased_at = NULL,
    updated_at = NOW()
WHERE id = $1
`, sagaID, in.ErrorCode, in.ErrorMessage); err != nil {
			return Instance{}, err
		}
	} else {
		nextRetryAt := time.Now().Add(in.RetryBackoff)
		if _, err := tx.Exec(ctx, `
UPDATE paygate_sagas.saga_steps
SET status = 'pending',
    error_code = $2,
    error_message = $3,
    next_retry_at = $4,
    leased_by = NULL,
    leased_at = NULL,
    updated_at = NOW()
WHERE id = $1
`, in.StepID, in.ErrorCode, in.ErrorMessage, nextRetryAt); err != nil {
			return Instance{}, err
		}
		if _, err := tx.Exec(ctx, `
UPDATE paygate_sagas.saga_instances
SET status = 'running',
    leased_by = NULL,
    last_leased_at = NULL,
    updated_at = NOW()
WHERE id = $1
`, sagaID); err != nil {
			return Instance{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Instance{}, err
	}
	return r.Get(ctx, merchantID, sagaID)
}

func (r *PostgresRepository) FailSaga(ctx context.Context, in FailSagaInput) (Instance, error) {
	tag, err := r.db.Exec(ctx, `
UPDATE paygate_sagas.saga_instances
SET status = 'failed',
    failure_code = $3,
    failure_reason = $4,
    leased_by = NULL,
    last_leased_at = NULL,
    updated_at = NOW()
WHERE id = $1 AND merchant_id = $2
`, in.SagaID, in.MerchantID, in.FailureCode, in.FailureReason)
	if err != nil {
		return Instance{}, err
	}
	if tag.RowsAffected() == 0 {
		return Instance{}, ErrSagaNotFound
	}
	return r.Get(ctx, in.MerchantID, in.SagaID)
}

func (r *PostgresRepository) MarkCompensating(ctx context.Context, merchantID, sagaID, failureCode, failureReason string) (Instance, error) {
	tag, err := r.db.Exec(ctx, `
UPDATE paygate_sagas.saga_instances
SET status = 'compensating',
    failure_code = $3,
    failure_reason = $4,
    leased_by = NULL,
    last_leased_at = NULL,
    updated_at = NOW()
WHERE id = $1 AND merchant_id = $2
`, sagaID, merchantID, failureCode, failureReason)
	if err != nil {
		return Instance{}, err
	}
	if tag.RowsAffected() == 0 {
		return Instance{}, ErrSagaNotFound
	}
	return r.Get(ctx, merchantID, sagaID)
}

func (r *PostgresRepository) Abort(ctx context.Context, in OverrideInput) (Instance, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Instance{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
UPDATE paygate_sagas.saga_instances
SET status = 'aborted',
    completed_at = NOW(),
    leased_by = NULL,
    last_leased_at = NULL,
    updated_at = NOW()
WHERE id = $1 AND merchant_id = $2
`, in.SagaID, in.MerchantID)
	if err != nil {
		return Instance{}, err
	}
	if tag.RowsAffected() == 0 {
		return Instance{}, ErrSagaNotFound
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_sagas.saga_steps
SET status = CASE
        WHEN status IN ('completed', 'compensated', 'cancelled', 'failed') THEN status
        ELSE 'cancelled'
    END,
    leased_by = NULL,
    leased_at = NULL,
    updated_at = NOW()
WHERE saga_id = $1
`, in.SagaID); err != nil {
		return Instance{}, err
	}
	body, err := marshalPayload(map[string]any{"status_after": StatusAborted})
	if err != nil {
		return Instance{}, err
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO paygate_sagas.saga_operator_actions
    (id, saga_id, merchant_id, action, actor_type, actor_id, reason, payload_json)
VALUES
    ($1, $2, $3, $4, $5, $6, $7, $8)
`, idgen.New("sact"), in.SagaID, in.MerchantID, string(in.Action), fallbackString(in.ActorType, "system"), in.ActorID, in.Reason, body); err != nil {
		return Instance{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Instance{}, err
	}
	return r.Get(ctx, in.MerchantID, in.SagaID)
}

func (r *PostgresRepository) ForceComplete(ctx context.Context, in OverrideInput) (Instance, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Instance{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
UPDATE paygate_sagas.saga_instances
SET status = 'completed',
    completed_at = NOW(),
    failure_code = NULL,
    failure_reason = NULL,
    leased_by = NULL,
    last_leased_at = NULL,
    updated_at = NOW()
WHERE id = $1 AND merchant_id = $2
`, in.SagaID, in.MerchantID)
	if err != nil {
		return Instance{}, err
	}
	if tag.RowsAffected() == 0 {
		return Instance{}, ErrSagaNotFound
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_sagas.saga_steps
SET status = CASE
        WHEN status IN ('completed', 'compensated', 'cancelled', 'failed') THEN status
        ELSE 'cancelled'
    END,
    leased_by = NULL,
    leased_at = NULL,
    updated_at = NOW()
WHERE saga_id = $1
`, in.SagaID); err != nil {
		return Instance{}, err
	}
	body, err := marshalPayload(map[string]any{"status_after": StatusCompleted})
	if err != nil {
		return Instance{}, err
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO paygate_sagas.saga_operator_actions
    (id, saga_id, merchant_id, action, actor_type, actor_id, reason, payload_json)
VALUES
    ($1, $2, $3, $4, $5, $6, $7, $8)
`, idgen.New("sact"), in.SagaID, in.MerchantID, string(in.Action), fallbackString(in.ActorType, "system"), in.ActorID, in.Reason, body); err != nil {
		return Instance{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Instance{}, err
	}
	return r.Get(ctx, in.MerchantID, in.SagaID)
}

func (r *PostgresRepository) Replay(ctx context.Context, in ReplayInput) (Instance, error) {
	current, err := r.Get(ctx, in.MerchantID, in.SagaID)
	if err != nil {
		return Instance{}, err
	}
	if in.DryRun {
		return current, nil
	}
	if current.Status != StatusFailed && !in.Force {
		return Instance{}, ErrSagaNotReplayable
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Instance{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var stepID string
	if err := tx.QueryRow(ctx, `
SELECT id
FROM paygate_sagas.saga_steps
WHERE saga_id = $1 AND status = 'failed'
ORDER BY step_index DESC
LIMIT 1
FOR UPDATE
`, in.SagaID).Scan(&stepID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Instance{}, ErrSagaNotReplayable
		}
		return Instance{}, err
	}

	if _, err := tx.Exec(ctx, `
UPDATE paygate_sagas.saga_steps
SET status = 'pending',
    command_id = $2,
    output_payload = '{}'::jsonb,
    error_code = NULL,
    error_message = NULL,
    next_retry_at = NOW(),
    leased_by = NULL,
    leased_at = NULL,
    completed_at = NULL,
    attempt_count = 0,
    updated_at = NOW()
WHERE id = $1
`, stepID, idgen.New("cmd")); err != nil {
		return Instance{}, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_sagas.saga_instances
SET status = 'running',
    replay_count = replay_count + 1,
    failure_code = NULL,
    failure_reason = NULL,
    leased_by = NULL,
    last_leased_at = NULL,
    completed_at = NULL,
    updated_at = NOW()
WHERE id = $1 AND merchant_id = $2
`, in.SagaID, in.MerchantID); err != nil {
		return Instance{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Instance{}, err
	}
	if err := r.RecordOperatorAction(ctx, OperatorActionInput{
		MerchantID: in.MerchantID,
		SagaID:     in.SagaID,
		Action:     "replay",
		ActorType:  fallbackString(in.ActorType, "system"),
		ActorID:    in.ActorID,
		Reason:     in.Reason,
		Payload:    map[string]any{"force": in.Force, "dry_run": in.DryRun},
	}); err != nil {
		return Instance{}, err
	}
	return r.Get(ctx, in.MerchantID, in.SagaID)
}

func (r *PostgresRepository) RecordDeadLetter(ctx context.Context, in DeadLetterInput) error {
	body, err := marshalPayload(in.Payload)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
INSERT INTO paygate_sagas.saga_dead_letters
    (id, saga_id, step_id, merchant_id, dead_letter_type, command_name, command_id, error_code, error_message, payload_json)
VALUES
    ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, $8, $9, $10)
`, idgen.New("sdlq"), in.SagaID, in.StepID, in.MerchantID, in.DeadLetterType, in.CommandName, in.CommandID, in.ErrorCode, in.ErrorMessage, body)
	return err
}

func (r *PostgresRepository) RecordOperatorAction(ctx context.Context, in OperatorActionInput) error {
	body, err := marshalPayload(in.Payload)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
INSERT INTO paygate_sagas.saga_operator_actions
    (id, saga_id, merchant_id, action, actor_type, actor_id, reason, payload_json)
VALUES
    ($1, $2, $3, $4, $5, $6, $7, $8)
`, idgen.New("sact"), in.SagaID, in.MerchantID, in.Action, fallbackString(in.ActorType, "system"), in.ActorID, in.Reason, body)
	return err
}

func (r *PostgresRepository) ListDeadLetters(ctx context.Context, merchantID, sagaID string, limit int) ([]DeadLetter, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
SELECT id, saga_id, COALESCE(step_id, ''), merchant_id, dead_letter_type, command_name, command_id, error_code, error_message, payload_json, created_at
FROM paygate_sagas.saga_dead_letters
WHERE saga_id = $1 AND merchant_id = $2
ORDER BY created_at DESC
LIMIT $3
`, sagaID, merchantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DeadLetter
	for rows.Next() {
		item, err := scanDeadLetter(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ListOperatorActions(ctx context.Context, merchantID, sagaID string, limit int) ([]OperatorAction, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
SELECT id, saga_id, merchant_id, action, actor_type, actor_id, reason, payload_json, created_at
FROM paygate_sagas.saga_operator_actions
WHERE saga_id = $1 AND merchant_id = $2
ORDER BY created_at DESC
LIMIT $3
`, sagaID, merchantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OperatorAction
	for rows.Next() {
		item, err := scanOperatorAction(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) RecordDispatch(ctx context.Context, in RecordDispatchInput) (CommandDispatch, error) {
	inputPayload, err := marshalPayload(in.InputPayload)
	if err != nil {
		return CommandDispatch{}, err
	}
	row := r.db.QueryRow(ctx, `
INSERT INTO paygate_sagas.saga_command_dispatches
    (id, saga_id, step_id, merchant_id, command_name, command_id, dispatch_attempt, status, leased_by, input_payload)
VALUES
    ($1, $2, $3, $4, $5, $6, $7, 'dispatched', $8, $9)
RETURNING id, saga_id, step_id, merchant_id, command_name, command_id, dispatch_attempt, status, leased_by, leased_at,
          acked_at, nacked_at, retry_backoff_seconds, error_code, error_message, error_classification,
          input_payload, output_payload, created_at, updated_at
`, idgen.New("sdisp"), in.SagaID, in.StepID, in.MerchantID, in.CommandName, in.CommandID, in.DispatchAttempt, in.LeasedBy, inputPayload)
	return scanDispatch(row)
}

func (r *PostgresRepository) AckDispatch(ctx context.Context, in AckDispatchInput) error {
	outputPayload, err := marshalPayload(in.CommandOutput)
	if err != nil {
		return err
	}
	tag, err := r.db.Exec(ctx, `
UPDATE paygate_sagas.saga_command_dispatches
SET status = 'acked',
    acked_at = NOW(),
    output_payload = $3,
    updated_at = NOW()
WHERE step_id = $1 AND command_id = $2 AND status = 'dispatched'
`, in.StepID, in.CommandID, outputPayload)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	return nil
}

func (r *PostgresRepository) NackDispatch(ctx context.Context, in NackDispatchInput) error {
	tag, err := r.db.Exec(ctx, `
UPDATE paygate_sagas.saga_command_dispatches
SET status = 'nacked',
    nacked_at = NOW(),
    retry_backoff_seconds = $3,
    error_code = $4,
    error_message = $5,
    error_classification = $6,
    updated_at = NOW()
WHERE step_id = $1 AND command_id = $2 AND status = 'dispatched'
`, in.StepID, in.CommandID, int(in.RetryBackoff.Seconds()), in.ErrorCode, in.ErrorMessage, in.ErrorClassification)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	return nil
}

func (r *PostgresRepository) ListDispatches(ctx context.Context, merchantID, sagaID string, limit int) ([]CommandDispatch, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `
SELECT id, saga_id, step_id, merchant_id, command_name, command_id, dispatch_attempt, status, leased_by, leased_at,
       acked_at, nacked_at, retry_backoff_seconds, error_code, error_message, error_classification,
       input_payload, output_payload, created_at, updated_at
FROM paygate_sagas.saga_command_dispatches
WHERE saga_id = $1 AND merchant_id = $2
ORDER BY created_at DESC
LIMIT $3
`, sagaID, merchantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CommandDispatch
	for rows.Next() {
		item, err := scanDispatch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) RecordProcessedCommand(ctx context.Context, consumerName, commandID string, result map[string]any) (map[string]any, bool, error) {
	payload, err := marshalPayload(result)
	if err != nil {
		return nil, false, err
	}
	sum := sha256.Sum256(payload)
	hash := hex.EncodeToString(sum[:])

	var returned []byte
	err = r.db.QueryRow(ctx, `
INSERT INTO paygate_sagas.processed_commands
    (consumer_name, command_id, result_hash, result_payload)
VALUES
    ($1, $2, $3, $4)
ON CONFLICT (consumer_name, command_id) DO NOTHING
RETURNING result_payload
`, consumerName, commandID, hash, payload).Scan(&returned)
	switch {
	case err == nil:
		return result, true, nil
	case errors.Is(err, pgx.ErrNoRows):
		if err := r.db.QueryRow(ctx, `
SELECT result_payload
FROM paygate_sagas.processed_commands
WHERE consumer_name = $1 AND command_id = $2
`, consumerName, commandID).Scan(&returned); err != nil {
			return nil, false, err
		}
		var existing map[string]any
		if err := json.Unmarshal(returned, &existing); err != nil {
			return nil, false, err
		}
		return existing, false, nil
	default:
		return nil, false, err
	}
}

func (r *PostgresRepository) listSteps(ctx context.Context, sagaID string) ([]Step, error) {
	rows, err := r.db.Query(ctx, `
SELECT ss.id, ss.saga_id, si.merchant_id, ss.step_index, ss.step_name, ss.step_kind,
       ss.status, ss.command_name, ss.command_id, ss.reply_topic, ss.input_payload,
       ss.output_payload, ss.error_code, ss.error_message, ss.next_retry_at,
       ss.leased_by, ss.leased_at, ss.completed_at, ss.attempt_count, ss.max_attempts,
       ss.created_at, ss.updated_at
FROM paygate_sagas.saga_steps ss
JOIN paygate_sagas.saga_instances si ON si.id = ss.saga_id
WHERE ss.saga_id = $1
ORDER BY ss.step_index ASC
`, sagaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Step
	for rows.Next() {
		var step Step
		if err := scanStep(rows, &step); err != nil {
			return nil, err
		}
		out = append(out, step)
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanInstance(row rowScanner) (Instance, error) {
	var inst Instance
	var inputPayload []byte
	var contextPayload []byte
	var failureCode *string
	var failureReason *string
	var leasedBy *string
	err := row.Scan(
		&inst.ID, &inst.MerchantID, &inst.SagaType, &inst.Status, &inst.CorrelationID, &inst.CausationID,
		&inputPayload, &contextPayload, &inst.CurrentStepIndex, &failureCode, &failureReason,
		&leasedBy, &inst.LastLeasedAt, &inst.ReplayCount, &inst.DeadlineAt, &inst.TimeoutAt,
		&inst.StartedAt, &inst.CompletedAt, &inst.CreatedAt, &inst.UpdatedAt,
	)
	if err != nil {
		return Instance{}, err
	}
	inst.InputPayload, err = unmarshalPayload(inputPayload)
	if err != nil {
		return Instance{}, err
	}
	inst.ContextPayload, err = unmarshalPayload(contextPayload)
	if err != nil {
		return Instance{}, err
	}
	if failureCode != nil {
		inst.FailureCode = *failureCode
	}
	if failureReason != nil {
		inst.FailureReason = *failureReason
	}
	if leasedBy != nil {
		inst.LeasedBy = *leasedBy
	}
	return inst, nil
}

func scanStep(row rowScanner, step *Step) error {
	var inputPayload []byte
	var outputPayload []byte
	var errorCode *string
	var errorMessage *string
	var leasedBy *string
	err := row.Scan(
		&step.ID, &step.SagaID, &step.MerchantID, &step.StepIndex, &step.StepName, &step.StepKind,
		&step.Status, &step.CommandName, &step.CommandID, &step.ReplyTopic, &inputPayload,
		&outputPayload, &errorCode, &errorMessage, &step.NextRetryAt,
		&leasedBy, &step.LeasedAt, &step.CompletedAt, &step.AttemptCount, &step.MaxAttempts,
		&step.CreatedAt, &step.UpdatedAt,
	)
	if err != nil {
		return err
	}
	step.InputPayload, err = unmarshalPayload(inputPayload)
	if err != nil {
		return err
	}
	step.OutputPayload, err = unmarshalPayload(outputPayload)
	if err != nil {
		return err
	}
	if errorCode != nil {
		step.ErrorCode = *errorCode
	}
	if errorMessage != nil {
		step.ErrorMessage = *errorMessage
	}
	if leasedBy != nil {
		step.LeasedBy = *leasedBy
	}
	return nil
}

func scanDeadLetter(row rowScanner) (DeadLetter, error) {
	var item DeadLetter
	var payload []byte
	err := row.Scan(&item.ID, &item.SagaID, &item.StepID, &item.MerchantID, &item.DeadLetterType, &item.CommandName, &item.CommandID, &item.ErrorCode, &item.ErrorMessage, &payload, &item.CreatedAt)
	if err != nil {
		return DeadLetter{}, err
	}
	item.Payload, err = unmarshalPayload(payload)
	if err != nil {
		return DeadLetter{}, err
	}
	return item, nil
}

func scanOperatorAction(row rowScanner) (OperatorAction, error) {
	var item OperatorAction
	var payload []byte
	err := row.Scan(&item.ID, &item.SagaID, &item.MerchantID, &item.Action, &item.ActorType, &item.ActorID, &item.Reason, &payload, &item.CreatedAt)
	if err != nil {
		return OperatorAction{}, err
	}
	item.Payload, err = unmarshalPayload(payload)
	if err != nil {
		return OperatorAction{}, err
	}
	return item, nil
}

func scanDispatch(row rowScanner) (CommandDispatch, error) {
	var item CommandDispatch
	var inputPayload []byte
	var outputPayload []byte
	err := row.Scan(
		&item.ID, &item.SagaID, &item.StepID, &item.MerchantID, &item.CommandName, &item.CommandID,
		&item.DispatchAttempt, &item.Status, &item.LeasedBy, &item.LeasedAt, &item.AckedAt, &item.NackedAt,
		&item.RetryBackoffSeconds, &item.ErrorCode, &item.ErrorMessage, &item.ErrorClassification,
		&inputPayload, &outputPayload, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return CommandDispatch{}, err
	}
	item.InputPayload, err = unmarshalPayload(inputPayload)
	if err != nil {
		return CommandDispatch{}, err
	}
	item.OutputPayload, err = unmarshalPayload(outputPayload)
	if err != nil {
		return CommandDispatch{}, err
	}
	return item, nil
}

func marshalPayload(payload map[string]any) ([]byte, error) {
	if payload == nil {
		payload = map[string]any{}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func unmarshalPayload(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if payload == nil {
		payload = map[string]any{}
	}
	return payload, nil
}

func fallbackString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
