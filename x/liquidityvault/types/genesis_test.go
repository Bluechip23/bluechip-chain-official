package types_test

import (
	"testing"
	"time"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"bluechipChain/x/liquidityvault/types"
)

func TestGenesisState_Validate(t *testing.T) {
	valAddr := sdk.ValAddress([]byte("validator-operator-1")).String()
	validVault := types.Vault{
		ValidatorAddress:     valAddr,
		Balance:              math.NewInt(100),
		DelegatorRewardShare: math.LegacyNewDecWithPrec(5, 1),
		RewardIndex:          math.LegacyZeroDec(),
		OutstandingRewards:   math.LegacyZeroDec(),
	}

	tests := []struct {
		desc     string
		genState *types.GenesisState
		valid    bool
	}{
		{
			desc:     "default is valid",
			genState: types.DefaultGenesis(),
			valid:    true,
		},
		{
			desc: "valid genesis state with vault and pending withdrawal",
			genState: &types.GenesisState{
				Params: types.DefaultParams(),
				Vaults: []types.Vault{validVault},
				PendingWithdrawals: []types.PendingWithdrawal{
					{
						ValidatorAddress: valAddr,
						Amount:           math.NewInt(50),
						CompleteTime:     time.Now().UTC(),
					},
				},
			},
			valid: true,
		},
		{
			desc:     "empty params are invalid",
			genState: &types.GenesisState{},
			valid:    false,
		},
		{
			desc: "duplicate vaults are invalid",
			genState: &types.GenesisState{
				Params: types.DefaultParams(),
				Vaults: []types.Vault{validVault, validVault},
			},
			valid: false,
		},
		{
			desc: "negative vault balance is invalid",
			genState: &types.GenesisState{
				Params: types.DefaultParams(),
				Vaults: []types.Vault{
					{
						ValidatorAddress:     valAddr,
						Balance:              math.NewInt(-1),
						DelegatorRewardShare: math.LegacyNewDecWithPrec(5, 1),
						RewardIndex:          math.LegacyZeroDec(),
						OutstandingRewards:   math.LegacyZeroDec(),
					},
				},
			},
			valid: false,
		},
		{
			desc: "reward share above one is invalid",
			genState: &types.GenesisState{
				Params: types.DefaultParams(),
				Vaults: []types.Vault{
					{
						ValidatorAddress:     valAddr,
						Balance:              math.NewInt(1),
						DelegatorRewardShare: math.LegacyNewDec(2),
						RewardIndex:          math.LegacyZeroDec(),
						OutstandingRewards:   math.LegacyZeroDec(),
					},
				},
			},
			valid: false,
		},
		{
			desc: "non-positive pending withdrawal is invalid",
			genState: &types.GenesisState{
				Params: types.DefaultParams(),
				PendingWithdrawals: []types.PendingWithdrawal{
					{
						ValidatorAddress: valAddr,
						Amount:           math.ZeroInt(),
						CompleteTime:     time.Now().UTC(),
					},
				},
			},
			valid: false,
		},
		{
			desc: "pending withdrawal without complete time is invalid",
			genState: &types.GenesisState{
				Params: types.DefaultParams(),
				PendingWithdrawals: []types.PendingWithdrawal{
					{
						ValidatorAddress: valAddr,
						Amount:           math.NewInt(1),
					},
				},
			},
			valid: false,
		},
		{
			desc: "value post history without a vault is invalid",
			genState: &types.GenesisState{
				Params: types.DefaultParams(),
				ValuePostHistories: []types.ValuePostHistory{
					{
						ValidatorAddress: valAddr,
						Posts:            []types.ValuePost{{Value: math.NewInt(1), PostTime: time.Now().UTC()}},
					},
				},
			},
			valid: false,
		},
		{
			desc: "scheduled value post without a vault is invalid",
			genState: &types.GenesisState{
				Params: types.DefaultParams(),
				ScheduledValuePosts: []types.ScheduledValuePost{
					{ValidatorAddress: valAddr, PostTime: time.Now().UTC()},
				},
			},
			valid: false,
		},
		{
			desc: "value posts tied to a vault are valid",
			genState: &types.GenesisState{
				Params: types.DefaultParams(),
				Vaults: []types.Vault{validVault},
				ValuePostHistories: []types.ValuePostHistory{
					{
						ValidatorAddress: valAddr,
						Posts:            []types.ValuePost{{Value: math.NewInt(1), PostTime: time.Now().UTC()}},
					},
				},
				ScheduledValuePosts: []types.ScheduledValuePost{
					{ValidatorAddress: valAddr, PostTime: time.Now().UTC()},
				},
			},
			valid: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			err := tc.genState.Validate()
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
