package keeper

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"time"

	"cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"bluechipChain/x/liquidityvault/types"
)

// maxValuePostsPerBlock caps how many due value posts one end blocker
// takes, so a backlog is worked off over several blocks.
const maxValuePostsPerBlock = 10

// GetValuePostHistory returns a validator's rolling value-post window.
func (k Keeper) GetValuePostHistory(ctx context.Context, valAddr sdk.ValAddress) types.ValuePostHistory {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	bz := store.Get(types.ValuePostHistoryKey(valAddr))
	if bz == nil {
		return types.ValuePostHistory{ValidatorAddress: valAddr.String()}
	}

	var history types.ValuePostHistory
	k.cdc.MustUnmarshal(bz, &history)
	return history
}

// SetValuePostHistory stores a validator's value-post window.
func (k Keeper) SetValuePostHistory(ctx context.Context, history types.ValuePostHistory) error {
	valAddr, err := sdk.ValAddressFromBech32(history.ValidatorAddress)
	if err != nil {
		return err
	}

	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	bz, err := k.cdc.Marshal(&history)
	if err != nil {
		return err
	}
	store.Set(types.ValuePostHistoryKey(valAddr), bz)
	return nil
}

// GetAllValuePostHistories returns every validator's value-post window.
func (k Keeper) GetAllValuePostHistories(ctx context.Context) []types.ValuePostHistory {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	iterator := storetypes.KVStorePrefixIterator(store, types.ValuePostHistoryKeyPrefix)
	defer iterator.Close()

	var histories []types.ValuePostHistory
	for ; iterator.Valid(); iterator.Next() {
		var history types.ValuePostHistory
		k.cdc.MustUnmarshal(iterator.Value(), &history)
		histories = append(histories, history)
	}
	return histories
}

// MedianOfPosts returns the median of the given posts' values, averaging
// the two middle values (truncated) for an even count, and zero for none.
func MedianOfPosts(posts []types.ValuePost) math.Int {
	if len(posts) == 0 {
		return math.ZeroInt()
	}
	values := make([]math.Int, len(posts))
	for i, post := range posts {
		values[i] = post.Value
	}
	sort.Slice(values, func(i, j int) bool { return values[i].LT(values[j]) })

	mid := len(values) / 2
	if len(values)%2 == 1 {
		return values[mid]
	}
	return values[mid-1].Add(values[mid]).QuoRaw(2)
}

// SetScheduledValuePost inserts a schedule entry.
func (k Keeper) SetScheduledValuePost(ctx context.Context, scheduled types.ScheduledValuePost) error {
	valAddr, err := sdk.ValAddressFromBech32(scheduled.ValidatorAddress)
	if err != nil {
		return err
	}

	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	bz, err := k.cdc.Marshal(&scheduled)
	if err != nil {
		return err
	}
	store.Set(types.ValuePostScheduleKey(scheduled.PostTime, valAddr), bz)
	return nil
}

// GetAllScheduledValuePosts returns the whole schedule in time order.
func (k Keeper) GetAllScheduledValuePosts(ctx context.Context) []types.ScheduledValuePost {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	iterator := storetypes.KVStorePrefixIterator(store, types.ValuePostScheduleKeyPrefix)
	defer iterator.Close()

	var scheduled []types.ScheduledValuePost
	for ; iterator.Valid(); iterator.Next() {
		var entry types.ScheduledValuePost
		k.cdc.MustUnmarshal(iterator.Value(), &entry)
		scheduled = append(scheduled, entry)
	}
	return scheduled
}

// NextScheduledValuePost returns a validator's next scheduled post, if any.
// It scans the time-ordered schedule and stops at the first match.
func (k Keeper) NextScheduledValuePost(ctx context.Context, valAddr sdk.ValAddress) (types.ScheduledValuePost, bool) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	iterator := storetypes.KVStorePrefixIterator(store, types.ValuePostScheduleKeyPrefix)
	defer iterator.Close()

	valStr := valAddr.String()
	for ; iterator.Valid(); iterator.Next() {
		var entry types.ScheduledValuePost
		k.cdc.MustUnmarshal(iterator.Value(), &entry)
		if entry.ValidatorAddress == valStr {
			return entry, true
		}
	}
	return types.ScheduledValuePost{}, false
}

