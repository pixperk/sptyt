package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	client *redis.Client
}

type CachedTrack struct {
	Name    string   `json:"name"`
	Artists []string `json:"artists"`
}

type CachedRedirect struct {
	URL string `json:"url"`
}

func NewRedisCache(connString string) (*RedisCache, error) {
	var client *redis.Client

	if opt, err := redis.ParseURL(connString); err == nil {
		opt.PoolSize = 50
		opt.MinIdleConns = 10
		opt.MaxRetries = 3
		opt.DialTimeout = 5 * time.Second
		opt.ReadTimeout = 3 * time.Second
		opt.WriteTimeout = 3 * time.Second
		client = redis.NewClient(opt)
	} else {
		client = redis.NewClient(&redis.Options{
			Addr:         connString,
			PoolSize:     50,
			MinIdleConns: 10,
			MaxRetries:   3,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return &RedisCache{client: client}, nil
}

func (r *RedisCache) GetTrack(ctx context.Context, trackID string) (*CachedTrack, error) {
	key := "track:" + trackID
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var track CachedTrack
	if err := json.Unmarshal([]byte(val), &track); err != nil {
		return nil, err
	}

	return &track, nil
}

func (r *RedisCache) SetTrack(ctx context.Context, trackID string, track *CachedTrack, ttl time.Duration) error {
	key := "track:" + trackID
	data, err := json.Marshal(track)
	if err != nil {
		return err
	}

	return r.client.Set(ctx, key, data, ttl).Err()
}

func (r *RedisCache) GetYouTubeMVURL(ctx context.Context, trackID string) (string, error) {
	key := "youtube:mv:" + trackID
	return r.client.Get(ctx, key).Result()
}

func (r *RedisCache) SetYouTubeMVURL(ctx context.Context, trackID string, url string, ttl time.Duration) error {
	key := "youtube:mv:" + trackID
	return r.client.Set(ctx, key, url, ttl).Err()
}

func (r *RedisCache) GetYouTubeLyricsURL(ctx context.Context, trackID string) (string, error) {
	key := "youtube:lyrics:" + trackID
	return r.client.Get(ctx, key).Result()
}

func (r *RedisCache) SetYouTubeLyricsURL(ctx context.Context, trackID string, url string, ttl time.Duration) error {
	key := "youtube:lyrics:" + trackID
	return r.client.Set(ctx, key, url, ttl).Err()
}

func (r *RedisCache) GetGeniusURL(ctx context.Context, trackID string) (string, error) {
	key := "genius:" + trackID
	return r.client.Get(ctx, key).Result()
}

func (r *RedisCache) SetGeniusURL(ctx context.Context, trackID string, url string, ttl time.Duration) error {
	key := "genius:" + trackID
	return r.client.Set(ctx, key, url, ttl).Err()
}

func (r *RedisCache) GetClient() *redis.Client {
	return r.client
}

func (r *RedisCache) Close() error {
	return r.client.Close()
}
