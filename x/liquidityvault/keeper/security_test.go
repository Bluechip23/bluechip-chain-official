package keeper_test

import (
	"math/big"
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/stretchr/testify/require"

	keepertest "bluechipChain/testutil/keeper"
	"bluechipChain/x/liquidityvault/keeper"
	"bluechipChain/x/liquidityvault/types"
)

// ---------------------------------------------------------------------------
// 1. Authorization Enforcement
// ---------------------------------------------------------------------------

func TestAuthorizationEnforcement(t *testing.T) {
	_, msgServer, ctx, _, _ := setupSecurityMsgServer(t)

	validParams := types.DefaultParams()
	correctAuthority := authtypes.NewModuleAddress(govtypes.ModuleName).String()

	// Correct authority succeeds
	resp, err := msgServer.UpdateParams(ctx, &types.MsgUpdateParams{
		Authority: correctAuthority,
		Params:    validParams,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Random addresses must be rejected
	randomAddresses := []string{
		sdk.AccAddress([]byte("random_user_1_____")).String(),
		sdk.AccAddress([]byte("random_user_2_____")).String(),
		sdk.AccAddress([]byte("attacker__________")).String(),
		authtypes.NewModuleAddress("staking").String(),
		authtypes.NewModuleAddress("bank").String(),
	}

	for _, addr := range randomAddresses {
		t.Run("reject_"+addr[:20], func(t *testing.T) {
			resp, err := msgServer.UpdateParams(ctx, &types.MsgUpdateParams{
				Authority: addr,
				Params:    validParams,
			})
			require.Error(t, err)
			require.Nil(t, resp)
			require.ErrorIs(t, err, types.ErrInvalidSigner)
		})
	}
}

// ---------------------------------------------------------------------------
// 2. Stake Cap Enforcement
// ---------------------------------------------------------------------------

func TestStakeCapEnforcement(t *testing.T) {
	k, ctx, _, sk, _ := keepertest.LiquidityvaultKeeper(t)

	valAddr := sdk.ValAddress([]byte("val_stake_cap_____"))

	t.Run("check stake cap returns excess when exceeded", func(t *testing.T) {
		// Set a low stake cap
		params := types.DefaultParams()
		params.StakeCap = math.NewInt(1000)
		require.NoError(t, k.SetParams(ctx, params))

		// Add validator with tokens near cap
		sk.AddValidator(valAddr, math.NewInt(900))

		// Check with additional amount that exceeds cap: 900 + 200 = 1100 > 1000
		excess, err := k.CheckStakeCap(ctx, valAddr, math.NewInt(200))
		require.NoError(t, err)
		require.True(t, excess.IsPositive(), "excess should be positive")
		require.Equal(t, math.NewInt(100), excess, "excess should be 100 (1100 - 1000)")
	})

	t.Run("check stake cap returns zero when within cap", func(t *testing.T) {
		excess, err := k.CheckStakeCap(ctx, valAddr, math.NewInt(50))
		require.NoError(t, err)
		require.True(t, excess.IsZero(), "excess should be zero when within cap")
	})

	t.Run("enforce stake cap returns error when exceeded", func(t *testing.T) {
		err := k.EnforceStakeCap(ctx, valAddr, math.NewInt(200))
		require.Error(t, err)
		require.ErrorIs(t, err, types.ErrStakeCapExceeded)
	})

	t.Run("enforce stake cap succeeds when within cap", func(t *testing.T) {
		err := k.EnforceStakeCap(ctx, valAddr, math.NewInt(50))
		require.NoError(t, err)
	})

	t.Run("zero stake cap means no limit enforced", func(t *testing.T) {
		params := types.DefaultParams()
		params.StakeCap = math.ZeroInt()
		require.NoError(t, k.SetParams(ctx, params))

		// Even a very large additional stake should succeed
		excess, err := k.CheckStakeCap(ctx, valAddr, math.NewInt(999_999_999_999))
		require.NoError(t, err)
		require.True(t, excess.IsZero(), "no cap should be enforced when stake cap is zero")

		err = k.EnforceStakeCap(ctx, valAddr, math.NewInt(999_999_999_999))
		require.NoError(t, err)
	})
}

// ---------------------------------------------------------------------------
// 3. Double Registration
// ---------------------------------------------------------------------------

func TestDoubleRegistration(t *testing.T) {
	_, msgServer, ctx, _, _ := setupSecurityMsgServer(t)

	valAddr := sdk.ValAddress([]byte("val_double_reg____")).String()

	// First registration succeeds
	resp, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Second registration must fail with ErrVaultAlreadyExists
	resp, err = msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	})
	require.Error(t, err)
	require.Nil(t, resp)
	require.ErrorIs(t, err, types.ErrVaultAlreadyExists)

	// Attempting with the same type again should also fail
	resp, err = msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	})
	require.Error(t, err)
	require.Nil(t, resp)
	require.ErrorIs(t, err, types.ErrVaultAlreadyExists)
}

