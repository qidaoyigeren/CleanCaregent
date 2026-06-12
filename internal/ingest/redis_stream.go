package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"CleanCaregent/internal/platform/id"
	"CleanCaregent/internal/service"

	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type StreamConfig struct {
	Stream     string
	Group      string
	Consumer   string
	DeadLetter string
	Block      time.Duration
	ClaimIdle  time.Duration
	BatchSize  int64
	MaxRetries int
}

type EnqueueResult struct {
	JobID     string `json:"job_id"`
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}

type Publisher interface {
	Enqueue(ctx context.Context, request service.IngestDocumentRequest) (EnqueueResult, error)
}

type KnowledgeIngester interface {
	Ingest(ctx context.Context, request service.IngestDocumentRequest) (service.IngestDocumentResult, error)
}

type RedisStream struct {
	client   goredis.UniversalClient
	ingester KnowledgeIngester
	config   StreamConfig
	logger   *zap.Logger
}

type streamJob struct {
	JobID   string                        `json:"job_id"`
	Attempt int                           `json:"attempt"`
	Request service.IngestDocumentRequest `json:"request"`
}

func NewRedisStream(
	client goredis.UniversalClient,
	ingester KnowledgeIngester,
	config StreamConfig,
	logger *zap.Logger,
) *RedisStream {
	if logger == nil {
		logger = zap.NewNop()
	}
	if config.BatchSize < 1 {
		config.BatchSize = 8
	}
	if config.MaxRetries < 1 {
		config.MaxRetries = 3
	}
	if config.Block <= 0 {
		config.Block = 2 * time.Second
	}
	if config.ClaimIdle <= 0 {
		config.ClaimIdle = time.Minute
	}
	return &RedisStream{client: client, ingester: ingester, config: config, logger: logger}
}

func (s *RedisStream) EnsureGroup(ctx context.Context) error {
	if s == nil || s.client == nil {
		return errors.New("redis ingest stream is not configured")
	}
	err := s.client.XGroupCreateMkStream(ctx, s.config.Stream, s.config.Group, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("create ingest consumer group: %w", err)
	}
	return nil
}

func (s *RedisStream) Enqueue(
	ctx context.Context,
	request service.IngestDocumentRequest,
) (EnqueueResult, error) {
	if s == nil || s.client == nil {
		return EnqueueResult{}, errors.New("redis ingest stream is not configured")
	}
	job := streamJob{JobID: id.New("kbjob"), Request: request}
	payload, err := json.Marshal(job)
	if err != nil {
		return EnqueueResult{}, fmt.Errorf("encode ingest job: %w", err)
	}
	messageID, err := s.client.XAdd(ctx, &goredis.XAddArgs{
		Stream: s.config.Stream,
		Values: map[string]any{"payload": string(payload)},
	}).Result()
	if err != nil {
		return EnqueueResult{}, fmt.Errorf("enqueue ingest job: %w", err)
	}
	return EnqueueResult{JobID: job.JobID, MessageID: messageID, Status: "queued"}, nil
}

func (s *RedisStream) Run(ctx context.Context) error {
	if s == nil || s.client == nil || s.ingester == nil {
		return errors.New("redis ingest worker is not configured")
	}
	if err := s.EnsureGroup(ctx); err != nil {
		return err
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		s.claimStale(ctx)
		streams, err := s.client.XReadGroup(ctx, &goredis.XReadGroupArgs{
			Group:    s.config.Group,
			Consumer: s.config.Consumer,
			Streams:  []string{s.config.Stream, ">"},
			Count:    s.config.BatchSize,
			Block:    s.config.Block,
		}).Result()
		if errors.Is(err, goredis.Nil) {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			s.logger.Warn("read knowledge ingest stream failed", zap.Error(err))
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(250 * time.Millisecond):
			}
			continue
		}
		for _, stream := range streams {
			for _, message := range stream.Messages {
				s.processMessage(ctx, message)
			}
		}
	}
}

