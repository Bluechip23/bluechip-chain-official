package liquidityvault_test

import (
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	keepertest "bluechipChain/testutil/keeper"
	liquidityvault "bluechipChain/x/liquidityvault/module"
	"bluechipChain/x/liquidityvault/types"
)

func TestGenesis_DefaultState(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	defaultGenesis := types.DefaultGenesis()
	liquidityvault.InitGenesis(ctx, k, *defaultGenesis)

	exported := liquidityvault.ExportGenesis(ctx, k)
	require.NotNil(t, exported)

	// Verify params match defaults
	require.Equal(t, defaultGenesis.Params.StakeCap, exported.Params.StakeCap)
	require.Equal(t, defaultGenesis.Params.SimpleCheckInterval, exported.Params.SimpleCheckInterval)
	require.Equal(t, defaultGenesis.Params.ComplexCheckInterval, exported.Params.ComplexCheckInterval)
	require.Equal(t, defaultGenesis.Params.ValuePostsPerComplexInterval, exported.Params.ValuePostsPerComplexInterval)
	require.True(t, defaultGenesis.Params.DefaultDelegatorRewardPercent.Equal(exported.Params.DefaultDelegatorRewardPercent))

	// Verify check heights are zero
	require.Equal(t, int64(0), exported.LastSimpleCheckHeight)
	require.Equal(t, int64(0), exported.LastComplexCheckHeight)

	// Verify no validator records
	require.Empty(t, exported.ValidatorRecords)
}

