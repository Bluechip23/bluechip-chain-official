package types

// DONTCOVER

import (
	sdkerrors "cosmossdk.io/errors"
)

// x/liquidityvault module sentinel errors
var (
	ErrInvalidSigner             = sdkerrors.Register(ModuleName, 1100, "expected gov account as only signer for proposal message")
	ErrVaultNotFound             = sdkerrors.Register(ModuleName, 1101, "vault not found")
	ErrVaultAlreadyExists        = sdkerrors.Register(ModuleName, 1102, "vault already exists for this validator")
	ErrNotValidator              = sdkerrors.Register(ModuleName, 1103, "address is not a registered validator")
	ErrStakeCapExceeded          = sdkerrors.Register(ModuleName, 1104, "delegation would exceed stake cap")
	ErrInvalidDelegatorPercent   = sdkerrors.Register(ModuleName, 1105, "delegator reward percent must be between 0 and 100")
	ErrInvalidValidatorType      = sdkerrors.Register(ModuleName, 1106, "invalid validator type")
	ErrInvalidPoolAddress        = sdkerrors.Register(ModuleName, 1107, "invalid pool contract address")
	ErrWasmExecutionFailed       = sdkerrors.Register(ModuleName, 1108, "wasm contract execution failed")
	ErrWasmQueryFailed           = sdkerrors.Register(ModuleName, 1109, "wasm contract query failed")
	ErrInvalidDepositAmount      = sdkerrors.Register(ModuleName, 1110, "invalid deposit amount")
	ErrWasmKeeperNotSet          = sdkerrors.Register(ModuleName, 1111, "wasm keeper not set")
	ErrValidatorNotRegistered    = sdkerrors.Register(ModuleName, 1112, "validator not registered in liquidityvault")
)
