package types

import (
	"encoding/binary"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"
)

const (
	// ModuleName defines the module name
	ModuleName = "liquidityvault"

	// StoreKey defines the primary module store key
	StoreKey = ModuleName

	// MemStoreKey defines the in-memory store key
	MemStoreKey = "mem_liquidityvault"
)

var (
	// ParamsKey is the store key for the module parameters.
	ParamsKey = []byte("p_liquidityvault")

	// VaultKeyPrefix is the store prefix for validator vaults, keyed by
	// validator operator address.
	VaultKeyPrefix = []byte{0x01}

	// PendingWithdrawalKeyPrefix is the store prefix for the time-ordered
	// pending withdrawal queue, keyed by completion time then validator
	// operator address.
	PendingWithdrawalKeyPrefix = []byte{0x02}

	// TokensSnapshotKeyPrefix is the store prefix for the transient
	// validator-token snapshots taken by the stake cap hooks, keyed by
	// validator operator address. Entries only live within a single
	// delegation operation: written by the Before* hooks and deleted by
	// AfterDelegationModified.
	TokensSnapshotKeyPrefix = []byte{0x03}

	// PoolKeyPrefix is the store prefix for registered pools, keyed by pool
	// id.
	PoolKeyPrefix = []byte{0x04}

	// NextPoolIDKey is the store key for the next pool id counter.
	NextPoolIDKey = []byte{0x05}

	// PositionKeyPrefix is the store prefix for validator pool positions,
	// keyed by length-prefixed validator operator address then pool id.
	PositionKeyPrefix = []byte{0x06}

	// PoolTotalSharesKeyPrefix is the store prefix for per-pool total
	// internal shares, keyed by pool id.
	PoolTotalSharesKeyPrefix = []byte{0x07}

	// PendingDeallocationKeyPrefix is the store prefix for the time-ordered
	// deallocation queue, keyed by completion time, then length-prefixed
	// validator operator address, then pool id.
	PendingDeallocationKeyPrefix = []byte{0x08}

	// PendingSharesKeyPrefix is the store prefix for the per-(validator,
	// pool) sum of shares sitting in the deallocation queue, keyed like
	// PositionKeyPrefix. It always equals the queue's per-position sums
	// (enforced by the module invariant).
	PendingSharesKeyPrefix = []byte{0x09}

	// ValuePostHistoryKeyPrefix is the store prefix for validators' rolling
	// value-post windows, keyed by validator operator address.
	ValuePostHistoryKeyPrefix = []byte{0x0A}

	// ValuePostScheduleKeyPrefix is the store prefix for the time-ordered
	// value-post schedule, keyed by post time then validator operator
	// address.
	ValuePostScheduleKeyPrefix = []byte{0x0B}

	// CachedPoolValueKeyPrefix is the store prefix for the last successfully
	// observed position value per pool, keyed by pool id. Used as the
	// fallback when a pool contract cannot be queried during a value post,
	// so a broken pool degrades a score gracefully instead of halting the
	// end blocker or zeroing the vault.
	CachedPoolValueKeyPrefix = []byte{0x0C}
)

// VaultKey returns the store key for a validator's vault.
func VaultKey(valAddr sdk.ValAddress) []byte {
	return append(VaultKeyPrefix, valAddr.Bytes()...)
}

// PendingWithdrawalTimeKey returns the queue prefix covering every pending
// withdrawal that completes at time t (regardless of validator).
func PendingWithdrawalTimeKey(t time.Time) []byte {
	return append(PendingWithdrawalKeyPrefix, sdk.FormatTimeBytes(t)...)
}

// PendingWithdrawalKey returns the store key for a validator's pending
// withdrawal completing at time t.
func PendingWithdrawalKey(t time.Time, valAddr sdk.ValAddress) []byte {
	return append(PendingWithdrawalTimeKey(t), valAddr.Bytes()...)
}

// TokensSnapshotKey returns the store key for a validator's pre-operation
// token snapshot used by the stake cap hooks.
func TokensSnapshotKey(valAddr sdk.ValAddress) []byte {
	return append(TokensSnapshotKeyPrefix, valAddr.Bytes()...)
}

// PoolKey returns the store key for a registered pool.
func PoolKey(poolID uint64) []byte {
	return binary.BigEndian.AppendUint64(PoolKeyPrefix, poolID)
}

// PoolTotalSharesKey returns the store key for a pool's total internal
// shares.
func PoolTotalSharesKey(poolID uint64) []byte {
	return binary.BigEndian.AppendUint64(PoolTotalSharesKeyPrefix, poolID)
}

// positionSuffix is the length-prefixed validator address followed by the
// pool id, shared by the position and pending-shares keys.
func positionSuffix(valAddr sdk.ValAddress, poolID uint64) []byte {
	suffix := address.MustLengthPrefix(valAddr.Bytes())
	return binary.BigEndian.AppendUint64(suffix, poolID)
}

// PositionKey returns the store key for a validator's position in a pool.
func PositionKey(valAddr sdk.ValAddress, poolID uint64) []byte {
	return append(PositionKeyPrefix, positionSuffix(valAddr, poolID)...)
}

// PositionKeyPrefixForValidator returns the prefix covering all of a
// validator's positions.
func PositionKeyPrefixForValidator(valAddr sdk.ValAddress) []byte {
	return append(PositionKeyPrefix, address.MustLengthPrefix(valAddr.Bytes())...)
}

// PendingSharesKey returns the store key for a validator's queued
// deallocation shares in a pool.
func PendingSharesKey(valAddr sdk.ValAddress, poolID uint64) []byte {
	return append(PendingSharesKeyPrefix, positionSuffix(valAddr, poolID)...)
}

// PendingDeallocationTimeKey returns the queue prefix covering every pending
// deallocation that completes at time t.
func PendingDeallocationTimeKey(t time.Time) []byte {
	return append(PendingDeallocationKeyPrefix, sdk.FormatTimeBytes(t)...)
}

// PendingDeallocationKey returns the store key for a validator's pending
// deallocation from a pool completing at time t.
func PendingDeallocationKey(t time.Time, valAddr sdk.ValAddress, poolID uint64) []byte {
	return append(PendingDeallocationTimeKey(t), positionSuffix(valAddr, poolID)...)
}

// ValuePostHistoryKey returns the store key for a validator's value-post
// window.
func ValuePostHistoryKey(valAddr sdk.ValAddress) []byte {
	return append(ValuePostHistoryKeyPrefix, valAddr.Bytes()...)
}

// ValuePostScheduleTimeKey returns the schedule prefix covering every post
// due at time t.
func ValuePostScheduleTimeKey(t time.Time) []byte {
	return append(ValuePostScheduleKeyPrefix, sdk.FormatTimeBytes(t)...)
}

// ValuePostScheduleKey returns the store key for a validator's scheduled
// value post at time t.
func ValuePostScheduleKey(t time.Time, valAddr sdk.ValAddress) []byte {
	return append(ValuePostScheduleTimeKey(t), valAddr.Bytes()...)
}

// CachedPoolValueKey returns the store key for a pool's last observed
// position value.
func CachedPoolValueKey(poolID uint64) []byte {
	return binary.BigEndian.AppendUint64(CachedPoolValueKeyPrefix, poolID)
}
