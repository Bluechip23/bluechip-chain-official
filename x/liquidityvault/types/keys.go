package types

import (
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
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
