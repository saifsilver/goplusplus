package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultRedisIdempotencyPrefix  = "gpp:idempotency:"
	defaultRedisTransactionRetries = 8
)

type RedisIdempotencyConfig struct {
	Prefix             string
	TransactionRetries int
}

// RedisIdempotencyStore coordinates idempotency across application instances.
type RedisIdempotencyStore struct {
	client  redis.UniversalClient
	prefix  string
	retries int
}

type redisIdempotencyRecord struct {
	State       IdempotencyState     `json:"state"`
	Fingerprint string               `json:"fingerprint"`
	Owner       string               `json:"owner,omitempty"`
	Response    *IdempotencyResponse `json:"response,omitempty"`
}

func NewRedisIdempotencyStore(client redis.UniversalClient, config RedisIdempotencyConfig) (*RedisIdempotencyStore, error) {
	if client == nil {
		return nil, errors.New("idempotency: Redis client is required")
	}
	if config.Prefix == "" {
		config.Prefix = defaultRedisIdempotencyPrefix
	}
	if strings.ContainsAny(config.Prefix, "\r\n") {
		return nil, errors.New("idempotency: Redis prefix contains invalid characters")
	}
	if config.TransactionRetries <= 0 {
		config.TransactionRetries = defaultRedisTransactionRetries
	}
	return &RedisIdempotencyStore{client: client, prefix: config.Prefix, retries: config.TransactionRetries}, nil
}

func (s *RedisIdempotencyStore) Claim(
	ctx context.Context, key, fingerprint, owner string, ttl time.Duration,
) (IdempotencyClaim, error) {
	if err := validateIdempotencyStoreInput(ctx, key, fingerprint, owner, ttl); err != nil {
		return IdempotencyClaim{}, err
	}
	record := redisIdempotencyRecord{State: IdempotencyPending, Fingerprint: fingerprint, Owner: owner}
	encoded, err := json.Marshal(record)
	if err != nil {
		return IdempotencyClaim{}, fmt.Errorf("idempotency: encode Redis claim: %w", err)
	}
	redisKey := s.prefix + key
	for range s.retries {
		acquired, err := s.client.SetNX(ctx, redisKey, encoded, ttl).Result()
		if err != nil {
			return IdempotencyClaim{}, fmt.Errorf("idempotency: claim Redis key: %w", err)
		}
		if acquired {
			return IdempotencyClaim{Acquired: true, State: IdempotencyPending, Fingerprint: fingerprint}, nil
		}
		existing, err := s.read(ctx, redisKey)
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			return IdempotencyClaim{}, err
		}
		return redisClaim(existing), nil
	}
	return IdempotencyClaim{}, errors.New("idempotency: Redis claim retry limit exceeded")
}

func (s *RedisIdempotencyStore) Complete(
	ctx context.Context, key, fingerprint, owner string, response IdempotencyResponse, ttl time.Duration,
) error {
	if err := validateIdempotencyStoreInput(ctx, key, fingerprint, owner, ttl); err != nil {
		return err
	}
	redisKey := s.prefix + key
	return s.update(ctx, redisKey, func(record redisIdempotencyRecord) (*redisIdempotencyRecord, error) {
		if record.State != IdempotencyPending || record.Owner != owner || record.Fingerprint != fingerprint {
			return nil, ErrIdempotencyOwnershipLost
		}
		cloned := cloneIdempotencyResponse(response)
		return &redisIdempotencyRecord{
			State: IdempotencyComplete, Fingerprint: fingerprint, Response: &cloned,
		}, nil
	}, ttl)
}

func (s *RedisIdempotencyStore) Release(ctx context.Context, key, owner string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if key == "" || owner == "" {
		return errors.New("idempotency: Redis key and owner are required")
	}
	redisKey := s.prefix + key
	for range s.retries {
		err := s.client.Watch(ctx, func(tx *redis.Tx) error {
			record, err := readRedisIdempotencyRecord(ctx, tx, redisKey)
			if errors.Is(err, redis.Nil) {
				return nil
			}
			if err != nil {
				return err
			}
			if record.State != IdempotencyPending || record.Owner != owner {
				return ErrIdempotencyOwnershipLost
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Del(ctx, redisKey)
				return nil
			})
			return err
		}, redisKey)
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		if err != nil {
			return fmt.Errorf("idempotency: release Redis claim: %w", err)
		}
		return nil
	}
	return errors.New("idempotency: Redis release retry limit exceeded")
}

func (s *RedisIdempotencyStore) update(
	ctx context.Context,
	key string,
	mutate func(redisIdempotencyRecord) (*redisIdempotencyRecord, error),
	ttl time.Duration,
) error {
	for range s.retries {
		err := s.client.Watch(ctx, func(tx *redis.Tx) error {
			record, err := readRedisIdempotencyRecord(ctx, tx, key)
			if errors.Is(err, redis.Nil) {
				return ErrIdempotencyOwnershipLost
			}
			if err != nil {
				return err
			}
			updated, err := mutate(record)
			if err != nil {
				return err
			}
			encoded, err := json.Marshal(updated)
			if err != nil {
				return err
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, key, encoded, ttl)
				return nil
			})
			return err
		}, key)
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		if err != nil {
			return fmt.Errorf("idempotency: update Redis claim: %w", err)
		}
		return nil
	}
	return errors.New("idempotency: Redis transaction retry limit exceeded")
}

func (s *RedisIdempotencyStore) read(ctx context.Context, key string) (redisIdempotencyRecord, error) {
	return readRedisIdempotencyRecord(ctx, s.client, key)
}

type redisGetter interface {
	Get(ctx context.Context, key string) *redis.StringCmd
}

func readRedisIdempotencyRecord(ctx context.Context, client redisGetter, key string) (redisIdempotencyRecord, error) {
	data, err := client.Get(ctx, key).Bytes()
	if err != nil {
		return redisIdempotencyRecord{}, err
	}
	var record redisIdempotencyRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return redisIdempotencyRecord{}, fmt.Errorf("idempotency: decode Redis record: %w", err)
	}
	if record.State != IdempotencyPending && record.State != IdempotencyComplete {
		return redisIdempotencyRecord{}, errors.New("idempotency: Redis record has invalid state")
	}
	return record, nil
}

func redisClaim(record redisIdempotencyRecord) IdempotencyClaim {
	claim := IdempotencyClaim{State: record.State, Fingerprint: record.Fingerprint}
	if record.Response != nil {
		response := cloneIdempotencyResponse(*record.Response)
		claim.Response = &response
	}
	return claim
}
