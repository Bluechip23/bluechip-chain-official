package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	keepertest "bluechipChain/testutil/keeper"
	"bluechipChain/x/liquidityvault/types"
)

var delAddr = sdk.AccAddress([]byte("delegator------------"))

// delegate simulates the staking keeper's hook sequence for a delegation that
// moves the validator's tokens from before to after.
func TestStakeCapHooks(t *testing.T) {
	tests := []struct {
		desc         string
		stakeCap     int64
		tokensBefore int64
		tokensAfter  int64
		expErr       bool
	}{
		{
			desc:         "cap disabled allows any delegation",
			stakeCap:     0,
			tokensBefore: 0,
			tokensAfter:  1_000_000,
		},
		{
			desc:         "increase below cap allowed",
			stakeCap:     1_000,
			tokensBefore: 400,
			tokensAfter:  900,
		},
		{
			desc:         "increase exactly to cap allowed",
			stakeCap:     1_000,
			tokensBefore: 400,
			tokensAfter:  1_000,
		},
		{
			desc:         "increase beyond cap rejected",
			stakeCap:     1_000,
			tokensBefore: 400,
			tokensAfter:  1_001,
			expErr:       true,
		},
		{
			desc:         "new validator beyond cap rejected",
			stakeCap:     1_000,
			tokensBefore: 0,
			tokensAfter:  2_000,
			expErr:       true,
		},
		{
			desc:         "decrease always allowed even above a lowered cap",
			stakeCap:     1_000,
			tokensBefore: 5_000,
			tokensAfter:  4_000,
		},
		{
			desc:         "unchanged above cap allowed (not an increase)",
			stakeCap:     1_000,
			tokensBefore: 5_000,
			tokensAfter:  5_000,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			k, ctx, stakingKeeper, _, _ := keepertest.LiquidityvaultKeeper(t)
			require.NoError(t, k.SetParams(ctx, types.NewParams(math.NewInt(tc.stakeCap), types.DefaultWithdrawalGracePeriod, types.DefaultDeallocationGracePeriod)))
			hooks := k.StakingHooks()

			if tc.tokensBefore > 0 {
				stakingKeeper.SetValidatorTokens(valAddr, math.NewInt(tc.tokensBefore))
				require.NoError(t, hooks.BeforeDelegationSharesModified(ctx, delAddr, valAddr))
			} else {
				// New validator: it does not exist when the Before hook runs.
				require.NoError(t, hooks.BeforeDelegationCreated(ctx, delAddr, valAddr))
			}

			stakingKeeper.SetValidatorTokens(valAddr, math.NewInt(tc.tokensAfter))
			err := hooks.AfterDelegationModified(ctx, delAddr, valAddr)
			if tc.expErr {
				require.ErrorIs(t, err, types.ErrStakeCapExceeded)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestStakeCapWithoutSnapshotIsNotEnforced(t *testing.T) {
	// Paths that skip the Before hooks (e.g. genesis import) must not be
	// blocked by the cap.
	k, ctx, stakingKeeper, _, _ := keepertest.LiquidityvaultKeeper(t)
	require.NoError(t, k.SetParams(ctx, types.NewParams(math.NewInt(100), types.DefaultWithdrawalGracePeriod, types.DefaultDeallocationGracePeriod)))

	stakingKeeper.SetValidatorTokens(valAddr, math.NewInt(1_000_000))
	require.NoError(t, k.StakingHooks().AfterDelegationModified(ctx, delAddr, valAddr))
}

func TestStakeCapSnapshotIsConsumed(t *testing.T) {
	// The snapshot from one operation must not leak into the next: after a
	// rejected delegation, a later After hook without a fresh Before must
	// pass (no snapshot -> no enforcement).
	k, ctx, stakingKeeper, _, _ := keepertest.LiquidityvaultKeeper(t)
	require.NoError(t, k.SetParams(ctx, types.NewParams(math.NewInt(100), types.DefaultWithdrawalGracePeriod, types.DefaultDeallocationGracePeriod)))
	hooks := k.StakingHooks()

	stakingKeeper.SetValidatorTokens(valAddr, math.NewInt(50))
	require.NoError(t, hooks.BeforeDelegationSharesModified(ctx, delAddr, valAddr))
	stakingKeeper.SetValidatorTokens(valAddr, math.NewInt(200))
	require.ErrorIs(t, hooks.AfterDelegationModified(ctx, delAddr, valAddr), types.ErrStakeCapExceeded)

	// Snapshot was consumed by the failed check.
	require.NoError(t, hooks.AfterDelegationModified(ctx, delAddr, valAddr))
}