// scheduleNextValuePost schedules a validator's next value post at a
// pseudo-random time in [interval/2, interval*3/2) from now. The jitter is
// derived deterministically from the block header hash and the validator
// address, so all nodes agree while validators cannot predict post times
// far in advance.
func (k Keeper) scheduleNextValuePost(ctx sdk.Context, valAddr sdk.ValAddress) error {
	interval := k.GetParams(ctx).ValuePostInterval
	if interval <= 0 {
		interval = types.DefaultValuePostInterval
	}

	seed := sha256.Sum256(append(append([]byte{}, ctx.HeaderHash()...), valAddr.Bytes()...))
	frac := binary.BigEndian.Uint64(seed[:8]) % 1_000_000 // parts per million

	// interval/2 + frac/1e6 * interval, computed without overflow.
	jitter := time.Duration(interval.Nanoseconds() / 1_000_000 * int64(frac))
	postTime := ctx.BlockTime().Add(interval/2 + jitter)

	return k.SetScheduledValuePost(ctx, types.ScheduledValuePost{
		ValidatorAddress: valAddr.String(),
		PostTime:         postTime,
	})
}

// GetCachedPoolValue returns the last successfully observed position value
// for a pool.
func (k Keeper) GetCachedPoolValue(ctx context.Context, poolID uint64) (math.Int, bool) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	bz := store.Get(types.CachedPoolValueKey(poolID))
	if bz == nil {
		return math.ZeroInt(), false
	}

	var value math.Int
	if err := value.Unmarshal(bz); err != nil {
		panic(fmt.Sprintf("corrupt cached pool value for pool %d: %v", poolID, err))
	}
	return value, true
}

// GetAllCachedPoolValues returns every pool's cached value in pool-id
// order, for genesis export.
func (k Keeper) GetAllCachedPoolValues(ctx context.Context) []types.CachedPoolValue {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	iterator := storetypes.KVStorePrefixIterator(store, types.CachedPoolValueKeyPrefix)
	defer iterator.Close()

	var cached []types.CachedPoolValue
	for ; iterator.Valid(); iterator.Next() {
		poolID := binary.BigEndian.Uint64(iterator.Key()[len(types.CachedPoolValueKeyPrefix):])
		var value math.Int
		if err := value.Unmarshal(iterator.Value()); err != nil {
			panic(fmt.Sprintf("corrupt cached pool value for pool %d: %v", poolID, err))
		}
		cached = append(cached, types.CachedPoolValue{PoolId: poolID, Value: value})
	}
	return cached
}

// ImportCachedPoolValues writes genesis cached pool values.
func (k Keeper) ImportCachedPoolValues(ctx context.Context, cached []types.CachedPoolValue) error {
	for _, entry := range cached {
		if err := k.setCachedPoolValue(ctx, entry.PoolId, entry.Value); err != nil {
			return err
		}
	}
	return nil
}

// setCachedPoolValue stores a pool's last observed position value.
func (k Keeper) setCachedPoolValue(ctx context.Context, poolID uint64, value math.Int) error {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	bz, err := value.Marshal()
	if err != nil {
		return err
	}
	store.Set(types.CachedPoolValueKey(poolID), bz)
	return nil
}

// guardedPoolValue queries a pool's position value under a bounded gas
// meter and a panic guard, for use in the end blocker and other
// non-tx paths where a misbehaving contract must be able neither to abort
// the block nor to stall it: wasmd's raw QuerySmart grants the contract gas
// from the calling context's meter, which is infinite in an end blocker.
func (k Keeper) guardedPoolValue(ctx sdk.Context, pool types.RegisteredPool) (value math.Int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = types.ErrPoolValueUnavailable.Wrapf("pool %d value query aborted: %v", pool.PoolId, r)
		}
	}()
	gasCtx := ctx.WithGasMeter(storetypes.NewGasMeter(contractCallGasLimit))
	return k.PoolPositionValue(gasCtx, pool)
}