// ---------------------------------------------------------------------------
// 4. Delegator Reward Percent Boundaries
// ---------------------------------------------------------------------------

func TestDelegatorRewardPercentBoundaries(t *testing.T) {
	k, msgServer, ctx, _, _ := setupSecurityMsgServer(t)

	valAddr := sdk.ValAddress([]byte("val_boundary______")).String()

	// Register the validator
	_, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	})
	require.NoError(t, err)

	tests := []struct {
		name      string
		percent   math.LegacyDec
		expectErr bool
	}{
		{
			name:      "exactly 0 percent - should succeed",
			percent:   math.LegacyNewDec(0),
			expectErr: false,
		},
		{
			name:      "exactly 100 percent - should succeed",
			percent:   math.LegacyNewDec(100),
			expectErr: false,
		},
		{
			name:      "slightly negative (-0.01) - should fail",
			percent:   math.LegacyNewDecWithPrec(-1, 2), // -0.01
			expectErr: true,
		},
		{
			name:      "slightly over (100.01) - should fail",
			percent:   math.LegacyNewDecWithPrec(10001, 2), // 100.01
			expectErr: true,
		},
		{
			name:      "very large value (999999) - should fail",
			percent:   math.LegacyNewDec(999999),
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := msgServer.SetDelegatorRewardPercent(ctx, &types.MsgSetDelegatorRewardPercent{
				ValidatorAddress: valAddr,
				Percent:          tc.percent,
			})
			if tc.expectErr {
				require.Error(t, err)
				require.Nil(t, resp)
				require.ErrorIs(t, err, types.ErrInvalidDelegatorPercent)
			} else {
				require.NoError(t, err)
				require.NotNil(t, resp)

				// Verify the update persisted
				vault, found := k.GetVault(ctx, valAddr)
				require.True(t, found)
				require.True(t, vault.DelegatorRewardPercent.Equal(tc.percent),
					"expected %s, got %s", tc.percent, vault.DelegatorRewardPercent)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 5. Vault Isolation
// ---------------------------------------------------------------------------

func TestVaultIsolation(t *testing.T) {
	k, msgServer, ctx, _, _ := setupSecurityMsgServer(t)

	valAddr1 := sdk.ValAddress([]byte("val_isolated_1____")).String()
	valAddr2 := sdk.ValAddress([]byte("val_isolated_2____")).String()

	// Register two validators
	_, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr1,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	})
	require.NoError(t, err)

	_, err = msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr2,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	})
	require.NoError(t, err)

	// Get initial state of vault2
	vault2Before, found := k.GetVault(ctx, valAddr2)
	require.True(t, found)
	initialPercent2 := vault2Before.DelegatorRewardPercent

	// Modify vault1 delegator reward percent
	_, err = msgServer.SetDelegatorRewardPercent(ctx, &types.MsgSetDelegatorRewardPercent{
		ValidatorAddress: valAddr1,
		Percent:          math.LegacyNewDec(99),
	})
	require.NoError(t, err)

	// Verify vault1 was changed
	vault1After, found := k.GetVault(ctx, valAddr1)
	require.True(t, found)
	require.True(t, vault1After.DelegatorRewardPercent.Equal(math.LegacyNewDec(99)))

	// Verify vault2 was NOT changed
	vault2After, found := k.GetVault(ctx, valAddr2)
	require.True(t, found)
	require.True(t, vault2After.DelegatorRewardPercent.Equal(initialPercent2),
		"vault2 should be unchanged; expected %s, got %s", initialPercent2, vault2After.DelegatorRewardPercent)
	require.Equal(t, vault2Before.ValidatorAddress, vault2After.ValidatorAddress)
	require.Equal(t, vault2Before.TotalDeposited, vault2After.TotalDeposited)
	require.Equal(t, vault2Before.ValidatorType, vault2After.ValidatorType)
}

