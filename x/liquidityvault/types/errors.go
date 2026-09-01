package types

// DONTCOVER

import (
	sdkerrors "cosmossdk.io/errors"
)

// x/liquidityvault module sentinel errors
var (
	ErrInvalidSigner            = sdkerrors.Register(ModuleName, 1100, "expected gov account as only signer for proposal message")
	ErrValidatorNotFound        = sdkerrors.Register(ModuleName, 1101, "validator does not exist")
	ErrVaultNotFound            = sdkerrors.Register(ModuleName, 1102, "validator has no liquidity vault")
	ErrInvalidDeposit           = sdkerrors.Register(ModuleName, 1103, "invalid vault deposit")
	ErrInsufficientVaultBalance = sdkerrors.Register(ModuleName, 1104, "insufficient vault balance")
	ErrInvalidRewardShare       = sdkerrors.Register(ModuleName, 1105, "delegator reward share must be between 0 and 1")
	ErrStakeCapExceeded         = sdkerrors.Register(ModuleName, 1106, "delegation would exceed the validator stake cap")
	ErrInvalidWithdrawal        = sdkerrors.Register(ModuleName, 1107, "invalid vault withdrawal")
)