// positionsValueWithFallback values a validator's pool positions, falling
// back per pool to the last cached value — and to zero with a log line when
// no cache exists — so a broken pool degrades the valuation gracefully
// rather than erroring or aborting the block. refresh controls whether
// successful observations update the cache (state-machine paths only; gRPC
// query handlers run on ephemeral stores where writes are meaningless).
func (k Keeper) positionsValueWithFallback(ctx sdk.Context, valAddr sdk.ValAddress, refresh bool) (math.Int, error) {
	total := math.ZeroInt()
	for _, position := range k.GetValidatorPositions(ctx, valAddr) {
		pool, found := k.GetPool(ctx, position.PoolId)
		if !found {
			return math.Int{}, types.ErrPoolNotFound.Wrapf("position references pool %d", position.PoolId)
		}

		poolValue, err := k.guardedPoolValue(ctx, pool)
		if err == nil {
			if refresh {
				if err := k.setCachedPoolValue(ctx, position.PoolId, poolValue); err != nil {
					return math.Int{}, err
				}
			}
		} else {
			cached, found := k.GetCachedPoolValue(ctx, position.PoolId)
			if !found {
				k.Logger().Error("pool unreachable with no cached value; valuing its positions at zero",
					"pool", position.PoolId, "validator", position.ValidatorAddress, "error", err)
				continue
			}
			k.Logger().Info("pool unreachable; using cached value",
				"pool", position.PoolId, "validator", position.ValidatorAddress)
			poolValue = cached
		}

		total = total.Add(positionValueFromShares(position.Shares, k.GetPoolTotalShares(ctx, position.PoolId), poolValue))
	}
	return total, nil
}

// vaultTotalValueForPost values a vault (active balance plus positions) for
// a value post, refreshing the pool value cache on success.
func (k Keeper) vaultTotalValueForPost(ctx sdk.Context, valAddr sdk.ValAddress) (math.Int, error) {
	total := math.ZeroInt()
	if vault, found := k.GetVault(ctx, valAddr); found {
		total = total.Add(vault.Balance)
	}

	positions, err := k.positionsValueWithFallback(ctx, valAddr, true)
	if err != nil {
		return math.Int{}, err
	}
	return total.Add(positions), nil
}

// ProcessDueValuePosts takes the due vault value snapshots, appends them to
// the rolling windows, and schedules each vault's next post. Called from
// the module's EndBlocker; capped per block.
func (k Keeper) ProcessDueValuePosts(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))

	end := storetypes.PrefixEndBytes(types.ValuePostScheduleTimeKey(sdkCtx.BlockTime()))
	iterator := store.Iterator(types.ValuePostScheduleKeyPrefix, end)

	type due struct {
		key   []byte
		entry types.ScheduledValuePost
	}
	var duePosts []due
	for ; iterator.Valid() && len(duePosts) < maxValuePostsPerBlock; iterator.Next() {
		var entry types.ScheduledValuePost
		k.cdc.MustUnmarshal(iterator.Value(), &entry)
		key := make([]byte, len(iterator.Key()))
		copy(key, iterator.Key())
		duePosts = append(duePosts, due{key: key, entry: entry})
	}
	if err := iterator.Close(); err != nil {
		return err
	}

	for _, d := range duePosts {
		store.Delete(d.key)

		valAddr, err := sdk.ValAddressFromBech32(d.entry.ValidatorAddress)
		if err != nil {
			return err
		}

		value, err := k.vaultTotalValueForPost(sdkCtx, valAddr)
		if err != nil {
			return err
		}

		history := k.GetValuePostHistory(sdkCtx, valAddr)
		history.Posts = append(history.Posts, types.ValuePost{
			Value:    value,
			PostTime: sdkCtx.BlockTime(),
		})
		if len(history.Posts) > types.ValuePostWindow {
			history.Posts = history.Posts[len(history.Posts)-types.ValuePostWindow:]
		}
		if err := k.SetValuePostHistory(sdkCtx, history); err != nil {
			return err
		}

		if err := k.scheduleNextValuePost(sdkCtx, valAddr); err != nil {
			return err
		}

		sdkCtx.EventManager().EmitEvent(
			sdk.NewEvent(
				types.EventTypeValuePost,
				sdk.NewAttribute(types.AttributeKeyValidator, d.entry.ValidatorAddress),
				sdk.NewAttribute(types.AttributeKeyAmount, value.String()),
			),
		)
	}

	return nil
}
