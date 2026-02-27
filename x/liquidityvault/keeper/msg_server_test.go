package keeper_test

import (
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

func setupMsgServer(t testing.TB) (keeper.Keeper, types.MsgServer, sdk.Context, *keepertest.MockBankKeeper, *keepertest.MockStakingKeeper) {
	k, ctx, bk, sk, _ := keepertest.LiquidityvaultKeeper(t)
	return k, keeper.NewMsgServerImpl(k), ctx, bk, sk
}

// ---------------------------------------------------------------------------
// MsgUpdateParams tests
// ---------------------------------------------------------------------------

func TestMsgUpdateParams_ValidUpdate(t *testing.T) {
	_, msgServer, ctx, _, _ := setupMsgServer(t)

	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	newParams := types.NewParams(
		math.NewInt(500_000_000_000),
		uint64(7200),
		uint64(36000),
		uint64(3),
		math.LegacyNewDec(75),
	)

	resp, err := msgServer.UpdateParams(ctx, &types.MsgUpdateParams{
		Authority: authority,
		Params:    newParams,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestMsgUpdateParams_InvalidAuthority(t *testing.T) {
	_, msgServer, ctx, _, _ := setupMsgServer(t)

	validParams := types.DefaultParams()

	tests := []struct {
		name      string
		authority string
	}{
		{
			name:      "wrong address",
			authority: sdk.AccAddress([]byte("wrong_authority_addr")).String(),
		},
		{
			name:      "empty authority",
			authority: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := msgServer.UpdateParams(ctx, &types.MsgUpdateParams{
				Authority: tc.authority,
				Params:    validParams,
			})
			require.Error(t, err)
			require.Nil(t, resp)
			if tc.authority != "" {
				require.ErrorIs(t, err, types.ErrInvalidSigner)
			}
		})
	}
}

func TestMsgUpdateParams_InvalidParams(t *testing.T) {
	_, msgServer, ctx, _, _ := setupMsgServer(t)

	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()

	tests := []struct {
		name   string
		params types.Params
	}{
		{
			name: "negative stake cap",
			params: types.NewParams(
				math.NewInt(-1),
				uint64(14400),
				uint64(72000),
				uint64(6),
				math.LegacyNewDec(50),
			),
		},
		{
			name: "zero simple check interval",
			params: types.NewParams(
				math.NewInt(1_000_000_000_000),
				uint64(0),
				uint64(72000),
				uint64(6),
				math.LegacyNewDec(50),
			),
		},
		{
			name: "zero complex check interval",
			params: types.NewParams(
				math.NewInt(1_000_000_000_000),
				uint64(14400),
				uint64(0),
				uint64(6),
				math.LegacyNewDec(50),
			),
		},
		{
			name: "zero value posts per interval",
			params: types.NewParams(
				math.NewInt(1_000_000_000_000),
				uint64(14400),
				uint64(72000),
				uint64(0),
				math.LegacyNewDec(50),
			),
		},
		{
			name: "delegator reward percent greater than 100",
			params: types.NewParams(
				math.NewInt(1_000_000_000_000),
				uint64(14400),
				uint64(72000),
				uint64(6),
				math.LegacyNewDec(101),
			),
		},
		{
			name: "negative delegator reward percent",
			params: types.NewParams(
				math.NewInt(1_000_000_000_000),
				uint64(14400),
				uint64(72000),
				uint64(6),
				math.LegacyNewDec(-1),
			),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := msgServer.UpdateParams(ctx, &types.MsgUpdateParams{
				Authority: authority,
				Params:    tc.params,
			})
			require.Error(t, err)
			require.Nil(t, resp)
		})
	}
}

// ---------------------------------------------------------------------------
// MsgRegisterValidator tests
// ---------------------------------------------------------------------------

func TestMsgRegisterValidator_LiquidityType(t *testing.T) {
	// LIQUIDITY type does not require a staking validator
	k, msgServer, ctx, _, sk := setupMsgServer(t)

	valAddr := sdk.ValAddress([]byte("validator1________")).String()
	sk.AddValidator(sdk.ValAddress([]byte("validator1________")), math.NewInt(1000000))

	resp, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify vault was created
	vault, found := k.GetVault(ctx, valAddr)
	require.True(t, found)
	require.Equal(t, valAddr, vault.ValidatorAddress)
	require.Equal(t, types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY, vault.ValidatorType)
}

func TestMsgRegisterValidator_FullType_WithStakingValidator(t *testing.T) {
	k, msgServer, ctx, _, sk := setupMsgServer(t)

	valAddr := sdk.ValAddress([]byte("validator_full____"))
	sk.AddValidator(valAddr, math.NewInt(100_000))

	resp, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr.String(),
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_FULL,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify vault was created with correct type
	vault, found := k.GetVault(ctx, valAddr.String())
	require.True(t, found)
	require.Equal(t, types.ValidatorType_VALIDATOR_TYPE_FULL, vault.ValidatorType)
}

func TestMsgRegisterValidator_FullType_NotAStakingValidator(t *testing.T) {
	_, msgServer, ctx, _, _ := setupMsgServer(t)

	// Do NOT add this address to the staking keeper
	valAddr := sdk.ValAddress([]byte("not_a_validator___")).String()

	resp, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_FULL,
	})
	require.Error(t, err)
	require.Nil(t, resp)
	require.ErrorContains(t, err, "not a registered staking validator")
}

