package keeper

import (
	"context"
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

	// the address capable of executing a MsgUpdateParams message.
	// Typically, this should be the x/gov module account.
	authority string

	bankKeeper    types.BankKeeper
	stakingKeeper types.StakingKeeper
	accountKeeper types.AccountKeeper

	// wasmKeeper is set after initialization since it's not available via depinject
	wasmKeeper types.WasmKeeper
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	logger log.Logger,
	authority string,
	bankKeeper types.BankKeeper,
	stakingKeeper types.StakingKeeper,
	accountKeeper types.AccountKeeper,
) Keeper {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		panic(fmt.Sprintf("invalid authority address: %s", authority))
	}

	return Keeper{
		cdc:           cdc,
		storeService:  storeService,
		authority:     authority,
		logger:        logger,
		bankKeeper:    bankKeeper,
		stakingKeeper: stakingKeeper,
		accountKeeper: accountKeeper,
	}
}

// SetWasmKeeper sets the wasm keeper after module initialization.
// This must be called after the WasmKeeper is created (it's not available via depinject).
func (k *Keeper) SetWasmKeeper(wasmKeeper types.WasmKeeper) {
	k.wasmKeeper = wasmKeeper
}

// GetAuthority returns the module's authority.
func (k Keeper) GetAuthority() string {
	return k.authority
}

// Logger returns a module-specific logger.
func (k Keeper) Logger() log.Logger {
	return k.logger.With("module", fmt.Sprintf("x/%s", types.ModuleName))
}

// GetModuleAddress returns the module account address.
func (k Keeper) GetModuleAddress() sdk.AccAddress {
	return k.accountKeeper.GetModuleAddress(types.ModuleName)
}

// BeginBlock is called at the beginning of each block.
func (k Keeper) BeginBlock(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	blockHeight := sdkCtx.BlockHeight()
	params := k.GetParams(ctx)

	// 1. Check if this is a scheduled value post block
	if k.IsValuePostBlock(ctx, blockHeight) {
		if err := k.ExecuteValuePost(ctx); err != nil {
			k.Logger().Error("failed to execute value post", "error", err)
		}
	}

	// 2. Simple check every SimpleCheckInterval blocks
	lastSimple := k.GetLastSimpleCheckHeight(ctx)
	if blockHeight-lastSimple >= int64(params.SimpleCheckInterval) {
		if err := k.ExecuteSimpleCheck(ctx); err != nil {
			k.Logger().Error("failed to execute simple check", "error", err)
		}
		k.SetLastSimpleCheckHeight(ctx, blockHeight)
	}

	// 3. Complex check every ComplexCheckInterval blocks
	lastComplex := k.GetLastComplexCheckHeight(ctx)
	if blockHeight-lastComplex >= int64(params.ComplexCheckInterval) {
		if err := k.ExecuteComplexCheck(ctx); err != nil {
			k.Logger().Error("failed to execute complex check", "error", err)
		}
		k.SetLastComplexCheckHeight(ctx, blockHeight)
		// Clear old value posts and schedule new ones
		k.ClearAllValuePosts(ctx)
		k.ScheduleValuePosts(ctx, blockHeight, blockHeight+int64(params.ComplexCheckInterval))
	}

	return nil
}
