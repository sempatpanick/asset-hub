package redisqueue

import (
	"context"
	"github.com/redis/go-redis/v9"
	"time"
)

type Queue struct{ Client *redis.Client }

const queueKey = "assethub:upload:pending"

func (q *Queue) Heartbeat(ctx context.Context) error {
	return q.Client.Set(ctx, "assethub:worker:heartbeat", "alive", 30*time.Second).Err()
}

func (q *Queue) Push(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	members := make([]redis.Z, len(ids))
	for i, id := range ids {
		members[i] = redis.Z{Score: float64(time.Now().UnixMicro()) + float64(i), Member: id}
	}
	return q.Client.ZAddNX(ctx, queueKey, members...).Err()
}
func (q *Queue) Pop(ctx context.Context) (string, error) {
	values, err := q.Client.ZPopMin(ctx, queueKey, 1).Result()
	if err != nil {
		return "", err
	}
	if len(values) == 0 {
		return "", nil
	}
	return values[0].Member.(string), nil
}

type TelegramGate struct{ Client *redis.Client }

const gateKey = "assethub:telegram:send-gate"

func (g *TelegramGate) Wait(ctx context.Context) error {
	for {
		ok, err := g.Client.SetNX(ctx, gateKey, "1", 3*time.Second).Result()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
func (g *TelegramGate) Delay(ctx context.Context, d time.Duration) error {
	return g.Client.Eval(ctx, `local t=redis.call('PTTL',KEYS[1]); if t<tonumber(ARGV[1]) then redis.call('SET',KEYS[1],'1','PX',ARGV[1]) end; return 1`, []string{gateKey}, d.Milliseconds()).Err()
}
