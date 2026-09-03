package keeper

import (
	"context"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"bluechipChain/x/liquidityvault/types"
)

// ImportPositions writes genesis positions and rebuilds each pool's total
// shares from them.
func (k Keeper) ImportPositions(ctx context.Context, positions []types.PoolPosition) error {
	totals := make(map[uint64]math.Int)
	for _, position := range positions {
		if err := k.setPosition(ctx, position); err != nil {
			return err
		}
		total, ok := totals[position.PoolId]
		if !ok {
			total = math.ZeroInt()
		}
		totals[position.PoolId] = total.Add(position.Shares)
	}
	for poolID, total := range totals {
		if err := k.setPoolTotalShares(ctx, poolID, total); err != nil {
			return err
		}
	}
	return nil
}

// ImportValuePostSchedule writes the genesis value-post schedule and starts
// the cadence for any vault the schedule does not cover (e.g. a genesis file
// hand-written without schedule entries).
func (k Keeper) ImportValuePostSchedule(ctx context.Context, vaults []types.Vault, scheduled []types.ScheduledValuePost) error {
	covered := make(map[string]bool, len(scheduled))
	for _, entry := range scheduled {
		if err := k.SetScheduledValuePost(ctx, entry); err != nil {
			return err
		}
		covered[entry.ValidatorAddress] = true
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	for _, vault := range vaults {
		if covered[vault.ValidatorAddress] {
			continue
		}
		valAddr, err := sdk.ValAddressFromBech32(vault.ValidatorAddress)
		if err != nil {
			return err
		}
		if err := k.scheduleNextValuePost(sdkCtx, valAddr); err != nil {
			return err
		}
	}
	return nil
}

// ImportPendingDeallocations writes genesis queue entries and rebuilds the
// per-(validator, pool) pending-share reservations from them.
func (k Keeper) ImportPendingDeallocations(ctx context.Context, deallocations []types.PendingDeallocation) error {
	type pairKey struct {
		validator string
		poolID    uint64
	}
	reservations := make(map[pairKey]math.Int)
	for _, deallocation := range deallocations {
		if err := k.setPendingDeallocation(ctx, deallocation); err != nil {
			return err
		}
		key := pairKey{deallocation.ValidatorAddress, deallocation.PoolId}
		sum, ok := reservations[key]
		if !ok {
			sum = math.ZeroInt()
		}
		reservations[key] = sum.Add(deallocation.Shares)
	}
	for key, sum := range reservations {
		valAddr, err := sdk.ValAddressFromBech32(key.validator)
		if err != nil {
			return err
		}
		if err := k.setPendingShares(ctx, valAddr, key.poolID, sum); err != nil {
			return err
		}
	}
	return nil
}