func TestMsgRegisterValidator_UnspecifiedType(t *testing.T) {
	_, msgServer, ctx, _, _ := setupMsgServer(t)

	valAddr := sdk.ValAddress([]byte("validator_unspec__")).String()

	resp, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_UNSPECIFIED,
	})
	require.Error(t, err)
	require.Nil(t, resp)
	require.ErrorIs(t, err, types.ErrInvalidValidatorType)
}

func TestMsgRegisterValidator_Duplicate(t *testing.T) {
	_, msgServer, ctx, _, sk := setupMsgServer(t)

	valAddr := sdk.ValAddress([]byte("validator_dup_____")).String()
	sk.AddValidator(sdk.ValAddress([]byte("validator_dup_____")), math.NewInt(1000000))

	// First registration succeeds
	resp, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Second registration fails
	resp, err = msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	})
	require.Error(t, err)
	require.Nil(t, resp)
	require.ErrorIs(t, err, types.ErrVaultAlreadyExists)
}

func TestMsgRegisterValidator_MultipleTypes(t *testing.T) {
	k, msgServer, ctx, _, sk := setupMsgServer(t)

	// Register a LIQUIDITY type validator
	liquidityValAddr := sdk.ValAddress([]byte("val_liquidity_____")).String()
	sk.AddValidator(sdk.ValAddress([]byte("val_liquidity_____")), math.NewInt(1000000))
	resp, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: liquidityValAddr,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Register a FULL type validator (must exist in staking)
	fullValAddr := sdk.ValAddress([]byte("val_full__________"))
	sk.AddValidator(fullValAddr, math.NewInt(500_000))
	resp, err = msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: fullValAddr.String(),
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_FULL,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify both vaults exist with correct types
	liqVault, found := k.GetVault(ctx, liquidityValAddr)
	require.True(t, found)
	require.Equal(t, types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY, liqVault.ValidatorType)

	fullVault, found := k.GetVault(ctx, fullValAddr.String())
	require.True(t, found)
	require.Equal(t, types.ValidatorType_VALIDATOR_TYPE_FULL, fullVault.ValidatorType)
}

// ---------------------------------------------------------------------------
// MsgSetDelegatorRewardPercent tests
// ---------------------------------------------------------------------------

func TestMsgSetDelegatorRewardPercent_ValidUpdates(t *testing.T) {
	k, msgServer, ctx, _, sk := setupMsgServer(t)

	valAddr := sdk.ValAddress([]byte("validator_reward__")).String()
	sk.AddValidator(sdk.ValAddress([]byte("validator_reward__")), math.NewInt(1000000))

	// Register the validator first
	_, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	})
	require.NoError(t, err)

	tests := []struct {
		name    string
		percent math.LegacyDec
	}{
		{name: "0 percent", percent: math.LegacyNewDec(0)},
		{name: "50 percent", percent: math.LegacyNewDec(50)},
		{name: "100 percent", percent: math.LegacyNewDec(100)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := msgServer.SetDelegatorRewardPercent(ctx, &types.MsgSetDelegatorRewardPercent{
				ValidatorAddress: valAddr,
				Percent:          tc.percent,
			})
			require.NoError(t, err)
			require.NotNil(t, resp)

			// Verify vault was actually updated
			vault, found := k.GetVault(ctx, valAddr)
			require.True(t, found)
			require.True(t, vault.DelegatorRewardPercent.Equal(tc.percent),
				"expected %s, got %s", tc.percent, vault.DelegatorRewardPercent)
		})
	}
}

func TestMsgSetDelegatorRewardPercent_NotRegistered(t *testing.T) {
	_, msgServer, ctx, _, _ := setupMsgServer(t)

	valAddr := sdk.ValAddress([]byte("unregistered______")).String()

	resp, err := msgServer.SetDelegatorRewardPercent(ctx, &types.MsgSetDelegatorRewardPercent{
		ValidatorAddress: valAddr,
		Percent:          math.LegacyNewDec(50),
	})
	require.Error(t, err)
	require.Nil(t, resp)
	require.ErrorIs(t, err, types.ErrValidatorNotRegistered)
}

func TestMsgSetDelegatorRewardPercent_NegativePercent(t *testing.T) {
	_, msgServer, ctx, _, sk := setupMsgServer(t)

	valAddr := sdk.ValAddress([]byte("validator_neg_____")).String()
	sk.AddValidator(sdk.ValAddress([]byte("validator_neg_____")), math.NewInt(1000000))

	// Register first
	_, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	})
	require.NoError(t, err)

	resp, err := msgServer.SetDelegatorRewardPercent(ctx, &types.MsgSetDelegatorRewardPercent{
		ValidatorAddress: valAddr,
		Percent:          math.LegacyNewDec(-1),
	})
	require.Error(t, err)
	require.Nil(t, resp)
	require.ErrorIs(t, err, types.ErrInvalidDelegatorPercent)
}

