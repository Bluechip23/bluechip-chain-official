package liquidityvault

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"bluechipChain/x/liquidityvault/keeper"
	"bluechipChain/x/liquidityvault/types"
)

// InitGenesis initializes the module's state from a provided genesis state.
func InitGenesis(ctx sdk.Context, k keeper.Keeper, genState types.GenesisState) {
	if err := k.SetParams(ctx, genState.Params); err != nil {
		panic(err)
	}
	for _, vault := range genState.Vaults {
		if err := k.SetVault(ctx, vault); err != nil {
			panic(err)
		}
	}
	for _, withdrawal := range genState.PendingWithdrawals {
		if err := k.SetPendingWithdrawal(ctx, withdrawal); err != nil {
			panic(err)
		}
	}
}

// ExportGenesis returns the module's exported genesis.
func ExportGenesis(ctx sdk.Context, k keeper.Keeper) *types.GenesisState {
	genesis := types.DefaultGenesis()
	genesis.Params = k.GetParams(ctx)
	genesis.Vaults = k.GetAllVaults(ctx)
	genesis.PendingWithdrawals = k.GetAllPendingWithdrawals(ctx)

	return genesis
}
