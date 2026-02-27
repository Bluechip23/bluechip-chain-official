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

	// Restore validator records
	for _, record := range genState.ValidatorRecords {
		if record.Vault != nil {
			if err := k.SetVault(ctx, *record.Vault); err != nil {
				panic(err)
			}
		}
		k.SetValidatorType(ctx, record.ValidatorAddress, record.ValidatorType)
		if record.CompositeScore != nil {
			if err := k.SetCompositeScore(ctx, *record.CompositeScore); err != nil {
				panic(err)
			}
		}
	}

	// Restore check heights
	if genState.LastSimpleCheckHeight > 0 {
		k.SetLastSimpleCheckHeight(ctx, int64(genState.LastSimpleCheckHeight))
	}
	if genState.LastComplexCheckHeight > 0 {
		k.SetLastComplexCheckHeight(ctx, int64(genState.LastComplexCheckHeight))
	}
}

// ExportGenesis returns the module's exported genesis.
func ExportGenesis(ctx sdk.Context, k keeper.Keeper) *types.GenesisState {
	genesis := types.DefaultGenesis()
	genesis.Params = k.GetParams(ctx)
	genesis.LastSimpleCheckHeight = uint64(k.GetLastSimpleCheckHeight(ctx))
	genesis.LastComplexCheckHeight = uint64(k.GetLastComplexCheckHeight(ctx))

	// Export all validator records
	vaults := k.GetAllVaults(ctx)
	for _, vault := range vaults {
		v := vault // copy to avoid pointer issues
		record := types.ValidatorRecord{
			ValidatorAddress: vault.ValidatorAddress,
			ValidatorType:    vault.ValidatorType,
			Vault:            &v,
		}

		score, found := k.GetCompositeScore(ctx, vault.ValidatorAddress)
		if found {
			record.CompositeScore = &score
		}

		genesis.ValidatorRecords = append(genesis.ValidatorRecords, record)
	}

	return genesis
}