func (s *RedisStream) claimStale(ctx context.Context) {
	messages, _, err := s.client.XAutoClaim(ctx, &goredis.XAutoClaimArgs{
		Stream:   s.config.Stream,
		Group:    s.config.Group,
		Consumer: s.config.Consumer,
		MinIdle:  s.config.ClaimIdle,
		Start:    "0-0",
		Count:    s.config.BatchSize,
	}).Result()
	if err != nil && !errors.Is(err, goredis.Nil) {
		s.logger.Debug("claim stale ingest messages failed", zap.Error(err))
		return
	}
	for _, message := range messages {
		s.processMessage(ctx, message)
	}
}

func (s *RedisStream) processMessage(ctx context.Context, message goredis.XMessage) {
	job, err := decodeJob(message)
	if err != nil {
		s.moveToDeadLetter(ctx, message.ID, streamJob{}, "INVALID_JOB_PAYLOAD", err)
		return
	}
	result, ingestErr := s.ingester.Ingest(ctx, job.Request)
	if ingestErr == nil {
		s.ackAndDelete(ctx, message.ID)
		s.logger.Info("knowledge ingest job completed",
			zap.String("job_id", job.JobID),
			zap.String("doc_id", result.DocID),
			zap.Int("chunk_count", result.ChunkCount),
		)
		return
	}
	job.Attempt++
	if job.Attempt >= s.config.MaxRetries {
		s.moveToDeadLetter(ctx, message.ID, job, "INGEST_RETRIES_EXHAUSTED", ingestErr)
		return
	}
	payload, encodeErr := json.Marshal(job)
	if encodeErr != nil {
		s.moveToDeadLetter(ctx, message.ID, job, "JOB_REENCODE_FAILED", encodeErr)
		return
	}
	if _, retryErr := s.client.XAdd(ctx, &goredis.XAddArgs{
		Stream: s.config.Stream,
		Values: map[string]any{"payload": string(payload)},
	}).Result(); retryErr != nil {
		s.logger.Error("requeue knowledge ingest job failed",
			zap.String("job_id", job.JobID),
			zap.Error(retryErr),
		)
		return
	}
	s.ackAndDelete(ctx, message.ID)
	s.logger.Warn("knowledge ingest job requeued",
		zap.String("job_id", job.JobID),
		zap.Int("attempt", job.Attempt),
		zap.Error(ingestErr),
	)
}

func decodeJob(message goredis.XMessage) (streamJob, error) {
	raw, ok := message.Values["payload"]
	if !ok {
		return streamJob{}, errors.New("payload field is missing")
	}
	var job streamJob
	if err := json.Unmarshal([]byte(fmt.Sprint(raw)), &job); err != nil {
		return streamJob{}, err
	}
	if job.JobID == "" || job.Request.DocID == "" {
		return streamJob{}, errors.New("job_id and request.doc_id are required")
	}
	return job, nil
}

func (s *RedisStream) moveToDeadLetter(
	ctx context.Context,
	messageID string,
	job streamJob,
	code string,
	cause error,
) {
	payload, _ := json.Marshal(job)
	_, err := s.client.XAdd(ctx, &goredis.XAddArgs{
		Stream: s.config.DeadLetter,
		Values: map[string]any{
			"payload":              string(payload),
			"source_message_id":    messageID,
			"error_code":           code,
			"error_message":        cause.Error(),
			"failed_at_unix_milli": time.Now().UTC().UnixMilli(),
		},
	}).Result()
	if err != nil {
		s.logger.Error("write knowledge ingest dead letter failed",
			zap.String("message_id", messageID),
			zap.Error(err),
		)
		return
	}
	s.ackAndDelete(ctx, messageID)
	s.logger.Error("knowledge ingest job moved to dead letter",
		zap.String("job_id", job.JobID),
		zap.String("error_code", code),
		zap.Error(cause),
	)
}

func (s *RedisStream) ackAndDelete(ctx context.Context, messageID string) {
	if err := s.client.XAck(ctx, s.config.Stream, s.config.Group, messageID).Err(); err != nil {
		s.logger.Warn("ack knowledge ingest message failed", zap.String("message_id", messageID), zap.Error(err))
		return
	}
	if err := s.client.XDel(ctx, s.config.Stream, messageID).Err(); err != nil {
		s.logger.Debug("delete acknowledged ingest message failed", zap.String("message_id", messageID), zap.Error(err))
	}
}
