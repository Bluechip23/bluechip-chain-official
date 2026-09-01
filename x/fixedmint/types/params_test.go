package types_test

import (
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"bluechipChain/x/fixedmint/types"
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
			desc:   "zero mint amount is valid (minting no-op)",
			params: types.NewParams(types.DefaultMintDenom, math.ZeroInt(), true),
		},
		{
			desc:      "empty mint denom",
			params:    types.NewParams("", types.DefaultMintAmount, true),
			expErr:    true,
			expErrMsg: "mint denom cannot be empty",
		},
		{
			desc:      "malformed mint denom",
			params:    types.NewParams("!!invalid!!", types.DefaultMintAmount, true),
			expErr:    true,
			expErrMsg: "invalid mint denom",
		},
		{
			desc:      "nil mint amount",
			params:    types.NewParams(types.DefaultMintDenom, math.Int{}, true),
			expErr:    true,
			expErrMsg: "mint amount cannot be nil",
		},
		{
			desc:      "negative mint amount",
			params:    types.NewParams(types.DefaultMintDenom, math.NewInt(-1), true),
			expErr:    true,
			expErrMsg: "mint amount cannot be negative",
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
