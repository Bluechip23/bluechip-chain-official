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