func TestGenesis_CustomState(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	valAddr1 := "cosmosvaloper1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa000001"
	valAddr2 := "cosmosvaloper1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa000002"

	customParams := types.NewParams(
		math.NewInt(500_000_000_000),        // custom stake cap
		uint64(7200),                        // custom simple check interval
		uint64(36000),                       // custom complex check interval
		uint64(3),                           // custom value posts per complex interval
		math.LegacyNewDecWithPrec(75, 0),    // 75% delegator reward
	)

	vault1 := types.Vault{
		ValidatorAddress:       valAddr1,
		TotalDeposited:         sdk.NewCoin("ubluechip", math.NewInt(1000000)),
		DelegatorRewardPercent: math.LegacyNewDecWithPrec(50, 0),
		Positions: []types.PoolPosition{
			{
				PoolContractAddress: "cosmos1poolcontract1",
				PositionId:          "pos-1",
				DepositAmount0:      math.NewInt(500000),
				DepositAmount1:      math.NewInt(500000),
			},
		},
		ValidatorType: types.ValidatorType_VALIDATOR_TYPE_FULL,
	}

	vault2 := types.Vault{
		ValidatorAddress:       valAddr2,
		TotalDeposited:         sdk.NewCoin("ubluechip", math.NewInt(2000000)),
		DelegatorRewardPercent: math.LegacyNewDecWithPrec(60, 0),
		Positions:              []types.PoolPosition{},
		ValidatorType:          types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	}

	score1 := types.CompositeScore{
		ValidatorAddress: valAddr1,
		ChainStake:       math.NewInt(100000),
		VaultValue:       math.NewInt(200000),
	}

	score2 := types.CompositeScore{
		ValidatorAddress: valAddr2,
		ChainStake:       math.NewInt(300000),
		VaultValue:       math.NewInt(400000),
	}

	genesisState := types.GenesisState{
		Params: customParams,
		ValidatorRecords: []types.ValidatorRecord{
			{
				ValidatorAddress: valAddr1,
				ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_FULL,
				Vault:            &vault1,
				CompositeScore:   &score1,
			},
			{
				ValidatorAddress: valAddr2,
				ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
				Vault:            &vault2,
				CompositeScore:   &score2,
			},
		},
		LastSimpleCheckHeight:  int64(10000),
		LastComplexCheckHeight: int64(5000),
	}

	liquidityvault.InitGenesis(ctx, k, genesisState)

	exported := liquidityvault.ExportGenesis(ctx, k)
	require.NotNil(t, exported)

	// Verify custom params
	require.Equal(t, customParams.StakeCap, exported.Params.StakeCap)
	require.Equal(t, customParams.SimpleCheckInterval, exported.Params.SimpleCheckInterval)
	require.Equal(t, customParams.ComplexCheckInterval, exported.Params.ComplexCheckInterval)
	require.Equal(t, customParams.ValuePostsPerComplexInterval, exported.Params.ValuePostsPerComplexInterval)
	require.True(t, customParams.DefaultDelegatorRewardPercent.Equal(exported.Params.DefaultDelegatorRewardPercent))

	// Verify check heights
	require.Equal(t, int64(10000), exported.LastSimpleCheckHeight)
	require.Equal(t, int64(5000), exported.LastComplexCheckHeight)

	// Verify validator records count
	require.Len(t, exported.ValidatorRecords, 2)

	// Build a map of exported records by validator address for easier lookup
	recordMap := make(map[string]types.ValidatorRecord)
	for _, r := range exported.ValidatorRecords {
		recordMap[r.ValidatorAddress] = r
	}

	// Verify validator 1 record
	record1, ok := recordMap[valAddr1]
	require.True(t, ok, "validator 1 record should exist")
	require.Equal(t, valAddr1, record1.ValidatorAddress)
	require.Equal(t, types.ValidatorType_VALIDATOR_TYPE_FULL, record1.ValidatorType)

	require.NotNil(t, record1.Vault)
	require.Equal(t, valAddr1, record1.Vault.ValidatorAddress)
	require.Equal(t, sdk.NewCoin("ubluechip", math.NewInt(1000000)), record1.Vault.TotalDeposited)
	require.True(t, math.LegacyNewDecWithPrec(50, 0).Equal(record1.Vault.DelegatorRewardPercent))
	require.Len(t, record1.Vault.Positions, 1)
	require.Equal(t, "cosmos1poolcontract1", record1.Vault.Positions[0].PoolContractAddress)
	require.Equal(t, "pos-1", record1.Vault.Positions[0].PositionId)
	require.True(t, math.NewInt(500000).Equal(record1.Vault.Positions[0].DepositAmount0))
	require.True(t, math.NewInt(500000).Equal(record1.Vault.Positions[0].DepositAmount1))
	require.Equal(t, types.ValidatorType_VALIDATOR_TYPE_FULL, record1.Vault.ValidatorType)

	require.NotNil(t, record1.CompositeScore)
	require.Equal(t, valAddr1, record1.CompositeScore.ValidatorAddress)
	require.True(t, math.NewInt(100000).Equal(record1.CompositeScore.ChainStake))
	require.True(t, math.NewInt(200000).Equal(record1.CompositeScore.VaultValue))

	// Verify validator 2 record
	record2, ok := recordMap[valAddr2]
	require.True(t, ok, "validator 2 record should exist")
	require.Equal(t, valAddr2, record2.ValidatorAddress)
	require.Equal(t, types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY, record2.ValidatorType)

	require.NotNil(t, record2.Vault)
	require.Equal(t, valAddr2, record2.Vault.ValidatorAddress)
	require.Equal(t, sdk.NewCoin("ubluechip", math.NewInt(2000000)), record2.Vault.TotalDeposited)
	require.True(t, math.LegacyNewDecWithPrec(60, 0).Equal(record2.Vault.DelegatorRewardPercent))
	require.Empty(t, record2.Vault.Positions)
	require.Equal(t, types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY, record2.Vault.ValidatorType)

	require.NotNil(t, record2.CompositeScore)
	require.Equal(t, valAddr2, record2.CompositeScore.ValidatorAddress)
	require.True(t, math.NewInt(300000).Equal(record2.CompositeScore.ChainStake))
	require.True(t, math.NewInt(400000).Equal(record2.CompositeScore.VaultValue))
}

