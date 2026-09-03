package keeper

import (
	"fmt"

	"cosmossdk.io/core/store"
	"cosmossdk.io/log"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"bluechipChain/x/liquidityvault/types"
)

type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	logger       log.Logger

	// the address capable of executing a MsgUpdateParams message. Typically, this
	// should be the x/gov module account.
	authority string

	bankKeeper    types.BankKeeper
	stakingKeeper types.StakingKeeper

	// wasmRef holds the pool-contract bridge. It is a pointer so that
	// SetWasmKeeper, called from app.go after the wasm module is wired
	// (wasmd is not depinject-enabled), reaches every copy of the Keeper —
	// including the one already embedded in the AppModule.
	wasmRef *wasmRef
}

type wasmRef struct {
	keeper types.WasmKeeper
}

// SetWasmKeeper wires the pool-contract bridge. Called once from app.go
// after the wasm keeper exists.
func (k Keeper) SetWasmKeeper(wk types.WasmKeeper) {
	k.wasmRef.keeper = wk
}

// wasmKeeper returns the pool-contract bridge, or an error if the app has
// not wired it.
func (k Keeper) wasmKeeper() (types.WasmKeeper, error) {
	if k.wasmRef == nil || k.wasmRef.keeper == nil {
		return nil, types.ErrPoolBridgeUnavailable
	}
	return k.wasmRef.keeper, nil
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	logger log.Logger,
	authority string,
	bankKeeper types.BankKeeper,
	stakingKeeper types.StakingKeeper,
) Keeper {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		panic(fmt.Sprintf("invalid authority address: %s", authority))
	}

	return Keeper{
		cdc:           cdc,
		storeService:  storeService,
		logger:        logger,
		authority:     authority,
		bankKeeper:    bankKeeper,
		stakingKeeper: stakingKeeper,
		wasmRef:       &wasmRef{},
	}
}

// GetAuthority returns the module's authority.
func (k Keeper) GetAuthority() string {
	return k.authority
}

// Logger returns a module-specific logger.
func (k Keeper) Logger() log.Logger {
	return k.logger.With("module", fmt.Sprintf("x/%s", types.ModuleName))
}
