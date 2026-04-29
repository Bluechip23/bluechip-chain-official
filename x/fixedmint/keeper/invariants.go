package keeper

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"bluechipChain/x/fixedmint/types"
)

// RegisterInvariants registers all module invariants.
func RegisterInvariants(ir sdk.InvariantRegistry, k Keeper) {
	ir.RegisterRoute(types.ModuleName, "module-account", ModuleAccountInvariant(k))
}

// ModuleAccountInvariant checks that the fixedmint module account exists and has
// no residual balance (all minted coins should be forwarded to the fee collector).
func ModuleAccountInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		moduleAddr := authtypes.NewModuleAddress(types.ModuleName)
		coins := k.bankKeeper.SpendableCoins(ctx, moduleAddr)
		broken := !coins.IsZero()

		return sdk.FormatInvariant(
			types.ModuleName, "module-account",
			fmt.Sprintf("fixedmint module account should have zero balance, has: %s", coins),
		), broken
	}
}
