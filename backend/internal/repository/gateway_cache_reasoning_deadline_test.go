package repository

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// The five-second socket timeout is deliberately much longer than both the
// caller's budget and the generous two-second scheduling allowance below.
// These tests exercise go-redis itself, including its connection handshake.
func TestGatewayCacheReasoningDeadline(t *testing.T) {
	for _, tc := range []struct {
		phase string
		write bool
	}{
		{"tls", false},
		{"tls", true},
		{"handshake", false},
		{"handshake", true},
		{"read", false},
		{"read", true},
		{"write", true},
	} {
		t.Run(fmt.Sprintf("%s/set=%t", tc.phase, tc.write), func(t *testing.T) {
			addr, stalled := stalledReasoningRedis(t, tc.phase)
			cfg := reasoningDeadlineConfig(t, addr)
			cfg.Redis.EnableTLS = tc.phase == "tls"
			client := ProvideReasoningRedis(cfg)
			t.Cleanup(func() { _ = client.Close() })
			ordinary := InitRedis(cfg)
			t.Cleanup(func() { _ = ordinary.Close() })
			cache := ProvideGatewayCache(ordinary, client)
			if tc.phase != "handshake" && tc.phase != "tls" {
				require.NoError(t, client.Ping(context.Background()).Err())
			}

			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			start := time.Now()
			var err error
			if tc.write {
				content := "reasoning"
				if tc.phase == "write" {
					// Exceed TCP buffers while the peer stops reading after PING.
					content = strings.Repeat("r", 8<<20)
				}
				err = cache.SetReasoningContent(ctx, "item", content, time.Minute)
			} else {
				_, err = cache.GetReasoningContent(ctx, "item")
			}
			elapsed := time.Since(start)
			require.Error(t, err)
			select {
			case <-stalled:
			default:
				t.Fatal("operation did not reach the stalled socket phase")
			}
			t.Logf("%s SET=%t: budget=150ms elapsed=%s error=%v", tc.phase, tc.write, elapsed, err)
			require.Less(t, elapsed, 2*time.Second, "Redis socket ignored the caller deadline")
		})
	}
}

func reasoningDeadlineConfig(t *testing.T, addr string) *config.Config {
	t.Helper()
	host, portText, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	return &config.Config{Redis: config.RedisConfig{
		Host: host, Port: port,
		DialTimeoutSeconds: 5, ReadTimeoutSeconds: 5, WriteTimeoutSeconds: 5,
		PoolSize: 1,
	}}
}

// stalledReasoningRedis accepts real TCP sockets. It answers startup commands
// using RESP2 fallback, then deliberately stops reading or replying. Cleanup
// closes and joins every accepted connection, even when an assertion fails.
func stalledReasoningRedis(t *testing.T, phase string) (string, <-chan struct{}) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	stop := make(chan struct{})
	stalled := make(chan struct{})
	var markStalled sync.Once
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer func() { _ = conn.Close() }()
				closed := make(chan struct{})
				go func() {
					select {
					case <-stop:
						_ = conn.Close()
					case <-closed:
					}
				}()
				defer close(closed)
				if phase == "tls" {
					markStalled.Do(func() { close(stalled) })
					<-stop
					return
				}
				rd := bufio.NewReader(conn)
				for {
					args, err := readReasoningRedisCommand(rd)
					if err != nil {
						return
					}
					cmd := strings.ToLower(args[0])
					if (phase == "handshake" && cmd == "hello") ||
						(phase == "read" && (cmd == "get" || cmd == "set")) {
						markStalled.Do(func() { close(stalled) })
						<-stop
						return
					}
					response := "+OK\r\n"
					switch cmd {
					case "hello":
						response = "-ERR unknown command 'hello'\r\n"
					case "ping":
						response = "+PONG\r\n"
					}
					if _, err := io.WriteString(conn, response); err != nil {
						return
					}
					if phase == "write" && cmd == "ping" {
						markStalled.Do(func() { close(stalled) })
						<-stop
						return
					}
				}
			}()
		}
	}()
	t.Cleanup(func() {
		close(stop)
		_ = listener.Close()
		workers.Wait()
	})
	return listener.Addr().String(), stalled
}

func readReasoningRedisCommand(rd *bufio.Reader) ([]string, error) {
	line, err := rd.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if len(line) < 3 || line[0] != '*' {
		return nil, fmt.Errorf("invalid RESP array %q", line)
	}
	n, err := strconv.Atoi(strings.TrimSpace(line[1:]))
	if err != nil || n <= 0 {
		return nil, fmt.Errorf("invalid RESP array length %q", line)
	}
	args := make([]string, n)
	for i := range args {
		line, err = rd.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if len(line) < 3 || line[0] != '$' {
			return nil, fmt.Errorf("invalid RESP bulk string %q", line)
		}
		size, err := strconv.Atoi(strings.TrimSpace(line[1:]))
		if err != nil || size < 0 {
			return nil, fmt.Errorf("invalid RESP bulk length %q", line)
		}
		value := make([]byte, size+2)
		if _, err = io.ReadFull(rd, value); err != nil {
			return nil, err
		}
		args[i] = string(value[:size])
	}
	return args, nil
}

func TestGatewayCacheReasoningPoolDeadline(t *testing.T) {
	addr, stalled := stalledReasoningRedis(t, "read")
	client := ProvideReasoningRedis(reasoningDeadlineConfig(t, addr))
	cache := ProvideGatewayCache(client.Client, client)
	require.NoError(t, client.Ping(context.Background()).Err())

	firstCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		_, _ = cache.GetReasoningContent(firstCtx, "first")
	}()
	t.Cleanup(func() {
		_ = client.Close()
		<-firstDone
	})
	select {
	case <-stalled:
	case <-time.After(2 * time.Second):
		t.Fatal("first request did not occupy the pool connection")
	}

	ctx, cancelSecond := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancelSecond()
	start := time.Now()
	_, err := cache.GetReasoningContent(ctx, "second")
	elapsed := time.Since(start)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, elapsed, 2*time.Second)
	select {
	case <-firstDone:
		t.Fatal("the occupied connection was released before pool-wait deadline")
	default:
	}
	t.Logf("pool wait: budget=150ms elapsed=%s", elapsed)
}

var _ service.GatewayCache = (*gatewayCache)(nil)
