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

var (
	delegator1 = sdk.AccAddress([]byte("delegator-1---------"))
	delegator2 = sdk.AccAddress([]byte("delegator-2---------"))
)

func balanceOf(bank *keepertest.MockBankKeeper, ctx sdk.Context, addr sdk.AccAddress) math.Int {
	return bank.GetAllBalances(ctx, addr).AmountOf(keepertest.TestBondDenom)
}

func TestCollectPoolRewardsSplit(t *testing.T) {
	k, ctx, stakingKeeper, bankKeeper, wasmKeeper, poolID := setupPool(t)
	_, err := k.AllocateToPool(ctx, valAddr, poolID, coin(600))
	require.NoError(t, err)

	// The default delegator reward share is 0.5: of 1000 collected fees,
	// 500 goes to the operator account immediately, 500 accrues to the
	// vault's delegators (validator tokens: 1000).
	wasmKeeper.AccrueFees(poolContract, math.NewInt(1_000))
	collected, err := k.CollectPoolRewards(ctx, valAddr, poolID)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(1_000), collected)

	require.Equal(t, math.NewInt(500), balanceOf(bankKeeper, ctx, valAccAddr))
	vault, _ := k.GetVault(ctx, valAddr)
	require.Equal(t, math.LegacyNewDec(500), vault.OutstandingRewards)
	// index = 500 / 1000 tokens = 0.5 per token.
	require.Equal(t, math.LegacyNewDecWithPrec(5, 1), vault.RewardIndex)

	// A collector without a position in the pool is rejected.
	stakingKeeper.SetValidatorTokens(val2Addr, math.NewInt(1))
	_, err = k.CollectPoolRewards(ctx, val2Addr, poolID)
	require.ErrorIs(t, err, types.ErrInsufficientShares)

	// Nothing accrued: collect succeeds with zero.
	collected, err = k.CollectPoolRewards(ctx, valAddr, poolID)
	require.NoError(t, err)
	require.True(t, collected.IsZero())
}

func TestValidatorWithoutStakeKeepsWholeCut(t *testing.T) {
	k, ctx, stakingKeeper, bankKeeper, wasmKeeper, poolID := setupPool(t)
	_, err := k.AllocateToPool(ctx, valAddr, poolID, coin(600))
	require.NoError(t, err)

	// A liquidity validator (zero staked tokens) has no delegators to pass
	// rewards to: the whole cut is paid out.
	stakingKeeper.SetValidatorTokens(valAddr, math.ZeroInt())
	wasmKeeper.AccrueFees(poolContract, math.NewInt(900))
	_, err = k.CollectPoolRewards(ctx, valAddr, poolID)
	require.NoError(t, err)

	require.Equal(t, math.NewInt(900), balanceOf(bankKeeper, ctx, valAccAddr))
	vault, _ := k.GetVault(ctx, valAddr)
	require.True(t, vault.OutstandingRewards.IsZero())
}

func TestDelegatorClaimLifecycle(t *testing.T) {
	k, ctx, stakingKeeper, bankKeeper, wasmKeeper, poolID := setupPool(t)
	_, err := k.AllocateToPool(ctx, valAddr, poolID, coin(600))
	require.NoError(t, err)
	hooks := k.StakingHooks()

	// Two delegators: 600 and 400 of the validator's 1000 tokens.
	stakingKeeper.SetDelegationTokens(delegator1, valAddr, math.NewInt(600))
	stakingKeeper.SetDelegationTokens(delegator2, valAddr, math.NewInt(400))

	// Collect 1000: 500 to the validator, 500 to delegators (0.5/token).
	wasmKeeper.AccrueFees(poolContract, math.NewInt(1_000))
	_, err = k.CollectPoolRewards(ctx, valAddr, poolID)
	require.NoError(t, err)

	// Claimable projections: 600 * 0.5 = 300 and 400 * 0.5 = 200.
	require.Equal(t, math.NewInt(300), k.ClaimableReward(ctx, delegator1, valAddr))
	require.Equal(t, math.NewInt(200), k.ClaimableReward(ctx, delegator2, valAddr))

	// Delegator 1 claims.
	paid, err := k.ClaimVaultRewards(ctx, delegator1, valAddr)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(300), paid.Amount)
	require.Equal(t, math.NewInt(300), balanceOf(bankKeeper, ctx, delegator1))
	require.True(t, k.ClaimableReward(ctx, delegator1, valAddr).IsZero())

	// A second claim pays nothing.
	paid, err = k.ClaimVaultRewards(ctx, delegator1, valAddr)
	require.NoError(t, err)
	require.True(t, paid.Amount.IsZero())

	// Delegator 2 unbonds fully: the removal hook settles their accrual,
	// which stays claimable afterwards.
	require.NoError(t, hooks.BeforeDelegationRemoved(ctx, delegator2, valAddr))
	stakingKeeper.RemoveDelegation(delegator2, valAddr)
	require.Equal(t, math.NewInt(200), k.ClaimableReward(ctx, delegator2, valAddr))
	paid, err = k.ClaimVaultRewards(ctx, delegator2, valAddr)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(200), paid.Amount)

	vault, _ := k.GetVault(ctx, valAddr)
	require.True(t, vault.OutstandingRewards.IsZero())
}

