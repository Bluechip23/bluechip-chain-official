package keeper

import (
	"context"
	"time"

	"cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"bluechipChain/x/liquidityvault/types"
)

// deallocationRetryDelay is how long a matured deallocation waits before
// retrying after its pool contract call fails (e.g. the pool is paused).
const deallocationRetryDelay = time.Hour

// contractCallGasLimit bounds the gas one pool contract call may burn inside
// the end blocker, so a misbehaving contract cannot stall block production.
const contractCallGasLimit = 5_000_000

// maxDeallocationRetries is how many failed settlement attempts a
// deallocation gets before it is abandoned: the entry is dropped and the
// shares simply stay in the validator's position (nothing is lost), so a
// permanently broken pool cannot wedge the queue in an eternal retry loop.
const maxDeallocationRetries = 24

// maxSettlementsPerBlock caps how many matured deallocations one end
// blocker settles, so a backlog (e.g. after downtime) is worked off over
// several blocks instead of stalling one block with unbounded contract
// executions.
const maxSettlementsPerBlock = 20

// setPendingDeallocation inserts a pending deallocation into the
// time-ordered queue, merging with an entry for the same validator, pool,
// and completion time.
func (k Keeper) setPendingDeallocation(ctx context.Context, deallocation types.PendingDeallocation) error {
	valAddr, err := sdk.ValAddressFromBech32(deallocation.ValidatorAddress)
	if err != nil {
		return err
	}

	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	key := types.PendingDeallocationKey(deallocation.CompleteTime, valAddr, deallocation.PoolId)

	if bz := store.Get(key); bz != nil {
		var existing types.PendingDeallocation
		k.cdc.MustUnmarshal(bz, &existing)
		deallocation.Shares = deallocation.Shares.Add(existing.Shares)
	}

	bz, err := k.cdc.Marshal(&deallocation)
	if err != nil {
		return err
	}
	store.Set(key, bz)
	return nil
}

// IteratePendingDeallocations iterates over the whole deallocation queue in
// completion-time order, stopping when cb returns true.
func (k Keeper) IteratePendingDeallocations(ctx context.Context, cb func(deallocation types.PendingDeallocation) (stop bool)) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	iterator := storetypes.KVStorePrefixIterator(store, types.PendingDeallocationKeyPrefix)
	defer iterator.Close()

	for ; iterator.Valid(); iterator.Next() {
		var deallocation types.PendingDeallocation
		k.cdc.MustUnmarshal(iterator.Value(), &deallocation)
		if cb(deallocation) {
			break
		}
	}
}

// GetAllPendingDeallocations returns the whole deallocation queue in
// completion-time order.
func (k Keeper) GetAllPendingDeallocations(ctx context.Context) []types.PendingDeallocation {
	var deallocations []types.PendingDeallocation
	k.IteratePendingDeallocations(ctx, func(deallocation types.PendingDeallocation) bool {
		deallocations = append(deallocations, deallocation)
		return false
	})
	return deallocations
}

// GetPendingDeallocations returns a validator's pending deallocations in
// completion-time order.
func (k Keeper) GetPendingDeallocations(ctx context.Context, valAddr sdk.ValAddress) []types.PendingDeallocation {
	valStr := valAddr.String()
	var deallocations []types.PendingDeallocation
	k.IteratePendingDeallocations(ctx, func(deallocation types.PendingDeallocation) bool {
		if deallocation.ValidatorAddress == valStr {
			deallocations = append(deallocations, deallocation)
		}
		return false
	})
	return deallocations
}