// ---------------------------------------------------------------------------
// 6. Param Validation (table-driven)
// ---------------------------------------------------------------------------

func TestParamValidation(t *testing.T) {
	tests := []struct {
		name   string
		params types.Params
		errMsg string
	}{
		{
			name: "negative stake cap",
			params: types.NewParams(
				math.NewInt(-100),
				uint64(14400),
				uint64(72000),
				uint64(6),
				math.LegacyNewDec(50),
			),
			errMsg: "stake cap cannot be negative",
		},
		{
			name: "zero simple check interval",
			params: types.NewParams(
				math.NewInt(1_000_000),
				uint64(0),
				uint64(72000),
				uint64(6),
				math.LegacyNewDec(50),
			),
			errMsg: "simple check interval must be positive",
		},
		{
			name: "zero complex check interval",
			params: types.NewParams(
				math.NewInt(1_000_000),
				uint64(14400),
				uint64(0),
				uint64(6),
				math.LegacyNewDec(50),
			),
			errMsg: "complex check interval must be positive",
		},
		{
			name: "zero value posts per interval",
			params: types.NewParams(
				math.NewInt(1_000_000),
				uint64(14400),
				uint64(72000),
				uint64(0),
				math.LegacyNewDec(50),
			),
			errMsg: "value posts per complex interval must be positive",
		},
		{
			name: "negative delegator reward percent",
			params: types.NewParams(
				math.NewInt(1_000_000),
				uint64(14400),
				uint64(72000),
				uint64(6),
				math.LegacyNewDec(-5),
			),
			errMsg: "default delegator reward percent must be between 0 and 100",
		},
		{
			name: "delegator reward percent over 100",
			params: types.NewParams(
				math.NewInt(1_000_000),
				uint64(14400),
				uint64(72000),
				uint64(6),
				math.LegacyNewDec(150),
			),
			errMsg: "default delegator reward percent must be between 0 and 100",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.params.Validate()
			require.Error(t, err)
			require.ErrorContains(t, err, tc.errMsg)
		})
	}

	// Positive case: valid params should pass
	t.Run("valid params pass validation", func(t *testing.T) {
		validParams := types.DefaultParams()
		err := validParams.Validate()
		require.NoError(t, err)
	})

	// Edge case: zero stake cap is valid (means no cap)
	t.Run("zero stake cap is valid", func(t *testing.T) {
		params := types.NewParams(
			math.ZeroInt(),
			uint64(14400),
			uint64(72000),
			uint64(6),
			math.LegacyNewDec(50),
		)
		err := params.Validate()
		require.NoError(t, err)
	})
}

// ---------------------------------------------------------------------------
// 7. Composite Score Overflow
// ---------------------------------------------------------------------------

func TestCompositeScoreOverflow(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	t.Run("very large chain stake values do not panic", func(t *testing.T) {
		// Create a composite score with very large math.Int values using big.Int
		// to safely construct large numbers without overflow in Mul
		bigTen := big.NewInt(10)
		exp := new(big.Int).Exp(bigTen, big.NewInt(77), nil) // 10^77 -- a very large number
		largeVal := math.NewIntFromBigInt(exp)

		score := types.CompositeScore{
			ValidatorAddress: "val_overflow_1",
			ChainStake:       largeVal,
			VaultValue:       largeVal,
		}

		// Setting and getting should not panic
		require.NotPanics(t, func() {
			err := k.SetCompositeScore(ctx, score)
			require.NoError(t, err)
		})

		require.NotPanics(t, func() {
			retrieved, found := k.GetCompositeScore(ctx, "val_overflow_1")
			require.True(t, found)
			require.True(t, retrieved.ChainStake.Equal(largeVal))
			require.True(t, retrieved.VaultValue.Equal(largeVal))
		})
	})

	t.Run("ranking with large values does not panic", func(t *testing.T) {
		// Build 10^30 using big.Int to avoid overflow in math.Int.Mul
		exp30 := new(big.Int).Exp(big.NewInt(10), big.NewInt(30), nil)
		bigVal1 := math.NewIntFromBigInt(exp30)
		bigVal2 := bigVal1.Add(math.NewInt(1))

		score1 := types.CompositeScore{
			ValidatorAddress: "val_big_rank_1",
			ChainStake:       bigVal1,
			VaultValue:       bigVal2,
		}
		score2 := types.CompositeScore{
			ValidatorAddress: "val_big_rank_2",
			ChainStake:       bigVal2,
			VaultValue:       bigVal1,
		}

		require.NoError(t, k.SetCompositeScore(ctx, score1))
		require.NoError(t, k.SetCompositeScore(ctx, score2))

		require.NotPanics(t, func() {
			rankings := k.GetRankedValidators(ctx)
			require.GreaterOrEqual(t, len(rankings), 2)
		})
	})

	t.Run("CalculateMedianValue with large values does not panic", func(t *testing.T) {
		exp30 := new(big.Int).Exp(big.NewInt(10), big.NewInt(30), nil)
		largeVal := math.NewIntFromBigInt(exp30)
		posts := []types.ValuePost{
			{ValidatorAddress: "v1", Value: largeVal, BlockHeight: 1},
			{ValidatorAddress: "v1", Value: largeVal.Add(math.NewInt(1)), BlockHeight: 2},
			{ValidatorAddress: "v1", Value: largeVal.Sub(math.NewInt(1)), BlockHeight: 3},
		}

		require.NotPanics(t, func() {
			median := keeper.CalculateMedianValue(posts)
			require.True(t, median.IsPositive())
		})
	})
}

