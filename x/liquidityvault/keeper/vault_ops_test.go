package keeper_test

import (
	"testing"
	"time"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/stretchr/testify/require"

	keepertest "bluechipChain/testutil/keeper"
	"bluechipChain/x/liquidityvault/types"
)

var (
	valAddr    = sdk.ValAddress([]byte("validator-operator-1"))
	valAccAddr = sdk.AccAddress(valAddr)
	moduleAddr = authtypes.NewModuleAddress(types.ModuleName)
)

func coin(amount int64) sdk.Coin {
	return sdk.NewInt64Coin(keepertest.TestBondDenom, amount)
}

func TestDeposit(t *testing.T) {
	k, ctx, stakingKeeper, bankKeeper, _ := keepertest.LiquidityvaultKeeper(t)
	stakingKeeper.SetValidatorTokens(valAddr, math.NewInt(1_000_000))
	bankKeeper.SetBalance(valAccAddr, sdk.NewCoins(coin(500)))

	require.NoError(t, k.Deposit(ctx, valAddr, coin(300)))

	vault, found := k.GetVault(ctx, valAddr)
	require.True(t, found)
	require.Equal(t, math.NewInt(300), vault.Balance)
	require.Equal(t, types.DefaultDelegatorRewardShare, vault.DelegatorRewardShare)
	require.Equal(t, sdk.NewCoins(coin(300)), bankKeeper.GetAllBalances(ctx, moduleAddr))
	require.Equal(t, sdk.NewCoins(coin(200)), bankKeeper.GetAllBalances(ctx, valAccAddr))

	// A second deposit accumulates.
	require.NoError(t, k.Deposit(ctx, valAddr, coin(100)))
	vault, _ = k.GetVault(ctx, valAddr)
	require.Equal(t, math.NewInt(400), vault.Balance)
}

func TestDepositRejections(t *testing.T) {
	k, ctx, stakingKeeper, bankKeeper, _ := keepertest.LiquidityvaultKeeper(t)

	// Unknown validator.
	err := k.Deposit(ctx, valAddr, coin(100))
	require.ErrorIs(t, err, types.ErrValidatorNotFound)

	stakingKeeper.SetValidatorTokens(valAddr, math.NewInt(1))
	bankKeeper.SetBalance(valAccAddr, sdk.NewCoins(coin(100), sdk.NewInt64Coin("other", 100)))

	// Wrong denom.
	err = k.Deposit(ctx, valAddr, sdk.NewInt64Coin("other", 50))
	require.ErrorIs(t, err, types.ErrInvalidDeposit)

	// Insufficient funds.
	err = k.Deposit(ctx, valAddr, coin(200))
	require.Error(t, err)
	_, found := k.GetVault(ctx, valAddr)
	require.False(t, found)
}

func TestInitiateWithdrawal(t *testing.T) {
	k, ctx, stakingKeeper, bankKeeper, _ := keepertest.LiquidityvaultKeeper(t)
	stakingKeeper.SetValidatorTokens(valAddr, math.NewInt(1))
	bankKeeper.SetBalance(valAccAddr, sdk.NewCoins(coin(500)))
	require.NoError(t, k.Deposit(ctx, valAddr, coin(500)))

	now := time.Now().UTC()
	ctx = ctx.WithBlockTime(now)

	withdrawal, err := k.InitiateWithdrawal(ctx, valAddr, coin(200))
	require.NoError(t, err)
	require.Equal(t, math.NewInt(200), withdrawal.Amount)
	require.Equal(t, now.Add(types.DefaultWithdrawalGracePeriod), withdrawal.CompleteTime)

	// The active balance drops immediately.
	vault, _ := k.GetVault(ctx, valAddr)
	require.Equal(t, math.NewInt(300), vault.Balance)

	// The funds stay in the module account until the grace period ends.
	require.Equal(t, sdk.NewCoins(coin(500)), bankKeeper.GetAllBalances(ctx, moduleAddr))

	pending := k.GetPendingWithdrawals(ctx, valAddr)
	require.Len(t, pending, 1)
	require.Equal(t, math.NewInt(200), pending[0].Amount)

	// A second withdrawal in the same block merges into the same queue entry.
	_, err = k.InitiateWithdrawal(ctx, valAddr, coin(100))
	require.NoError(t, err)
	pending = k.GetPendingWithdrawals(ctx, valAddr)
	require.Len(t, pending, 1)
	require.Equal(t, math.NewInt(300), pending[0].Amount)

	// Over-withdrawal is rejected.
	_, err = k.InitiateWithdrawal(ctx, valAddr, coin(300))
	require.ErrorIs(t, err, types.ErrInsufficientVaultBalance)

	// Wrong denom is rejected.
	_, err = k.InitiateWithdrawal(ctx, valAddr, sdk.NewInt64Coin("other", 1))
	require.ErrorIs(t, err, types.ErrInvalidWithdrawal)
}