// ProcessMaturedDeallocations executes every pending deallocation whose
// grace period has ended: it withdraws the share fraction from the pool
// contract and forwards the proceeds to the validator's own account (the
// funds leave the vault, per the LPV design document). A failing contract
// call never aborts the block; the entry is requeued with a delay instead.
// Called from the module's EndBlocker.
func (k Keeper) ProcessMaturedDeallocations(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))

	bondDenom, err := k.stakingKeeper.BondDenom(ctx)
	if err != nil {
		return err
	}

	// Collect matured entries first: the store must not be mutated while an
	// iterator over it is open. Settlement is capped per block; entries
	// beyond the cap stay matured in the queue and are picked up next
	// block.
	end := storetypes.PrefixEndBytes(types.PendingDeallocationTimeKey(sdkCtx.BlockTime()))
	iterator := store.Iterator(types.PendingDeallocationKeyPrefix, end)

	type matured struct {
		key          []byte
		deallocation types.PendingDeallocation
	}
	var maturedDeallocations []matured
	for ; iterator.Valid() && len(maturedDeallocations) < maxSettlementsPerBlock; iterator.Next() {
		var deallocation types.PendingDeallocation
		k.cdc.MustUnmarshal(iterator.Value(), &deallocation)
		key := make([]byte, len(iterator.Key()))
		copy(key, iterator.Key())
		maturedDeallocations = append(maturedDeallocations, matured{key: key, deallocation: deallocation})
	}
	if err := iterator.Close(); err != nil {
		return err
	}

	for _, m := range maturedDeallocations {
		store.Delete(m.key)
		if err := k.settleDeallocation(sdkCtx, m.deallocation, bondDenom); err != nil {
			if err := k.handleFailedDeallocation(sdkCtx, m.deallocation, err); err != nil {
				return err
			}
		}
	}

	return nil
}

// handleFailedDeallocation requeues a deallocation whose pool contract call
// failed, or abandons it after too many attempts (releasing the reservation
// so the shares stay usable in the position — nothing is lost).
func (k Keeper) handleFailedDeallocation(ctx sdk.Context, deallocation types.PendingDeallocation, cause error) error {
	if deallocation.Retries+1 >= maxDeallocationRetries {
		k.Logger().Error("deallocation abandoned after repeated failures; shares remain in the position",
			"validator", deallocation.ValidatorAddress,
			"pool", deallocation.PoolId,
			"error", cause,
		)
		valAddr, err := sdk.ValAddressFromBech32(deallocation.ValidatorAddress)
		if err != nil {
			return err
		}
		if err := k.releasePendingShares(ctx, valAddr, deallocation.PoolId, deallocation.Shares); err != nil {
			return err
		}
		ctx.EventManager().EmitEvent(
			sdk.NewEvent(
				types.EventTypeDeallocationAbandoned,
				sdk.NewAttribute(types.AttributeKeyValidator, deallocation.ValidatorAddress),
				sdk.NewAttribute(types.AttributeKeyPoolID, fmtPoolID(deallocation.PoolId)),
				sdk.NewAttribute(types.AttributeKeyShares, deallocation.Shares.String()),
				sdk.NewAttribute(types.AttributeKeyError, cause.Error()),
			),
		)
		return nil
	}

	k.Logger().Error("deallocation failed; requeueing",
		"validator", deallocation.ValidatorAddress,
		"pool", deallocation.PoolId,
		"attempt", deallocation.Retries+1,
		"error", cause,
	)
	requeued := deallocation
	requeued.Retries++
	requeued.CompleteTime = ctx.BlockTime().Add(deallocationRetryDelay)
	if err := k.setPendingDeallocation(ctx, requeued); err != nil {
		return err
	}
	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			types.EventTypeDeallocationRequeued,
			sdk.NewAttribute(types.AttributeKeyValidator, deallocation.ValidatorAddress),
			sdk.NewAttribute(types.AttributeKeyPoolID, fmtPoolID(deallocation.PoolId)),
			sdk.NewAttribute(types.AttributeKeyCompleteTime, requeued.CompleteTime.String()),
			sdk.NewAttribute(types.AttributeKeyError, cause.Error()),
		),
	)
	return nil
}