// ---------------------------------------------------------------------------
// 8. Empty State Queries
// ---------------------------------------------------------------------------

func TestEmptyStateQueries(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	t.Run("GetAllVaults on empty state returns empty slice", func(t *testing.T) {
		vaults := k.GetAllVaults(ctx)
		// Should return nil or empty, not panic
		require.NotPanics(t, func() {
			_ = len(vaults)
		})
		require.Empty(t, vaults)
	})

	t.Run("GetAllCompositeScores on empty state returns empty slice", func(t *testing.T) {
		scores := k.GetAllCompositeScores(ctx)
		require.NotPanics(t, func() {
			_ = len(scores)
		})
		require.Empty(t, scores)
	})

	t.Run("GetRankedValidators on empty state returns empty slice", func(t *testing.T) {
		rankings := k.GetRankedValidators(ctx)
		require.NotPanics(t, func() {
			_ = len(rankings)
		})
		require.Empty(t, rankings)
	})

	t.Run("AllVaults query on empty state returns valid response", func(t *testing.T) {
		resp, err := k.AllVaults(ctx, &types.QueryAllVaultsRequest{})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Empty(t, resp.Vaults)
		require.NotNil(t, resp.Pagination)
		require.Equal(t, uint64(0), resp.Pagination.Total)
	})

	t.Run("ValidatorRankings query on empty state returns valid response", func(t *testing.T) {
		resp, err := k.ValidatorRankings(ctx, &types.QueryValidatorRankingsRequest{})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Empty(t, resp.Scores)
		require.NotNil(t, resp.Pagination)
		require.Equal(t, uint64(0), resp.Pagination.Total)
	})

	t.Run("Vault query for nonexistent validator returns error not panic", func(t *testing.T) {
		require.NotPanics(t, func() {
			resp, err := k.Vault(ctx, &types.QueryVaultRequest{
				ValidatorAddress: "nonexistent_validator",
			})
			require.Error(t, err)
			require.Nil(t, resp)
		})
	})

	t.Run("CompositeScore query for nonexistent validator returns error not panic", func(t *testing.T) {
		require.NotPanics(t, func() {
			resp, err := k.CompositeScore(ctx, &types.QueryCompositeScoreRequest{
				ValidatorAddress: "nonexistent_validator",
			})
			require.Error(t, err)
			require.Nil(t, resp)
		})
	})

	t.Run("GetVault for nonexistent validator returns false", func(t *testing.T) {
		vault, found := k.GetVault(ctx, "does_not_exist")
		require.False(t, found)
		require.Equal(t, types.Vault{}, vault)
	})

	t.Run("GetCompositeScore for nonexistent validator returns false", func(t *testing.T) {
		score, found := k.GetCompositeScore(ctx, "does_not_exist")
		require.False(t, found)
		require.Equal(t, types.CompositeScore{}, score)
	})
}

// ---------------------------------------------------------------------------
// Helper: setup message server for security tests
// ---------------------------------------------------------------------------

func setupSecurityMsgServer(t testing.TB) (keeper.Keeper, types.MsgServer, sdk.Context, *keepertest.MockBankKeeper, *keepertest.MockStakingKeeper) {
	k, ctx, bk, sk, _ := keepertest.LiquidityvaultKeeper(t)
	return k, keeper.NewMsgServerImpl(k), ctx, bk, sk
}
