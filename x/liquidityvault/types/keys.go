package types

const (
	// ModuleName defines the module name
	ModuleName = "liquidityvault"

	// StoreKey defines the primary module store key
	StoreKey = ModuleName

	// MemStoreKey defines the in-memory store key
	MemStoreKey = "mem_liquidityvault"
)

var (
	ParamsKey = []byte("p_liquidityvault")

	// VaultKeyPrefix is the prefix for vault storage
	VaultKeyPrefix = []byte{0x01}

	// CompositeScoreKeyPrefix is the prefix for composite score storage
	CompositeScoreKeyPrefix = []byte{0x02}

	// ValuePostKeyPrefix is the prefix for value post storage
	ValuePostKeyPrefix = []byte{0x03}

	// ValidatorTypeKeyPrefix is the prefix for validator type storage
	ValidatorTypeKeyPrefix = []byte{0x04}

	// LastSimpleCheckHeightKey stores the last simple check block height
	LastSimpleCheckHeightKey = []byte{0x05}

	// LastComplexCheckHeightKey stores the last complex check block height
	LastComplexCheckHeightKey = []byte{0x06}

	// ScheduledValuePostKeyPrefix is the prefix for scheduled value post blocks
	ScheduledValuePostKeyPrefix = []byte{0x07}
)

func KeyPrefix(p string) []byte {
	return []byte(p)
}

// VaultKey returns the store key for a validator's vault
func VaultKey(valAddr string) []byte {
	return append(VaultKeyPrefix, []byte(valAddr)...)
}

// CompositeScoreKey returns the store key for a validator's composite score
func CompositeScoreKey(valAddr string) []byte {
	return append(CompositeScoreKeyPrefix, []byte(valAddr)...)
}

// ValuePostKey returns the store key for a specific value post
func ValuePostKey(valAddr string, blockHeight int64) []byte {
	key := append(ValuePostKeyPrefix, []byte(valAddr)...)
	key = append(key, '/')
	// Append block height as big-endian bytes
	bz := make([]byte, 8)
	bz[0] = byte(blockHeight >> 56)
	bz[1] = byte(blockHeight >> 48)
	bz[2] = byte(blockHeight >> 40)
	bz[3] = byte(blockHeight >> 32)
	bz[4] = byte(blockHeight >> 24)
	bz[5] = byte(blockHeight >> 16)
	bz[6] = byte(blockHeight >> 8)
	bz[7] = byte(blockHeight)
	return append(key, bz...)
}

// ValuePostPrefixKey returns the prefix key for all value posts of a validator
func ValuePostPrefixKey(valAddr string) []byte {
	key := append(ValuePostKeyPrefix, []byte(valAddr)...)
	return append(key, '/')
}

// ValidatorTypeKey returns the store key for a validator's type
func ValidatorTypeKey(valAddr string) []byte {
	return append(ValidatorTypeKeyPrefix, []byte(valAddr)...)
}

// ScheduledValuePostKey returns the store key for a scheduled value post block
func ScheduledValuePostKey(blockHeight int64) []byte {
	bz := make([]byte, 8)
	bz[0] = byte(blockHeight >> 56)
	bz[1] = byte(blockHeight >> 48)
	bz[2] = byte(blockHeight >> 40)
	bz[3] = byte(blockHeight >> 32)
	bz[4] = byte(blockHeight >> 24)
	bz[5] = byte(blockHeight >> 16)
	bz[6] = byte(blockHeight >> 8)
	bz[7] = byte(blockHeight)
	return append(ScheduledValuePostKeyPrefix, bz...)
}