func TestSettlementOnStakeChange(t *testing.T) {
	k, ctx, stakingKeeper, _, wasmKeeper, poolID := setupPool(t)
	_, err := k.AllocateToPool(ctx, valAddr, poolID, coin(600))
	require.NoError(t, err)
	hooks := k.StakingHooks()

	// Window 1: delegator holds 200 of 1000 tokens; 500 distributed at
	// 0.5/token -> 100 accrues to them.
	stakingKeeper.SetDelegationTokens(delegator1, valAddr, math.NewInt(200))
	wasmKeeper.AccrueFees(poolContract, math.NewInt(1_000))
	_, err = k.CollectPoolRewards(ctx, valAddr, poolID)
	require.NoError(t, err)

	// The delegator doubles their stake: the hook settles window 1 BEFORE
	// the change (100 at 200 tokens), then the stake becomes 400 (validator
	// 1200).
	require.NoError(t, hooks.BeforeDelegationSharesModified(ctx, delegator1, valAddr))
	stakingKeeper.SetDelegationTokens(delegator1, valAddr, math.NewInt(400))
	stakingKeeper.SetValidatorTokens(valAddr, math.NewInt(1_200))
	require.NoError(t, hooks.AfterDelegationModified(ctx, delegator1, valAddr))

	// Window 2: another 1200 collected -> 600 to delegators at 0.5/token;
	// the delegator's 400 tokens accrue 200.
	wasmKeeper.AccrueFees(poolContract, math.NewInt(1_200))
	_, err = k.CollectPoolRewards(ctx, valAddr, poolID)
	require.NoError(t, err)

	// Total claimable: 100 (window 1) + 200 (window 2) = 300 — NOT
	// 400 * (0.5 + 0.5) = 400, which a naive index-times-current-stake
	// would pay.
	require.Equal(t, math.NewInt(300), k.ClaimableReward(ctx, delegator1, valAddr))
}

func TestNewDelegationDoesNotEarnPastRewards(t *testing.T) {
	k, ctx, stakingKeeper, _, wasmKeeper, poolID := setupPool(t)
	_, err := k.AllocateToPool(ctx, valAddr, poolID, coin(600))
	require.NoError(t, err)
	hooks := k.StakingHooks()

	// Rewards are collected while delegator1 has no delegation.
	wasmKeeper.AccrueFees(poolContract, math.NewInt(1_000))
	_, err = k.CollectPoolRewards(ctx, valAddr, poolID)
	require.NoError(t, err)

	// They then delegate: the creation hook pins their window to the
	// current index, so the earlier distribution is not theirs.
	require.NoError(t, hooks.BeforeDelegationCreated(ctx, delegator1, valAddr))
	stakingKeeper.SetDelegationTokens(delegator1, valAddr, math.NewInt(500))
	stakingKeeper.SetValidatorTokens(valAddr, math.NewInt(1_500))
	require.NoError(t, hooks.AfterDelegationModified(ctx, delegator1, valAddr))

	require.True(t, k.ClaimableReward(ctx, delegator1, valAddr).IsZero())
}

