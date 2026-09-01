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
}

// ModuleAccountInvariant checks that the liquidityvault module account holds
// at least the sum of all active vault balances and pending withdrawals in
// the bond denom. (Stray sends could push the balance above the tracked sum;
// they are inert, so only a deficit breaks the invariant.)
func ModuleAccountInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		tracked := math.ZeroInt()
		k.IterateVaults(ctx, func(vault types.Vault) bool {
			tracked = tracked.Add(vault.Balance)
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