func TestProcessMaturedWithdrawals(t *testing.T) {
	k, ctx, stakingKeeper, bankKeeper, _ := keepertest.LiquidityvaultKeeper(t)
	stakingKeeper.SetValidatorTokens(valAddr, math.NewInt(1))
	bankKeeper.SetBalance(valAccAddr, sdk.NewCoins(coin(500)))
	require.NoError(t, k.Deposit(ctx, valAddr, coin(500)))

	start := time.Now().UTC()
	ctx = ctx.WithBlockTime(start)
	_, err := k.InitiateWithdrawal(ctx, valAddr, coin(200))
	require.NoError(t, err)

	// Before the grace period ends nothing is released.
	almostDone := start.Add(types.DefaultWithdrawalGracePeriod - time.Second)
	require.NoError(t, k.ProcessMaturedWithdrawals(ctx.WithBlockTime(almostDone)))
	require.Len(t, k.GetPendingWithdrawals(ctx, valAddr), 1)
	require.Equal(t, sdk.NewCoins(coin(0)), bankKeeper.GetAllBalances(ctx, valAccAddr))

	// After the grace period the funds land in the validator's account.
	done := start.Add(types.DefaultWithdrawalGracePeriod)
	require.NoError(t, k.ProcessMaturedWithdrawals(ctx.WithBlockTime(done)))
	require.Empty(t, k.GetPendingWithdrawals(ctx, valAddr))
	require.Equal(t, sdk.NewCoins(coin(200)), bankKeeper.GetAllBalances(ctx, valAccAddr))
	require.Equal(t, sdk.NewCoins(coin(300)), bankKeeper.GetAllBalances(ctx, moduleAddr))
}

func TestSetRewardShare(t *testing.T) {
	k, ctx, stakingKeeper, _, _ := keepertest.LiquidityvaultKeeper(t)
	stakingKeeper.SetValidatorTokens(valAddr, math.NewInt(1))

	// Can be set before the first deposit; creates the vault.
	share := math.LegacyNewDecWithPrec(7, 1) // 0.7
	require.NoError(t, k.SetRewardShare(ctx, valAddr, share))
	vault, found := k.GetVault(ctx, valAddr)
	require.True(t, found)
	require.Equal(t, share, vault.DelegatorRewardShare)
	require.True(t, vault.Balance.IsZero())

	// Bounds are enforced.
	require.ErrorIs(t, k.SetRewardShare(ctx, valAddr, math.LegacyNewDec(2)), types.ErrInvalidRewardShare)
	require.ErrorIs(t, k.SetRewardShare(ctx, valAddr, math.LegacyNewDec(-1)), types.ErrInvalidRewardShare)
	require.ErrorIs(t, k.SetRewardShare(ctx, valAddr, math.LegacyDec{}), types.ErrInvalidRewardShare)

	// Unknown validator is rejected.
	other := sdk.ValAddress([]byte("validator-operator-2"))
	require.ErrorIs(t, k.SetRewardShare(ctx, other, share), types.ErrValidatorNotFound)
}

func TestCompositeScore(t *testing.T) {
	k, ctx, stakingKeeper, bankKeeper, _ := keepertest.LiquidityvaultKeeper(t)
	stakingKeeper.SetValidatorTokens(valAddr, math.NewInt(1_000))
	bankKeeper.SetBalance(valAccAddr, sdk.NewCoins(coin(600)))

	// Without a vault the score is just the staked tokens.
	staked, vaultBalance, _, score, err := k.GetCompositeScore(ctx, valAddr)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(1_000), staked)
	require.True(t, vaultBalance.IsZero())
	require.Equal(t, math.NewInt(1_000), score)

	// Deposits raise the score.
	require.NoError(t, k.Deposit(ctx, valAddr, coin(600)))
	_, vaultBalance, _, score, err = k.GetCompositeScore(ctx, valAddr)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(600), vaultBalance)
	require.Equal(t, math.NewInt(1_600), score)

	// Pending withdrawals stop counting immediately.
	_, err = k.InitiateWithdrawal(ctx.WithBlockTime(time.Now().UTC()), valAddr, coin(100))
	require.NoError(t, err)
	_, vaultBalance, _, score, err = k.GetCompositeScore(ctx, valAddr)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(500), vaultBalance)
	require.Equal(t, math.NewInt(1_500), score)

	// Unknown validator errors.
	_, _, _, _, err = k.GetCompositeScore(ctx, sdk.ValAddress([]byte("validator-operator-2")))
	require.ErrorIs(t, err, types.ErrValidatorNotFound)
}
