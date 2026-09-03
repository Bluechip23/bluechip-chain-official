package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	keepertest "bluechipChain/testutil/keeper"
	"bluechipChain/x/liquidityvault/keeper"
	"bluechipChain/x/liquidityvault/types"
)

func TestMsgUpdateParams(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)
	ms := keeper.NewMsgServerImpl(k)
	params := types.DefaultParams()

	testCases := []struct {
		name      string
		input     *types.MsgUpdateParams
		expErr    bool
		expErrMsg string
	}{
		{
			name: "invalid authority",
			input: &types.MsgUpdateParams{
				Authority: "invalid",
				Params:    params,
			},
			expErr:    true,
			expErrMsg: "invalid authority",
		},
		{
			name: "invalid params rejected",
			input: &types.MsgUpdateParams{
				Authority: k.GetAuthority(),
				Params:    types.Params{},
			},
			expErr:    true,
			expErrMsg: "stake cap cannot be nil",
		},
		{
			name: "all good",
			input: &types.MsgUpdateParams{
				Authority: k.GetAuthority(),
				Params:    types.NewParams(math.NewInt(1_000_000), types.DefaultWithdrawalGracePeriod, types.DefaultDeallocationGracePeriod, types.DefaultValuePostInterval),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ms.UpdateParams(ctx, tc.input)
			if tc.expErr {
				require.ErrorContains(t, err, tc.expErrMsg)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.input.Params, k.GetParams(ctx))
			}
		})
	}
}

func TestMsgDepositAndWithdraw(t *testing.T) {
	k, ctx, stakingKeeper, bankKeeper, _ := keepertest.LiquidityvaultKeeper(t)
	ms := keeper.NewMsgServerImpl(k)

	stakingKeeper.SetValidatorTokens(valAddr, math.NewInt(1))
	bankKeeper.SetBalance(valAccAddr, sdk.NewCoins(coin(500)))

	_, err := ms.Deposit(ctx, &types.MsgDeposit{
		ValidatorAddress: valAddr.String(),
		Amount:           coin(400),
	})
	require.NoError(t, err)

	resp, err := ms.InitiateWithdrawal(ctx, &types.MsgInitiateWithdrawal{
		ValidatorAddress: valAddr.String(),
		Amount:           coin(150),
	})
	require.NoError(t, err)
	require.Equal(t, ctx.BlockTime().Add(types.DefaultWithdrawalGracePeriod).UTC(), resp.CompleteTime.UTC())

	_, err = ms.SetRewardShare(ctx, &types.MsgSetRewardShare{
		ValidatorAddress:     valAddr.String(),
		DelegatorRewardShare: math.LegacyNewDecWithPrec(25, 2),
	})
	require.NoError(t, err)

	vault, found := k.GetVault(ctx, valAddr)
	require.True(t, found)
	require.Equal(t, math.NewInt(250), vault.Balance)
	require.Equal(t, math.LegacyNewDecWithPrec(25, 2), vault.DelegatorRewardShare)

	// Zero amounts fail ValidateBasic.
	_, err = ms.Deposit(ctx, &types.MsgDeposit{ValidatorAddress: valAddr.String(), Amount: coin(0)})
	require.Error(t, err)
	_, err = ms.InitiateWithdrawal(ctx, &types.MsgInitiateWithdrawal{ValidatorAddress: valAddr.String(), Amount: coin(0)})
	require.Error(t, err)

	// Malformed addresses fail ValidateBasic.
	_, err = ms.Deposit(ctx, &types.MsgDeposit{ValidatorAddress: "not-an-address", Amount: coin(1)})
	require.Error(t, err)
}

func TestQueryServer(t *testing.T) {
	k, ctx, stakingKeeper, bankKeeper, _ := keepertest.LiquidityvaultKeeper(t)

	stakingKeeper.SetValidatorTokens(valAddr, math.NewInt(1_000))
	bankKeeper.SetBalance(valAccAddr, sdk.NewCoins(coin(500)))
	require.NoError(t, k.Deposit(ctx, valAddr, coin(500)))

	paramsResp, err := k.Params(ctx, &types.QueryParamsRequest{})
	require.NoError(t, err)
	require.Equal(t, types.DefaultParams(), paramsResp.Params)

	vaultResp, err := k.Vault(ctx, &types.QueryVaultRequest{ValidatorAddress: valAddr.String()})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(500), vaultResp.Vault.Balance)

	vaultsResp, err := k.Vaults(ctx, &types.QueryVaultsRequest{})
	require.NoError(t, err)
	require.Len(t, vaultsResp.Vaults, 1)

	scoreResp, err := k.CompositeScore(ctx, &types.QueryCompositeScoreRequest{ValidatorAddress: valAddr.String()})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(1_500), scoreResp.CompositeScore)

	// Unknown vault is a NotFound, not a panic.
	other := sdk.ValAddress([]byte("validator-operator-2"))
	_, err = k.Vault(ctx, &types.QueryVaultRequest{ValidatorAddress: other.String()})
	require.Error(t, err)
	_, err = k.CompositeScore(ctx, &types.QueryCompositeScoreRequest{ValidatorAddress: other.String()})
	require.Error(t, err)
}

func TestModuleAccountInvariant(t *testing.T) {
	k, ctx, stakingKeeper, bankKeeper, _ := keepertest.LiquidityvaultKeeper(t)
	stakingKeeper.SetValidatorTokens(valAddr, math.NewInt(1))
	bankKeeper.SetBalance(valAccAddr, sdk.NewCoins(coin(500)))
	require.NoError(t, k.Deposit(ctx, valAddr, coin(500)))

	invariant := keeper.ModuleAccountInvariant(k)
	_, broken := invariant(ctx)
	require.False(t, broken)

	// A deficit in the module account breaks the invariant.
	bankKeeper.SetBalance(moduleAddr, sdk.NewCoins(coin(499)))
	_, broken = invariant(ctx)
	require.True(t, broken)

	// A surplus (stray send) does not.
	bankKeeper.SetBalance(moduleAddr, sdk.NewCoins(coin(501)))
	_, broken = invariant(ctx)
	require.False(t, broken)
}
