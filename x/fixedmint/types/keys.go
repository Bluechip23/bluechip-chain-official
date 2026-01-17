package types

const (
	// ModuleName defines the module name
	ModuleName = "fixedmint"

	// StoreKey defines the primary module store key
	StoreKey = ModuleName

	// MemStoreKey defines the in-memory store key
	MemStoreKey = "mem_fixedmint"

    
)

var (
	ParamsKey = []byte("p_fixedmint")
)



func KeyPrefix(p string) []byte {
    return []byte(p)
}
