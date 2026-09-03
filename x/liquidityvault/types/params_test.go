package types_test

import (
	"testing"
	"time"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"bluechipChain/x/liquidityvault/types"
)

func TestParamsValidate(t *testing.T) {
	tests := []struct {
		desc      string
		params    types.Params
		expErr    bool
		expErrMsg string
	}{
		{
			desc:   "default params are valid",
			params: types.DefaultParams(),
		},
		{
			desc:   "positive stake cap is valid",
			params: types.NewParams(math.NewInt(1_000_000), time.Hour, time.Hour, time.Hour),
		},
		{
			desc:   "zero grace period is valid (immediate withdrawals)",
			params: types.NewParams(math.ZeroInt(), 0, 0, time.Hour),
		},
		{
			desc:      "nil stake cap",
			params:    types.Params{WithdrawalGracePeriod: time.Hour},
			expErr:    true,
			expErrMsg: "stake cap cannot be nil",
		},
		{
			desc:      "negative stake cap",
			params:    types.NewParams(math.NewInt(-1), time.Hour, time.Hour, time.Hour),
			expErr:    true,
			expErrMsg: "stake cap cannot be negative",
		},
		{
			desc:      "negative grace period",
			params:    types.NewParams(math.ZeroInt(), -time.Hour, time.Hour, time.Hour),
			expErr:    true,
			expErrMsg: "withdrawal grace period cannot be negative",
		},
		{
			desc:      "negative deallocation grace period",
			params:    types.NewParams(math.ZeroInt(), time.Hour, -time.Hour, time.Hour),
			expErr:    true,
			expErrMsg: "deallocation grace period cannot be negative",
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			err := tc.params.Validate()
			if tc.expErr {
				require.ErrorContains(t, err, tc.expErrMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
