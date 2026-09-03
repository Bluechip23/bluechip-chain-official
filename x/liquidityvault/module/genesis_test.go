package liquidityvault_test

import (
	"testing"
	"time"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	keepertest "bluechipChain/testutil/keeper"
	"bluechipChain/testutil/nullify"
	liquidityvault "bluechipChain/x/liquidityvault/module"
	"bluechipChain/x/liquidityvault/types"
)

func TestGenesisRoundTrip(t *testing.T) {
	valAddr := sdk.ValAddress([]byte("validator-operator-1")).String()
	genesisState := types.GenesisState{
		Params: types.NewParams(math.NewInt(1_000_000), 48*time.Hour, 24*time.Hour, 20*time.Hour),
		Vaults: []types.Vault{
			{
				ValidatorAddress:     valAddr,
				Balance:              math.NewInt(750),
				DelegatorRewardShare: math.LegacyNewDecWithPrec(6, 1),
			},
		},
		PendingWithdrawals: []types.PendingWithdrawal{
			{
				ValidatorAddress: valAddr,
				Amount:           math.NewInt(250),
				CompleteTime:     time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC),
			},
		},
		ValuePostHistories: []types.ValuePostHistory{
			{
				ValidatorAddress: valAddr,
				Posts: []types.ValuePost{
					{Value: math.NewInt(700), PostTime: time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)},
					{Value: math.NewInt(750), PostTime: time.Date(2026, 9, 2, 4, 0, 0, 0, time.UTC)},
				},
			},
		},
		ScheduledValuePosts: []types.ScheduledValuePost{
			{
				ValidatorAddress: valAddr,
				PostTime:         time.Date(2026, 9, 3, 1, 30, 0, 0, time.UTC),
			},
		},
		Pools: []types.RegisteredPool{
			{
				PoolId:          1,
				ContractAddress: sdk.AccAddress([]byte("pool-contract-1-----")).String(),
				Description:     "creator pool one",
				Enabled:         true,
			},
		},
		NextPoolId: 2,
		Positions: []types.PoolPosition{
			{
				ValidatorAddress: valAddr,
				PoolId:           1,
				Shares:           math.NewInt(120),
			},
		},
		CachedPoolValues: []types.CachedPoolValue{
			{PoolId: 1, Value: math.NewInt(180)},
		},
	}
	require.NoError(t, genesisState.Validate())

	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)
	liquidityvault.InitGenesis(ctx, k, genesisState)
	got := liquidityvault.ExportGenesis(ctx, k)
	require.NotNil(t, got)

	nullify.Fill(&genesisState)
	nullify.Fill(got)

	require.Equal(t, genesisState.Params, got.Params)
	require.ElementsMatch(t, genesisState.Vaults, got.Vaults)
	require.ElementsMatch(t, genesisState.PendingWithdrawals, got.PendingWithdrawals)
	require.ElementsMatch(t, genesisState.ValuePostHistories, got.ValuePostHistories)
	require.ElementsMatch(t, genesisState.ScheduledValuePosts, got.ScheduledValuePosts)
	require.ElementsMatch(t, genesisState.Pools, got.Pools)
	require.Equal(t, genesisState.NextPoolId, got.NextPoolId)
	require.ElementsMatch(t, genesisState.Positions, got.Positions)
	require.ElementsMatch(t, genesisState.CachedPoolValues, got.CachedPoolValues)
}
