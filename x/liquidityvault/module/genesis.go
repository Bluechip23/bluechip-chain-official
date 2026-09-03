package liquidityvault

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"bluechipChain/x/liquidityvault/keeper"
	"bluechipChain/x/liquidityvault/types"
)

// InitGenesis initializes the module's state from a provided genesis state.
// Pool total shares and pending-share reservations are derived state,
// recomputed here from the position and deallocation lists.
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
	for _, pool := range genState.Pools {
		if err := k.SetPool(ctx, pool); err != nil {
			panic(err)
		}
	}
	if genState.NextPoolId > 0 {
		k.SetNextPoolID(ctx, genState.NextPoolId)
	}
	if err := k.ImportPositions(ctx, genState.Positions); err != nil {
		panic(err)
	}
	if err := k.ImportPendingDeallocations(ctx, genState.PendingDeallocations); err != nil {
		panic(err)
	}
	for _, history := range genState.ValuePostHistories {
		if err := k.SetValuePostHistory(ctx, history); err != nil {
			panic(err)
		}
	}
	if err := k.ImportValuePostSchedule(ctx, genState.Vaults, genState.ScheduledValuePosts); err != nil {
		panic(err)
	}
	if err := k.ImportCachedPoolValues(ctx, genState.CachedPoolValues); err != nil {
		panic(err)
	}
}

// ExportGenesis returns the module's exported genesis.
func ExportGenesis(ctx sdk.Context, k keeper.Keeper) *types.GenesisState {
	genesis := types.DefaultGenesis()
	genesis.Params = k.GetParams(ctx)
	genesis.Vaults = k.GetAllVaults(ctx)
	genesis.PendingWithdrawals = k.GetAllPendingWithdrawals(ctx)
	genesis.Pools = k.GetAllPools(ctx)
	genesis.NextPoolId = k.GetNextPoolID(ctx)
	genesis.Positions = k.GetAllPositions(ctx)
	genesis.PendingDeallocations = k.GetAllPendingDeallocations(ctx)
	genesis.ValuePostHistories = k.GetAllValuePostHistories(ctx)
	genesis.ScheduledValuePosts = k.GetAllScheduledValuePosts(ctx)
	genesis.CachedPoolValues = k.GetAllCachedPoolValues(ctx)

	return genesis
}
