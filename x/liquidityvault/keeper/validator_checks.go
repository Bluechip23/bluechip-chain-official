package keeper

import (
	"context"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"bluechipChain/x/liquidityvault/types"
)

// ExecuteSimpleCheck performs a simple validator check based only on stake amounts.
// This runs every SimpleCheckInterval blocks (~24h).
// In Phase 1, this is advisory only - it computes and stores rankings but does not
// modify CometBFT validator power.
func (k Keeper) ExecuteSimpleCheck(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	k.Logger().Info("executing simple validator check", "block_height", sdkCtx.BlockHeight())

	// Recalculate composite scores for all registered validators
	k.IterateVaults(ctx, func(vault types.Vault) bool {
		score, err := k.CalculateCompositeScore(ctx, vault.ValidatorAddress)
		if err != nil {
			k.Logger().Error("failed to calculate composite score",
				"validator", vault.ValidatorAddress,
				"error", err,
			)
			return false
		}

		if err := k.SetCompositeScore(ctx, score); err != nil {
			k.Logger().Error("failed to set composite score",
				"validator", vault.ValidatorAddress,
				"error", err,
			)
		}
		return false
	})

	// Get ranked validators (for logging/events)
	rankings := k.GetRankedValidators(ctx)

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			"simple_validator_check",
			sdk.NewAttribute("block_height", strconv.FormatInt(sdkCtx.BlockHeight(), 10)),
			sdk.NewAttribute("num_validators", strconv.Itoa(len(rankings))),
		),
	)

	k.Logger().Info("simple validator check complete",
		"num_validators", len(rankings),
	)
	return nil
}

// ExecuteComplexCheck performs a complex validator check using composite scores.
// This runs every ComplexCheckInterval blocks (~5 days).
// Uses both chain stake (primary) and vault value median (tiebreaker).
// In Phase 1, this is advisory only.
func (k Keeper) ExecuteComplexCheck(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	k.Logger().Info("executing complex validator check", "block_height", sdkCtx.BlockHeight())

	// Recalculate all composite scores with value post medians
	k.IterateVaults(ctx, func(vault types.Vault) bool {
		score, err := k.CalculateCompositeScore(ctx, vault.ValidatorAddress)
		if err != nil {
			k.Logger().Error("failed to calculate composite score",
				"validator", vault.ValidatorAddress,
				"error", err,
			)
			return false
		}

		if err := k.SetCompositeScore(ctx, score); err != nil {
			k.Logger().Error("failed to set composite score",
				"validator", vault.ValidatorAddress,
				"error", err,
			)
		}
		return false
	})

	rankings := k.GetRankedValidators(ctx)

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			"complex_validator_check",
			sdk.NewAttribute("block_height", strconv.FormatInt(sdkCtx.BlockHeight(), 10)),
			sdk.NewAttribute("num_validators", strconv.Itoa(len(rankings))),
		),
	)

	k.Logger().Info("complex validator check complete",
		"num_validators", len(rankings),
	)
	return nil
}
