package id

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// ──────────────────────────────────────────────
//  Crockford Base32 encoding (for ULID)
// ──────────────────────────────────────────────

const crockfordAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// ──────────────────────────────────────────────
//  ULID — Universally Unique Lexicographically Sortable Identifier (128-bit)
// ──────────────────────────────────────────────

var (
	ulidMu      sync.Mutex
	ulidLastMs  int64
	ulidEntropy uint64
)

// NewULID generates a K-sortable 26-character ULID string.
// ULIDs are timestamp-prefixed to eliminate B-Tree index fragmentation in databases.
func NewULID() string {
	ulidMu.Lock()
	defer ulidMu.Unlock()

	nowMs := time.Now().UnixMilli()

	if nowMs == ulidLastMs {
		ulidEntropy++
	} else {
		ulidLastMs = nowMs
		var buf [8]byte
		_, _ = io.ReadFull(rand.Reader, buf[:])
		ulidEntropy = binary.BigEndian.Uint64(buf[:]) >> 16 // 48-bit random
	}

	var result [26]byte

	// Encode timestamp (first 10 characters — 48-bit millisecond timestamp)
	ts := uint64(nowMs)
	for i := 9; i >= 0; i-- {
		result[i] = crockfordAlphabet[ts&0x1F]
		ts >>= 5
	}

	// Encode entropy (last 16 characters — 80-bit randomness)
	entropy := ulidEntropy
	for i := 25; i >= 10; i-- {
		result[i] = crockfordAlphabet[entropy&0x1F]
		entropy >>= 5
	}

	return string(result[:])
}

// ──────────────────────────────────────────────
//  Snowflake — 64-bit Distributed Integer ID
// ──────────────────────────────────────────────

const (
	snowflakeEpoch        = int64(1704067200000) // 2024-01-01 00:00:00 UTC in millis
	snowflakeNodeBits     = 10
	snowflakeSequenceBits = 12
	snowflakeMaxNode      = (1 << snowflakeNodeBits) - 1
	snowflakeMaxSequence  = (1 << snowflakeSequenceBits) - 1
)

// SnowflakeNode generates unique 64-bit integer IDs using a Snowflake-style algorithm.
// Each node can produce up to 4,096 unique IDs per millisecond.
type SnowflakeNode struct {
	mu       sync.Mutex
	nodeID   int64
	lastMs   int64
	sequence int64
}

// NewSnowflakeNode creates a SnowflakeNode with the given node ID (0–1023).
func NewSnowflakeNode(nodeID int64) (*SnowflakeNode, error) {
	if nodeID < 0 || nodeID > snowflakeMaxNode {
		return nil, fmt.Errorf("id: snowflake node ID must be between 0 and %d, got %d", snowflakeMaxNode, nodeID)
	}
	return &SnowflakeNode{nodeID: nodeID}, nil
}

// NextID generates the next unique 64-bit snowflake ID.
func (n *SnowflakeNode) NextID() int64 {
	n.mu.Lock()
	defer n.mu.Unlock()

	nowMs := time.Now().UnixMilli()

	if nowMs == n.lastMs {
		n.sequence = (n.sequence + 1) & snowflakeMaxSequence
		if n.sequence == 0 {
			// Sequence overflow — spin-wait until next millisecond
			for nowMs <= n.lastMs {
				nowMs = time.Now().UnixMilli()
			}
		}
	} else {
		n.sequence = 0
	}

	n.lastMs = nowMs

	id := ((nowMs - snowflakeEpoch) << (snowflakeNodeBits + snowflakeSequenceBits)) |
		(n.nodeID << snowflakeSequenceBits) |
		n.sequence

	return id
}

// defaultSnowflakeNode is a package-level singleton for simple usage.
var defaultSnowflakeNode *SnowflakeNode

func init() {
	// Use a random node ID for the default singleton.
	var buf [2]byte
	_, _ = io.ReadFull(rand.Reader, buf[:])
	nodeID := int64(binary.BigEndian.Uint16(buf[:])) & snowflakeMaxNode
	defaultSnowflakeNode, _ = NewSnowflakeNode(nodeID)
}

// NewSnowflake generates a 64-bit snowflake ID using the default node.
func NewSnowflake() int64 {
	return defaultSnowflakeNode.NextID()
}

// ──────────────────────────────────────────────
//  Stripe-Style Prefixed IDs
// ──────────────────────────────────────────────

// NewPrefixed generates a Stripe-style prefixed ID (e.g. "usr_01JEX89K2P...", "ord_01JEX89K2P...").
func NewPrefixed(prefix string) string {
	return prefix + "_" + NewULID()
}

// ──────────────────────────────────────────────
//  UUID v4 (Random) & UUID v7 (Time-Ordered)
// ──────────────────────────────────────────────

var (
	uuidV7Mu       sync.Mutex
	uuidV7LastMs   uint64
	uuidV7Sequence uint16
)

// NewUUID generates a standard RFC 4122 UUID v4 (128-bit random).
func NewUUID() string {
	var buf [16]byte
	_, _ = io.ReadFull(rand.Reader, buf[:])

	// Set version 4 bits
	buf[6] = (buf[6] & 0x0f) | 0x40
	// Set variant 10 bits
	buf[8] = (buf[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}

// NewUUIDv7 generates an RFC 9562 UUID v7 (time-ordered, k-sortable).
func NewUUIDv7() string {
	uuidV7Mu.Lock()
	defer uuidV7Mu.Unlock()

	var buf [16]byte
	_, _ = io.ReadFull(rand.Reader, buf[:])

	nowMs := uint64(time.Now().UnixMilli())
	if nowMs <= uuidV7LastMs {
		nowMs = uuidV7LastMs
		uuidV7Sequence++
		if uuidV7Sequence > 0x0fff {
			for nowMs <= uuidV7LastMs {
				nowMs = uint64(time.Now().UnixMilli())
			}
			uuidV7Sequence = binary.BigEndian.Uint16(buf[6:8]) & 0x0fff
		}
	} else {
		uuidV7Sequence = binary.BigEndian.Uint16(buf[6:8]) & 0x0fff
	}
	uuidV7LastMs = nowMs

	// Encode 48-bit Unix timestamp in milliseconds (bytes 0-5)
	buf[0] = byte(nowMs >> 40)
	buf[1] = byte(nowMs >> 32)
	buf[2] = byte(nowMs >> 24)
	buf[3] = byte(nowMs >> 16)
	buf[4] = byte(nowMs >> 8)
	buf[5] = byte(nowMs)

	// Encode the monotonic 12-bit rand_a field and version 7 bits.
	buf[6] = byte(uuidV7Sequence>>8) | 0x70
	buf[7] = byte(uuidV7Sequence)
	// Set variant 10 bits (byte 8, upper 2 bits)
	buf[8] = (buf[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}

// ──────────────────────────────────────────────
//  Auto-ID Resolution (used by ORM auto_id tag)
// ──────────────────────────────────────────────

// GenerateAutoID produces an ID value based on the strategy string from struct tags.
// Supported strategies: "ulid", "snowflake", "uuid", "uuidv7", "prefix:xxx".
func GenerateAutoID(strategy string) any {
	switch {
	case strategy == "ulid":
		return NewULID()
	case strategy == "snowflake":
		return NewSnowflake()
	case strategy == "uuid":
		return NewUUID()
	case strategy == "uuidv7":
		return NewUUIDv7()
	case strings.HasPrefix(strategy, "prefix:"):
		prefix := strings.TrimPrefix(strategy, "prefix:")
		return NewPrefixed(prefix)
	default:
		return NewULID()
	}
}
