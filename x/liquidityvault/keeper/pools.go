package keeper

import (
	"context"
	"encoding/binary"

	errorsmod "cosmossdk.io/errors"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"bluechipChain/x/liquidityvault/types"
)

// GetPool returns a registered pool, if it exists.
func (k Keeper) GetPool(ctx context.Context, poolID uint64) (types.RegisteredPool, bool) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	bz := store.Get(types.PoolKey(poolID))
	if bz == nil {
		return types.RegisteredPool{}, false
	}

	var pool types.RegisteredPool
	k.cdc.MustUnmarshal(bz, &pool)
	return pool, true
}

// SetPool stores a registered pool.
func (k Keeper) SetPool(ctx context.Context, pool types.RegisteredPool) error {
	if err := pool.Validate(); err != nil {
		return err
	}

	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	bz, err := k.cdc.Marshal(&pool)
	if err != nil {
		return err
	}
	store.Set(types.PoolKey(pool.PoolId), bz)
	return nil
}

// IteratePools iterates over all registered pools in id order, stopping when
// cb returns true.
func (k Keeper) IteratePools(ctx context.Context, cb func(pool types.RegisteredPool) (stop bool)) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	iterator := storetypes.KVStorePrefixIterator(store, types.PoolKeyPrefix)
	defer iterator.Close()

	for ; iterator.Valid(); iterator.Next() {
		var pool types.RegisteredPool
		k.cdc.MustUnmarshal(iterator.Value(), &pool)
		if cb(pool) {
			break
		}
	}
}

// GetAllPools returns every registered pool in id order.
func (k Keeper) GetAllPools(ctx context.Context) []types.RegisteredPool {
	var pools []types.RegisteredPool
	k.IteratePools(ctx, func(pool types.RegisteredPool) bool {
		pools = append(pools, pool)
		return false
	})
	return pools
}

// NextPoolID returns and increments the pool id counter. The first pool gets
// id 1.
func (k Keeper) NextPoolID(ctx context.Context) uint64 {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	id := uint64(1)
	if bz := store.Get(types.NextPoolIDKey); bz != nil {
		id = binary.BigEndian.Uint64(bz)
	}
	store.Set(types.NextPoolIDKey, binary.BigEndian.AppendUint64(nil, id+1))
	return id
}

// SetNextPoolID sets the pool id counter (used by genesis import).
func (k Keeper) SetNextPoolID(ctx context.Context, id uint64) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	store.Set(types.NextPoolIDKey, binary.BigEndian.AppendUint64(nil, id))
}

// GetNextPoolID reads the pool id counter without incrementing it.
func (k Keeper) GetNextPoolID(ctx context.Context) uint64 {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	if bz := store.Get(types.NextPoolIDKey); bz != nil {
		return binary.BigEndian.Uint64(bz)
	}
	return 1
}

// RegisterPool registers a new pool contract and returns its id. The
// contract must exist on chain, implement the Vault Adapter Interface, and
// not already be registered.
func (k Keeper) RegisterPool(ctx context.Context, contractAddress, description string) (uint64, error) {
	contractAddr, err := sdk.AccAddressFromBech32(contractAddress)
	if err != nil {
		return 0, errorsmod.Wrap(err, "invalid pool contract address")
	}

	wasmKeeper, err := k.wasmKeeper()
	if err != nil {
		return 0, err
	}
	if !wasmKeeper.HasContractInfo(ctx, contractAddr) {
		return 0, errorsmod.Wrapf(types.ErrPoolNotFound, "no contract at %s", contractAddress)
	}

	duplicate := false
	k.IteratePools(ctx, func(pool types.RegisteredPool) bool {
		if pool.ContractAddress == contractAddress {
			duplicate = true
			return true
		}
		return false
	})
	if duplicate {
		return 0, errorsmod.Wrap(types.ErrPoolAlreadyRegistered, contractAddress)
	}

	pool := types.RegisteredPool{
		PoolId:          k.NextPoolID(ctx),
		ContractAddress: contractAddress,
		Description:     description,
		Enabled:         true,
	}

	// Probe the Vault Adapter Interface before accepting the pool: a
	// contract that cannot even answer position_value would let allocations
	// in but strand every later valuation and withdrawal.
	if _, err := k.PoolPositionValue(ctx, pool); err != nil {
		return 0, errorsmod.Wrapf(types.ErrPoolValueUnavailable,
			"contract %s does not implement the Vault Adapter Interface: %v", contractAddress, err)
	}

	if err := k.SetPool(ctx, pool); err != nil {
		return 0, err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			types.EventTypePoolRegistered,
			sdk.NewAttribute(types.AttributeKeyPoolID, fmtPoolID(pool.PoolId)),
			sdk.NewAttribute(types.AttributeKeyContract, contractAddress),
		),
	)

	return pool.PoolId, nil
}

// SetPoolEnabled toggles whether a pool accepts new allocations.
func (k Keeper) SetPoolEnabled(ctx context.Context, poolID uint64, enabled bool) error {
	pool, found := k.GetPool(ctx, poolID)
	if !found {
		return errorsmod.Wrapf(types.ErrPoolNotFound, "pool %d", poolID)
	}

	pool.Enabled = enabled
	if err := k.SetPool(ctx, pool); err != nil {
		return err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			types.EventTypePoolEnabledSet,
			sdk.NewAttribute(types.AttributeKeyPoolID, fmtPoolID(poolID)),
			sdk.NewAttribute(types.AttributeKeyEnabled, fmtBool(enabled)),
		),
	)

	return nil
}
