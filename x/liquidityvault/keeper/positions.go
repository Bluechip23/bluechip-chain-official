package keeper

import (
	"context"
	"encoding/binary"
	"fmt"
	"strconv"

	"cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"bluechipChain/x/liquidityvault/types"
)

func fmtPoolID(poolID uint64) string { return strconv.FormatUint(poolID, 10) }
func fmtBool(b bool) string          { return strconv.FormatBool(b) }

// GetPosition returns a validator's position in a pool, if it exists.
func (k Keeper) GetPosition(ctx context.Context, valAddr sdk.ValAddress, poolID uint64) (types.PoolPosition, bool) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	bz := store.Get(types.PositionKey(valAddr, poolID))
	if bz == nil {
		return types.PoolPosition{}, false
	}

	var position types.PoolPosition
	k.cdc.MustUnmarshal(bz, &position)
	return position, true
}

// setPosition stores a position, deleting the record when shares reach zero.
func (k Keeper) setPosition(ctx context.Context, position types.PoolPosition) error {
	valAddr, err := sdk.ValAddressFromBech32(position.ValidatorAddress)
	if err != nil {
		return err
	}

	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	key := types.PositionKey(valAddr, position.PoolId)
	if position.Shares.IsZero() {
		store.Delete(key)
		return nil
	}

	bz, err := k.cdc.Marshal(&position)
	if err != nil {
		return err
	}
	store.Set(key, bz)
	return nil
}

// IteratePositions iterates over all positions, stopping when cb returns
// true.
func (k Keeper) IteratePositions(ctx context.Context, cb func(position types.PoolPosition) (stop bool)) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	iterator := storetypes.KVStorePrefixIterator(store, types.PositionKeyPrefix)
	defer iterator.Close()

	for ; iterator.Valid(); iterator.Next() {
		var position types.PoolPosition
		k.cdc.MustUnmarshal(iterator.Value(), &position)
		if cb(position) {
			break
		}
	}
}

// GetValidatorPositions returns a validator's positions in pool-id order.
func (k Keeper) GetValidatorPositions(ctx context.Context, valAddr sdk.ValAddress) []types.PoolPosition {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	iterator := storetypes.KVStorePrefixIterator(store, types.PositionKeyPrefixForValidator(valAddr))
	defer iterator.Close()

	var positions []types.PoolPosition
	for ; iterator.Valid(); iterator.Next() {
		var position types.PoolPosition
		k.cdc.MustUnmarshal(iterator.Value(), &position)
		positions = append(positions, position)
	}
	return positions
}

// GetAllPositions returns every position in the store.
func (k Keeper) GetAllPositions(ctx context.Context) []types.PoolPosition {
	var positions []types.PoolPosition
	k.IteratePositions(ctx, func(position types.PoolPosition) bool {
		positions = append(positions, position)
		return false
	})
	return positions
}

// GetPoolTotalShares returns a pool's total internal shares (zero when no
// positions exist).
func (k Keeper) GetPoolTotalShares(ctx context.Context, poolID uint64) math.Int {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	bz := store.Get(types.PoolTotalSharesKey(poolID))
	if bz == nil {
		return math.ZeroInt()
	}

	var total math.Int
	if err := total.Unmarshal(bz); err != nil {
		panic(fmt.Sprintf("corrupt pool total shares for pool %d: %v", poolID, err))
	}
	return total
}

// setPoolTotalShares stores a pool's total internal shares, deleting the
// record at zero.
func (k Keeper) setPoolTotalShares(ctx context.Context, poolID uint64, total math.Int) error {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	key := types.PoolTotalSharesKey(poolID)
	if total.IsZero() {
		store.Delete(key)
		return nil
	}

	bz, err := total.Marshal()
	if err != nil {
		return err
	}
	store.Set(key, bz)
	return nil
}

// GetPendingShares returns the sum of a validator's shares sitting in the
// deallocation queue for a pool.
func (k Keeper) GetPendingShares(ctx context.Context, valAddr sdk.ValAddress, poolID uint64) math.Int {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	bz := store.Get(types.PendingSharesKey(valAddr, poolID))
	if bz == nil {
		return math.ZeroInt()
	}

	var pending math.Int
	if err := pending.Unmarshal(bz); err != nil {
		panic(fmt.Sprintf("corrupt pending shares for pool %d: %v", poolID, err))
	}
	return pending
}

// IteratePendingShareReservations iterates over every (validator, pool)
// pending-share reservation record, stopping when cb returns true. The key
// layout is length-prefixed validator address followed by a big-endian pool
// id.
func (k Keeper) IteratePendingShareReservations(ctx context.Context, cb func(valAddr sdk.ValAddress, poolID uint64, pending math.Int) (stop bool)) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	iterator := storetypes.KVStorePrefixIterator(store, types.PendingSharesKeyPrefix)
	defer iterator.Close()

	for ; iterator.Valid(); iterator.Next() {
		suffix := iterator.Key()[len(types.PendingSharesKeyPrefix):]
		addrLen := int(suffix[0])
		valAddr := sdk.ValAddress(suffix[1 : 1+addrLen])
		poolID := binary.BigEndian.Uint64(suffix[1+addrLen:])

		var pending math.Int
		if err := pending.Unmarshal(iterator.Value()); err != nil {
			panic(fmt.Sprintf("corrupt pending shares record: %v", err))
		}
		if cb(valAddr, poolID, pending) {
			break
		}
	}
}

// setPendingShares stores the queued-share sum, deleting the record at zero.
func (k Keeper) setPendingShares(ctx context.Context, valAddr sdk.ValAddress, poolID uint64, pending math.Int) error {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	key := types.PendingSharesKey(valAddr, poolID)
	if pending.IsZero() {
		store.Delete(key)
		return nil
	}

	bz, err := pending.Marshal()
	if err != nil {
		return err
	}
	store.Set(key, bz)
	return nil
}
