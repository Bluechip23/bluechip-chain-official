package keeper

import (
	"context"

	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"bluechipChain/x/liquidityvault/types"
)

// SetPendingWithdrawal inserts a pending withdrawal into the time-ordered
// queue. If the validator already has a withdrawal completing at the same
// time (e.g. two withdrawals in one block), the amounts are merged.
func (k Keeper) SetPendingWithdrawal(ctx context.Context, withdrawal types.PendingWithdrawal) error {
	valAddr, err := sdk.ValAddressFromBech32(withdrawal.ValidatorAddress)
	if err != nil {
		return err
	}

	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	key := types.PendingWithdrawalKey(withdrawal.CompleteTime, valAddr)

	if bz := store.Get(key); bz != nil {
		var existing types.PendingWithdrawal
		k.cdc.MustUnmarshal(bz, &existing)
		withdrawal.Amount = withdrawal.Amount.Add(existing.Amount)
	}

	bz, err := k.cdc.Marshal(&withdrawal)
	if err != nil {
		return err
	}
	store.Set(key, bz)

	return nil
}

// IteratePendingWithdrawals iterates over the whole withdrawal queue in
// completion-time order, stopping when cb returns true.
func (k Keeper) IteratePendingWithdrawals(ctx context.Context, cb func(withdrawal types.PendingWithdrawal) (stop bool)) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	iterator := storetypes.KVStorePrefixIterator(store, types.PendingWithdrawalKeyPrefix)
	defer iterator.Close()

	for ; iterator.Valid(); iterator.Next() {
		var withdrawal types.PendingWithdrawal
		k.cdc.MustUnmarshal(iterator.Value(), &withdrawal)
		if cb(withdrawal) {
			break
		}
	}
}

// GetAllPendingWithdrawals returns the whole withdrawal queue in
// completion-time order.
func (k Keeper) GetAllPendingWithdrawals(ctx context.Context) []types.PendingWithdrawal {
	var withdrawals []types.PendingWithdrawal
	k.IteratePendingWithdrawals(ctx, func(withdrawal types.PendingWithdrawal) bool {
		withdrawals = append(withdrawals, withdrawal)
		return false
	})
	return withdrawals
}

// GetPendingWithdrawals returns a validator's pending withdrawals in
// completion-time order.
func (k Keeper) GetPendingWithdrawals(ctx context.Context, valAddr sdk.ValAddress) []types.PendingWithdrawal {
	valStr := valAddr.String()
	var withdrawals []types.PendingWithdrawal
	k.IteratePendingWithdrawals(ctx, func(withdrawal types.PendingWithdrawal) bool {
		if withdrawal.ValidatorAddress == valStr {
			withdrawals = append(withdrawals, withdrawal)
		}
		return false
	})
	return withdrawals
}

// ProcessMaturedWithdrawals releases every pending withdrawal whose grace
// period has ended to its validator's account. Called from the module's
// EndBlocker.
func (k Keeper) ProcessMaturedWithdrawals(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))

	bondDenom, err := k.stakingKeeper.BondDenom(ctx)
	if err != nil {
		return err
	}

	// Covers every queue key whose completion time sorts at or before the
	// current block time, including keys with the validator address suffix.
	// Collect matured entries first: the store must not be mutated while an
	// iterator over it is open.
	end := storetypes.PrefixEndBytes(types.PendingWithdrawalTimeKey(sdkCtx.BlockTime()))
	iterator := store.Iterator(types.PendingWithdrawalKeyPrefix, end)

	type matured struct {
		key        []byte
		withdrawal types.PendingWithdrawal
	}
	var maturedWithdrawals []matured
	for ; iterator.Valid(); iterator.Next() {
		var withdrawal types.PendingWithdrawal
		k.cdc.MustUnmarshal(iterator.Value(), &withdrawal)
		key := make([]byte, len(iterator.Key()))
		copy(key, iterator.Key())
		maturedWithdrawals = append(maturedWithdrawals, matured{key: key, withdrawal: withdrawal})
	}
	if err := iterator.Close(); err != nil {
		return err
	}

	for _, m := range maturedWithdrawals {
		valAddr, err := sdk.ValAddressFromBech32(m.withdrawal.ValidatorAddress)
		if err != nil {
			return err
		}

		coins := sdk.NewCoins(sdk.NewCoin(bondDenom, m.withdrawal.Amount))
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, sdk.AccAddress(valAddr), coins); err != nil {
			return err
		}
		store.Delete(m.key)

		sdkCtx.EventManager().EmitEvent(
			sdk.NewEvent(
				types.EventTypeWithdrawalCompleted,
				sdk.NewAttribute(types.AttributeKeyValidator, m.withdrawal.ValidatorAddress),
				sdk.NewAttribute(types.AttributeKeyAmount, coins.String()),
			),
		)
	}

	return nil
}