// settleDeallocation executes one matured deallocation atomically: the pool
// withdrawal, the transfer to the validator, and the share bookkeeping all
// commit together or not at all. Only the contract call itself is guarded
// against panics (inside executeWithdrawal); a panic in the module's own
// bookkeeping indicates state corruption and is allowed to surface.
func (k Keeper) settleDeallocation(ctx sdk.Context, deallocation types.PendingDeallocation, bondDenom string) error {
	valAddr, err := sdk.ValAddressFromBech32(deallocation.ValidatorAddress)
	if err != nil {
		return err
	}

	cacheCtx, write := ctx.CacheContext()

	position, found := k.GetPosition(cacheCtx, valAddr, deallocation.PoolId)
	if !found {
		// The position vanished (should not happen); drop the entry and
		// release the pending-share reservation.
		return k.releasePendingShares(ctx, valAddr, deallocation.PoolId, deallocation.Shares)
	}

	pool, found := k.GetPool(cacheCtx, deallocation.PoolId)
	if !found {
		return types.ErrPoolNotFound.Wrapf("pool %d", deallocation.PoolId)
	}

	// Clamp defensively; the pending-share reservation normally guarantees
	// shares <= position.Shares.
	shares := math.MinInt(deallocation.Shares, position.Shares)
	totalShares := k.GetPoolTotalShares(cacheCtx, deallocation.PoolId)
	if totalShares.IsZero() || shares.IsZero() {
		return k.releasePendingShares(ctx, valAddr, deallocation.PoolId, deallocation.Shares)
	}

	ratio := math.LegacyNewDecFromInt(shares).QuoInt(totalShares)
	if ratio.GT(math.LegacyOneDec()) {
		ratio = math.LegacyOneDec()
	}
	if ratio.IsZero() {
		// shares/totalShares truncated below 18 decimals: the interface
		// requires a ratio in (0, 1], and burning shares for zero proceeds
		// would lose value. Drop the entry and keep the shares in the
		// position; the validator can deallocate a larger portion.
		k.Logger().Info("deallocation too small to express as a withdrawal ratio; shares remain in the position",
			"validator", deallocation.ValidatorAddress,
			"pool", deallocation.PoolId,
			"shares", deallocation.Shares,
		)
		return k.releasePendingShares(ctx, valAddr, deallocation.PoolId, deallocation.Shares)
	}

	received, err := k.executeWithdrawal(cacheCtx, pool, ratio, bondDenom)
	if err != nil {
		return err
	}

	if received.IsPositive() {
		coins := sdk.NewCoins(sdk.NewCoin(bondDenom, received))
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(cacheCtx, types.ModuleName, sdk.AccAddress(valAddr), coins); err != nil {
			return err
		}
	}

	position.Shares = position.Shares.Sub(shares)
	if err := k.setPosition(cacheCtx, position); err != nil {
		return err
	}
	if err := k.setPoolTotalShares(cacheCtx, deallocation.PoolId, totalShares.Sub(shares)); err != nil {
		return err
	}
	// Refresh the fallback cache with the post-withdrawal value so a pool
	// that goes dark right after doesn't get overvalued by a stale entry.
	if newValue, err := k.guardedPoolValue(cacheCtx, pool); err == nil {
		if err := k.setCachedPoolValue(cacheCtx, deallocation.PoolId, newValue); err != nil {
			return err
		}
	}
	pending := k.GetPendingShares(cacheCtx, valAddr, deallocation.PoolId)
	if err := k.setPendingShares(cacheCtx, valAddr, deallocation.PoolId, math.MaxInt(pending.Sub(deallocation.Shares), math.ZeroInt())); err != nil {
		return err
	}

	write()

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			types.EventTypeDeallocationCompleted,
			sdk.NewAttribute(types.AttributeKeyValidator, deallocation.ValidatorAddress),
			sdk.NewAttribute(types.AttributeKeyPoolID, fmtPoolID(deallocation.PoolId)),
			sdk.NewAttribute(types.AttributeKeyShares, shares.String()),
			sdk.NewAttribute(types.AttributeKeyAmount, received.String()),
		),
	)

	return nil
}

// releasePendingShares drops a queue entry's share reservation without
// touching the position (used when the entry can no longer be executed).
func (k Keeper) releasePendingShares(ctx sdk.Context, valAddr sdk.ValAddress, poolID uint64, shares math.Int) error {
	pending := k.GetPendingShares(ctx, valAddr, poolID)
	return k.setPendingShares(ctx, valAddr, poolID, math.MaxInt(pending.Sub(shares), math.ZeroInt()))
}