func TestMsgSetDelegatorRewardPercent_OverHundred(t *testing.T) {
	_, msgServer, ctx, _, sk := setupMsgServer(t)

	valAddr := sdk.ValAddress([]byte("validator_over____")).String()
	sk.AddValidator(sdk.ValAddress([]byte("validator_over____")), math.NewInt(1000000))

	// Register first
	_, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	})
	require.NoError(t, err)

	resp, err := msgServer.SetDelegatorRewardPercent(ctx, &types.MsgSetDelegatorRewardPercent{
		ValidatorAddress: valAddr,
		Percent:          math.LegacyNewDec(101),
	})
	require.Error(t, err)
	require.Nil(t, resp)
	require.ErrorIs(t, err, types.ErrInvalidDelegatorPercent)
}

func TestMsgSetDelegatorRewardPercent_VerifyUpdate(t *testing.T) {
	k, msgServer, ctx, _, sk := setupMsgServer(t)

	valAddr := sdk.ValAddress([]byte("val_verify_update_")).String()
	sk.AddValidator(sdk.ValAddress([]byte("val_verify_update_")), math.NewInt(1000000))

	// Register
	_, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	})
	require.NoError(t, err)

	// Get initial vault to check default
	vaultBefore, found := k.GetVault(ctx, valAddr)
	require.True(t, found)
	require.True(t, vaultBefore.DelegatorRewardPercent.Equal(math.LegacyNewDec(50)),
		"default should be 50%%")

	// Update to 75
	_, err = msgServer.SetDelegatorRewardPercent(ctx, &types.MsgSetDelegatorRewardPercent{
		ValidatorAddress: valAddr,
		Percent:          math.LegacyNewDec(75),
	})
	require.NoError(t, err)

	// Verify change persisted
	vaultAfter, found := k.GetVault(ctx, valAddr)
	require.True(t, found)
	require.True(t, vaultAfter.DelegatorRewardPercent.Equal(math.LegacyNewDec(75)))
	require.False(t, vaultAfter.DelegatorRewardPercent.Equal(vaultBefore.DelegatorRewardPercent))
}

// ---------------------------------------------------------------------------
// MsgDepositToVault tests
// ---------------------------------------------------------------------------

func TestMsgDepositToVault_NotRegistered(t *testing.T) {
	_, msgServer, ctx, _, _ := setupMsgServer(t)

	valAddr := sdk.ValAddress([]byte("unreg_deposit_____")).String()

	resp, err := msgServer.DepositToVault(ctx, &types.MsgDepositToVault{
		ValidatorAddress:    valAddr,
		PoolContractAddress: "cosmos1poolcontractaddr",
		Cw20ContractAddress: "cosmos1cw20contractaddr",
		Amount0:             sdk.NewCoin("ubluechip", math.NewInt(1000)),
		Amount1:             math.NewInt(500),
	})
	require.Error(t, err)
	require.Nil(t, resp)
	require.ErrorIs(t, err, types.ErrValidatorNotRegistered)
}

func TestMsgDepositToVault_ZeroAmount0(t *testing.T) {
	_, msgServer, ctx, _, sk := setupMsgServer(t)

	valAddr := sdk.ValAddress([]byte("val_zero_amount0__")).String()
	sk.AddValidator(sdk.ValAddress([]byte("val_zero_amount0__")), math.NewInt(1000000))

	// Register first
	_, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	})
	require.NoError(t, err)

	resp, err := msgServer.DepositToVault(ctx, &types.MsgDepositToVault{
		ValidatorAddress:    valAddr,
		PoolContractAddress: "cosmos1poolcontractaddr",
		Cw20ContractAddress: "cosmos1cw20contractaddr",
		Amount0:             sdk.NewCoin("ubluechip", math.ZeroInt()),
		Amount1:             math.NewInt(500),
	})
	require.Error(t, err)
	require.Nil(t, resp)
	require.ErrorIs(t, err, types.ErrInvalidDepositAmount)
}

func TestMsgDepositToVault_ZeroAmount1(t *testing.T) {
	_, msgServer, ctx, _, sk := setupMsgServer(t)

	valAddr := sdk.ValAddress([]byte("val_zero_amount1__")).String()
	sk.AddValidator(sdk.ValAddress([]byte("val_zero_amount1__")), math.NewInt(1000000))

	// Register first
	_, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	})
	require.NoError(t, err)

	resp, err := msgServer.DepositToVault(ctx, &types.MsgDepositToVault{
		ValidatorAddress:    valAddr,
		PoolContractAddress: "cosmos1poolcontractaddr",
		Cw20ContractAddress: "cosmos1cw20contractaddr",
		Amount0:             sdk.NewCoin("ubluechip", math.NewInt(1000)),
		Amount1:             math.ZeroInt(),
	})
	require.Error(t, err)
	require.Nil(t, resp)
	require.ErrorIs(t, err, types.ErrInvalidDepositAmount)
}