func TestRewardMsgServer(t *testing.T) {
	k, ctx, stakingKeeper, _, wasmKeeper, poolID := setupPool(t)
	ms := keeper.NewMsgServerImpl(k)
	_, err := k.AllocateToPool(ctx, valAddr, poolID, coin(600))
	require.NoError(t, err)

	stakingKeeper.SetDelegationTokens(delegator1, valAddr, math.NewInt(1_000))
	wasmKeeper.AccrueFees(poolContract, math.NewInt(800))

	collectResp, err := ms.CollectPoolRewards(ctx, &types.MsgCollectPoolRewards{
		ValidatorAddress: valAddr.String(),
		PoolId:           poolID,
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(800), collectResp.Collected)

	claimResp, err := ms.ClaimVaultRewards(ctx, &types.MsgClaimVaultRewards{
		DelegatorAddress: delegator1.String(),
		ValidatorAddress: valAddr.String(),
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(400), claimResp.Amount.Amount)

	queryResp, err := k.DelegatorReward(ctx, &types.QueryDelegatorRewardRequest{
		DelegatorAddress: delegator1.String(),
		ValidatorAddress: valAddr.String(),
	})
	require.NoError(t, err)
	require.True(t, queryResp.Claimable.IsZero())
}

func TestModuleAccountInvariantCoversOutstandingRewards(t *testing.T) {
	k, ctx, stakingKeeper, bankKeeper, wasmKeeper, poolID := setupPool(t)
	_, err := k.AllocateToPool(ctx, valAddr, poolID, coin(600))
	require.NoError(t, err)
	stakingKeeper.SetDelegationTokens(delegator1, valAddr, math.NewInt(1_000))
	wasmKeeper.AccrueFees(poolContract, math.NewInt(1_000))
	_, err = k.CollectPoolRewards(ctx, valAddr, poolID)
	require.NoError(t, err)

	invariant := keeper.ModuleAccountInvariant(k)
	_, broken := invariant(ctx)
	require.False(t, broken)

	// If the module account can't cover vault balances + outstanding
	// delegator rewards, the invariant breaks.
	held := balanceOf(bankKeeper, ctx, moduleAddr)
	bankKeeper.SetBalance(moduleAddr, sdk.NewCoins(sdk.NewCoin(keepertest.TestBondDenom, held.SubRaw(1))))
	_, broken = invariant(ctx)
	require.True(t, broken)
}

func TestTinyCutGoesToValidatorInsteadOfLocking(t *testing.T) {
	k, ctx, stakingKeeper, bankKeeper, wasmKeeper, poolID := setupPool(t)
	_, err := k.AllocateToPool(ctx, valAddr, poolID, coin(600))
	require.NoError(t, err)

	// With an enormous validator stake, a 1-token delegator cut cannot move
	// the 18-decimal reward index. Rather than locking the token as
	// forever-unclaimable outstanding rewards, the whole cut goes to the
	// validator.
	stakingKeeper.SetValidatorTokens(valAddr, math.NewInt(1).MulRaw(1_000_000_000_000_000_000).MulRaw(1_000_000))
	wasmKeeper.AccrueFees(poolContract, math.NewInt(2))
	_, err = k.CollectPoolRewards(ctx, valAddr, poolID)
	require.NoError(t, err)

	require.Equal(t, math.NewInt(2), balanceOf(bankKeeper, ctx, valAccAddr))
	vault, _ := k.GetVault(ctx, valAddr)
	require.True(t, vault.OutstandingRewards.IsZero())
	require.True(t, vault.RewardIndex.IsZero())
}

func TestFractionalDustRetainedAcrossClaims(t *testing.T) {
	k, ctx, stakingKeeper, _, wasmKeeper, poolID := setupPool(t)
	_, err := k.AllocateToPool(ctx, valAddr, poolID, coin(600))
	require.NoError(t, err)

	// Validator tokens 1000; delegator holds 333. Collecting 1000 puts 500
	// to delegators at 0.5/token -> the delegator accrues 166.5: a claim
	// pays the floor (166) and the 0.5 fraction stays recorded.
	stakingKeeper.SetDelegationTokens(delegator1, valAddr, math.NewInt(333))
	wasmKeeper.AccrueFees(poolContract, math.NewInt(1_000))
	_, err = k.CollectPoolRewards(ctx, valAddr, poolID)
	require.NoError(t, err)

	paid, err := k.ClaimVaultRewards(ctx, delegator1, valAddr)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(166), paid.Amount)

	reward, found := k.GetDelegatorReward(ctx, delegator1, valAddr)
	require.True(t, found)
	require.Equal(t, math.LegacyNewDecWithPrec(5, 1), reward.Accrued)

	// A second identical collection tops the fraction up to 167.
	wasmKeeper.AccrueFees(poolContract, math.NewInt(1_000))
	_, err = k.CollectPoolRewards(ctx, valAddr, poolID)
	require.NoError(t, err)
	paid, err = k.ClaimVaultRewards(ctx, delegator1, valAddr)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(167), paid.Amount)
}

func TestMultiValidatorPoolRewardSplit(t *testing.T) {
	k, ctx, stakingKeeper, bankKeeper, wasmKeeper, poolID := setupPool(t)

	// val1: 600 shares; val2: 300 shares (2:1).
	_, err := k.AllocateToPool(ctx, valAddr, poolID, coin(600))
	require.NoError(t, err)
	stakingKeeper.SetValidatorTokens(val2Addr, math.NewInt(500))
	bankKeeper.SetBalance(val2AccAddr, sdk.NewCoins(coin(300)))
	require.NoError(t, k.Deposit(ctx, val2Addr, coin(300)))
	_, err = k.AllocateToPool(ctx, val2Addr, poolID, coin(300))
	require.NoError(t, err)

	// val2 keeps everything for its delegators (share 1.0).
	require.NoError(t, k.SetRewardShare(ctx, val2Addr, math.LegacyOneDec()))

	// 900 collected: val1's cut 600 (300 to operator, 300 to delegators),
	// val2's cut 300 (all to delegators).
	wasmKeeper.AccrueFees(poolContract, math.NewInt(900))
	_, err = k.CollectPoolRewards(ctx, valAddr, poolID)
	require.NoError(t, err)

	require.Equal(t, math.NewInt(300), balanceOf(bankKeeper, ctx, valAccAddr))
	require.True(t, balanceOf(bankKeeper, ctx, val2AccAddr).IsZero())

	vault1, _ := k.GetVault(ctx, valAddr)
	require.Equal(t, math.LegacyNewDec(300), vault1.OutstandingRewards)
	vault2, _ := k.GetVault(ctx, val2Addr)
	require.Equal(t, math.LegacyNewDec(300), vault2.OutstandingRewards)
}