func TestGenesis_EmptyValidatorRecords(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	genesisState := types.GenesisState{
		Params:                 types.DefaultParams(),
		ValidatorRecords:       []types.ValidatorRecord{},
		LastSimpleCheckHeight:  0,
		LastComplexCheckHeight: 0,
	}

	liquidityvault.InitGenesis(ctx, k, genesisState)

	exported := liquidityvault.ExportGenesis(ctx, k)
	require.NotNil(t, exported)

	// Verify default params are preserved
	require.Equal(t, types.DefaultParams().StakeCap, exported.Params.StakeCap)
	require.Equal(t, types.DefaultParams().SimpleCheckInterval, exported.Params.SimpleCheckInterval)
	require.Equal(t, types.DefaultParams().ComplexCheckInterval, exported.Params.ComplexCheckInterval)
	require.Equal(t, types.DefaultParams().ValuePostsPerComplexInterval, exported.Params.ValuePostsPerComplexInterval)
	require.True(t, types.DefaultParams().DefaultDelegatorRewardPercent.Equal(exported.Params.DefaultDelegatorRewardPercent))

	// Verify empty validator records
	require.Empty(t, exported.ValidatorRecords)

	// Verify zero check heights
	require.Equal(t, int64(0), exported.LastSimpleCheckHeight)
	require.Equal(t, int64(0), exported.LastComplexCheckHeight)
}

func TestGenesis_Validation(t *testing.T) {
	// Default genesis should validate successfully
	defaultGenesis := types.DefaultGenesis()
	err := defaultGenesis.Validate()
	require.NoError(t, err)

	// Genesis with negative stake cap should fail validation
	invalidGenesis := types.GenesisState{
		Params: types.NewParams(
			math.NewInt(-1),                      // negative stake cap
			uint64(14400),
			uint64(72000),
			uint64(6),
			math.LegacyNewDecWithPrec(50, 0),
		),
		ValidatorRecords:       []types.ValidatorRecord{},
		LastSimpleCheckHeight:  0,
		LastComplexCheckHeight: 0,
	}
	err = invalidGenesis.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "stake cap cannot be negative")

	// Genesis with zero simple check interval should fail validation
	invalidGenesis2 := types.GenesisState{
		Params: types.NewParams(
			math.NewInt(1000),
			uint64(0),                            // zero simple check interval
			uint64(72000),
			uint64(6),
			math.LegacyNewDecWithPrec(50, 0),
		),
		ValidatorRecords:       []types.ValidatorRecord{},
		LastSimpleCheckHeight:  0,
		LastComplexCheckHeight: 0,
	}
	err = invalidGenesis2.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "simple check interval must be positive")

	// Genesis with zero complex check interval should fail validation
	invalidGenesis3 := types.GenesisState{
		Params: types.NewParams(
			math.NewInt(1000),
			uint64(14400),
			uint64(0),                            // zero complex check interval
			uint64(6),
			math.LegacyNewDecWithPrec(50, 0),
		),
		ValidatorRecords:       []types.ValidatorRecord{},
		LastSimpleCheckHeight:  0,
		LastComplexCheckHeight: 0,
	}
	err = invalidGenesis3.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "complex check interval must be positive")

	// Genesis with zero value posts per complex interval should fail validation
	invalidGenesis4 := types.GenesisState{
		Params: types.NewParams(
			math.NewInt(1000),
			uint64(14400),
			uint64(72000),
			uint64(0),                            // zero value posts
			math.LegacyNewDecWithPrec(50, 0),
		),
		ValidatorRecords:       []types.ValidatorRecord{},
		LastSimpleCheckHeight:  0,
		LastComplexCheckHeight: 0,
	}
	err = invalidGenesis4.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "value posts per complex interval must be positive")

	// Genesis with delegator reward percent > 100 should fail validation
	invalidGenesis5 := types.GenesisState{
		Params: types.NewParams(
			math.NewInt(1000),
			uint64(14400),
			uint64(72000),
			uint64(6),
			math.LegacyNewDecWithPrec(101, 0),   // 101% - out of range
		),
		ValidatorRecords:       []types.ValidatorRecord{},
		LastSimpleCheckHeight:  0,
		LastComplexCheckHeight: 0,
	}
	err = invalidGenesis5.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "default delegator reward percent must be between 0 and 100")

	// Genesis with negative delegator reward percent should fail validation
	invalidGenesis6 := types.GenesisState{
		Params: types.NewParams(
			math.NewInt(1000),
			uint64(14400),
			uint64(72000),
			uint64(6),
			math.LegacyNewDec(-1),                // negative
		),
		ValidatorRecords:       []types.ValidatorRecord{},
		LastSimpleCheckHeight:  0,
		LastComplexCheckHeight: 0,
	}
	err = invalidGenesis6.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "default delegator reward percent must be between 0 and 100")
}
