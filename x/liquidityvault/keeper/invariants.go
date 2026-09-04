package keeper

import (
	"fmt"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"bluechipChain/x/liquidityvault/types"
)

// RegisterInvariants registers all module invariants.
func RegisterInvariants(ir sdk.InvariantRegistry, k Keeper) {
	ir.RegisterRoute(types.ModuleName, "module-account", ModuleAccountInvariant(k))
	ir.RegisterRoute(types.ModuleName, "pool-shares", PoolSharesInvariant(k))
}

// ModuleAccountInvariant checks that the liquidityvault module account holds
// at least the sum of all active vault balances, pending withdrawals, and
// outstanding delegator rewards in the bond denom. (Stray sends could push
// the balance above the tracked sum; they are inert, so only a deficit
// breaks the invariant.)
func ModuleAccountInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		tracked := math.ZeroInt()
		k.IterateVaults(ctx, func(vault types.Vault) bool {
			tracked = tracked.Add(vault.Balance)
			if !vault.OutstandingRewards.IsNil() {
				tracked = tracked.Add(vault.OutstandingRewards.Ceil().TruncateInt())
			}
			return false
		})
		k.IteratePendingWithdrawals(ctx, func(withdrawal types.PendingWithdrawal) bool {
			tracked = tracked.Add(withdrawal.Amount)
			return false
		})

		bondDenom, err := k.stakingKeeper.BondDenom(ctx)
		if err != nil {
			return sdk.FormatInvariant(types.ModuleName, "module-account",
				fmt.Sprintf("failed to read bond denom: %s", err)), true
		}

		moduleAddr := authtypes.NewModuleAddress(types.ModuleName)
		held := k.bankKeeper.GetAllBalances(ctx, moduleAddr).AmountOf(bondDenom)
		broken := held.LT(tracked)

		return sdk.FormatInvariant(
			types.ModuleName, "module-account",
			fmt.Sprintf("liquidityvault module account holds %s %s, needs at least %s to back vaults and pending withdrawals", held, bondDenom, tracked),
		), broken
	}
}

// PoolSharesInvariant checks the share bookkeeping: each pool's total shares
// equals the sum of its positions' shares, and each (validator, pool)
// pending-share reservation equals the sum of that pair's queue entries and
// never exceeds the position's shares.
func PoolSharesInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		positionTotals := make(map[uint64]math.Int)
		type pairKey struct {
			validator string
			poolID    uint64
		}
		positionShares := make(map[pairKey]math.Int)
		k.IteratePositions(ctx, func(position types.PoolPosition) bool {
			total, ok := positionTotals[position.PoolId]
			if !ok {
				total = math.ZeroInt()
			}
			positionTotals[position.PoolId] = total.Add(position.Shares)
			positionShares[pairKey{position.ValidatorAddress, position.PoolId}] = position.Shares
			return false
		})

		for _, pool := range k.GetAllPools(ctx) {
			stored := k.GetPoolTotalShares(ctx, pool.PoolId)
			fromPositions, ok := positionTotals[pool.PoolId]
			if !ok {
				fromPositions = math.ZeroInt()
			}
			if !stored.Equal(fromPositions) {
				return sdk.FormatInvariant(types.ModuleName, "pool-shares",
					fmt.Sprintf("pool %d total shares %s != sum of positions %s", pool.PoolId, stored, fromPositions)), true
			}
		}

		queueSums := make(map[pairKey]math.Int)
		k.IteratePendingDeallocations(ctx, func(deallocation types.PendingDeallocation) bool {
			key := pairKey{deallocation.ValidatorAddress, deallocation.PoolId}
			sum, ok := queueSums[key]
			if !ok {
				sum = math.ZeroInt()
			}
			queueSums[key] = sum.Add(deallocation.Shares)
			return false
		})
		for key, sum := range queueSums {
			valAddr, err := sdk.ValAddressFromBech32(key.validator)
			if err != nil {
				return sdk.FormatInvariant(types.ModuleName, "pool-shares",
					fmt.Sprintf("queue entry has invalid validator %s", key.validator)), true
			}
			reserved := k.GetPendingShares(ctx, valAddr, key.poolID)
			if !reserved.Equal(sum) {
				return sdk.FormatInvariant(types.ModuleName, "pool-shares",
					fmt.Sprintf("pending-share reservation %s != queue sum %s for %s pool %d", reserved, sum, key.validator, key.poolID)), true
			}
			owned, ok := positionShares[key]
			if !ok || sum.GT(owned) {
				return sdk.FormatInvariant(types.ModuleName, "pool-shares",
					fmt.Sprintf("queued shares %s exceed position shares for %s pool %d", sum, key.validator, key.poolID)), true
			}
		}

		// The reverse direction: every reservation record must be backed by
		// queue entries, or it would permanently block the validator's
		// deallocations.
		var dangling string
		k.IteratePendingShareReservations(ctx, func(valAddr sdk.ValAddress, poolID uint64, pending math.Int) bool {
			if _, ok := queueSums[pairKey{valAddr.String(), poolID}]; !ok {
				dangling = fmt.Sprintf("reservation of %s shares for %s pool %d has no queue entries", pending, valAddr, poolID)
				return true
			}
			return false
		})
		if dangling != "" {
			return sdk.FormatInvariant(types.ModuleName, "pool-shares", dangling), true
		}

		return sdk.FormatInvariant(types.ModuleName, "pool-shares", "pool share bookkeeping is consistent"), false
	}
}
