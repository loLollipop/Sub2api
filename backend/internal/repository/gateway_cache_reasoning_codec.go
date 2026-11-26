package repository

import (
	"bytes"
	"compress/gzip"
	"io"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// reasoning 缓存值的透明压缩（只改 gatewayCache 的存取，key 名与 TTL 都不变）。
//
// 为什么：DeepSeek 原生 reasoning 条目（key 形如
// reasoning_content:deepseek-native-v1:<scopeHash>:<itemHash>，值是
// {"content":[reasoning_text...],"encrypted_digest":"..."}）保存可压缩的文本。
// scopeHash 继续由本地用户、会话、模型共同生成，不改变隔离键或 Redis 客户端。
//
// 格式：压缩值就是一个标准 gzip 流（以 1f 8b 08 开头），其余值保持明文。
//   - 读：以 1f 8b 08 识别。合法 UTF-8（JSON、summary 文本）不可能以 0x1f 0x8b 开头
//     （0x8b 是续字节），所以不会与旧版本写入的明文冲突；
//   - 写：明文若恰好以该前缀开头（只可能是任意二进制输入）也强制压缩，使任意字符串
//     都能无损往返，读侧不需要猜。
//
// 兼容（蓝绿 / 回滚）：压缩引入前的本地 v2.1.74 读到压缩值时，DeepSeek 原生读侧
// （prepareDeepSeekNativeReasoningReplay）对它 json.Unmarshal 失败，按未命中处理，随后
// 原有 replay helper 补占位符——与 Redis 被清空时一样，只是
// 那一轮丢掉这条历史 reasoning，不报错、不 400、不 panic。
// 所以只压缩这一族：老族 reasoning_content:<itemID> 由 reasoningContentByID 把读到的
// 字符串原样当 reasoning 文本塞进上游请求，旧进程读到二进制会把乱码当历史 reasoning
// 发给上游；该家族因此保持明文。

const (
	// reasoningCompressMinBytes：小于该长度的值收益可忽略（gzip 头尾约 20 字节），直接存原文。
	reasoningCompressMinBytes = 512
	// reasoningMaxDecodedBytes：解压输出上限（防压缩炸弹）。DeepSeek 原生条目写入上限是
	// 2MiB（deepSeekNativeReasoningMaxBytes），留一倍余量；超过按未命中处理。
	reasoningMaxDecodedBytes = 4 << 20
)

// 沿用来源修复的 gzip.DefaultCompression(6)，复用编解码器以减少逐条分配。
var (
	reasoningGzipWriters = sync.Pool{New: func() any { return gzip.NewWriter(io.Discard) }}
	reasoningGzipReaders sync.Pool // *gzip.Reader；池空时由 gzip.NewReader 现建
)

func hasGzipMagic(s string) bool {
	return len(s) >= 3 && s[0] == 0x1f && s[1] == 0x8b && s[2] == 0x08
}

// reasoningCompressibleKey 只放行 DeepSeek 原生族，原因见文件头注释。
func reasoningCompressibleKey(itemID string) bool {
	return strings.HasPrefix(itemID, service.DeepSeekNativeReasoningCacheKeyPrefix)
}

// encodeReasoningContent 返回应当写进 Redis 的字符串：够大、可压缩且压缩后确实更小
// 才存 gzip，否则存原文。一次写入只压缩一次。
func encodeReasoningContent(itemID, content string) string {
	// 明文长得像 gzip 头时必须包一层，否则读侧会把它当压缩流。
	mustWrap := hasGzipMagic(content)
	if !mustWrap && (len(content) < reasoningCompressMinBytes || !reasoningCompressibleKey(itemID)) {
		return content
	}
	z, ok := gzipReasoning(content)
	if !ok || (!mustWrap && len(z) >= len(content)) {
		return content
	}
	return z
}

func gzipReasoning(content string) (string, bool) {
	zw, ok := reasoningGzipWriters.Get().(*gzip.Writer)
	if !ok {
		return "", false
	}
	var buf bytes.Buffer
	buf.Grow(len(content)/2 + 64)
	zw.Reset(&buf)
	_, werr := zw.Write([]byte(content))
	cerr := zw.Close()
	reasoningGzipWriters.Put(zw)
	if werr != nil || cerr != nil {
		return "", false
	}
	return buf.String(), true
}

// decodeReasoningContent 还原 encodeReasoningContent 的结果。ok=false 表示「当作未命中」：
// 压缩流损坏/被截断、或解压后超过 reasoningMaxDecodedBytes。
func decodeReasoningContent(stored string) (string, bool) {
	if !hasGzipMagic(stored) {
		return stored, true // 旧版本写入的明文，或未压缩的小值
	}
	src := strings.NewReader(stored)
	zr, _ := reasoningGzipReaders.Get().(*gzip.Reader)
	if zr == nil {
		var err error
		if zr, err = gzip.NewReader(src); err != nil {
			return "", false
		}
	} else if err := zr.Reset(src); err != nil {
		reasoningGzipReaders.Put(zr)
		return "", false
	}
	defer reasoningGzipReaders.Put(zr)
	// 多读 1 字节用来区分「恰好等于上限」和「超过上限」。读到 EOF 时 gzip 会校验 CRC 与长度。
	out, err := io.ReadAll(io.LimitReader(zr, reasoningMaxDecodedBytes+1))
	if err != nil || len(out) > reasoningMaxDecodedBytes {
		return "", false
	}
	return string(out), true
}
