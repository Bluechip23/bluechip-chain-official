package keeper

import (
	"context"
	"sort"

	"cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"bluechipChain/x/liquidityvault/types"
)

// SetCompositeScore stores a validator's composite score
func (k Keeper) SetCompositeScore(ctx context.Context, score types.CompositeScore) error {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	bz, err := k.cdc.Marshal(&score)
	if err != nil {
		return err
	}
	store.Set(types.CompositeScoreKey(score.ValidatorAddress), bz)
	return nil
}

// GetCompositeScore retrieves a validator's composite score
func (k Keeper) GetCompositeScore(ctx context.Context, valAddr string) (types.CompositeScore, bool) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	bz := store.Get(types.CompositeScoreKey(valAddr))
	if bz == nil {
		return types.CompositeScore{}, false
	}

	var score types.CompositeScore
	k.cdc.MustUnmarshal(bz, &score)
	return score, true
}

// GetAllCompositeScores returns all composite scores
func (k Keeper) GetAllCompositeScores(ctx context.Context) []types.CompositeScore {
	var scores []types.CompositeScore
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	iter := storetypes.KVStorePrefixIterator(store, types.CompositeScoreKeyPrefix)
	defer iter.Close()

	for ; iter.Valid(); iter.Next() {
		var score types.CompositeScore
		k.cdc.MustUnmarshal(iter.Value(), &score)
		scores = append(scores, score)
	}
	return scores
}

// CalculateCompositeScore computes the composite score for a validator.
// Primary component: chain stake (from staking module)
// Tiebreaker component: median of value posts (vault value)
func (k Keeper) CalculateCompositeScore(ctx context.Context, valAddr string) (types.CompositeScore, error) {
	score := types.CompositeScore{
		ValidatorAddress: valAddr,
		ChainStake:       math.ZeroInt(),
		VaultValue:       math.ZeroInt(),
	}

	// Get chain stake from staking module
	valAddress, err := sdk.ValAddressFromBech32(valAddr)
	if err != nil {
		return score, err
	}

	validator, err := k.stakingKeeper.GetValidator(ctx, valAddress)
	if err == nil {
		score.ChainStake = validator.GetBondedTokens()
	}

	// Get vault value from value posts (median)
	valuePosts := k.GetValuePosts(ctx, valAddr)
	if len(valuePosts) > 0 {
		score.VaultValue = CalculateMedianValue(valuePosts)
	} else {
		// Fallback: use current vault total deposited
		vault, found := k.GetVault(ctx, valAddr)
		if found {
			score.VaultValue = vault.TotalDeposited.Amount
		}
	}

	return score, nil
}

// CalculateMedianValue computes the median value from a slice of value posts
func CalculateMedianValue(posts []types.ValuePost) math.Int {
	if len(posts) == 0 {
		return math.ZeroInt()
	}

	// Extract values and sort
	values := make([]math.Int, len(posts))
	for i, post := range posts {
		values[i] = post.Value
	}

	sort.Slice(values, func(i, j int) bool {
		return values[i].LT(values[j])
	})

	mid := len(values) / 2
	if len(values)%2 == 0 {
		// Average of two middle values
		return values[mid-1].Add(values[mid]).Quo(math.NewInt(2))
	}
	return values[mid]
}

// GetRankedValidators returns all validators sorted by composite score.
// Primary: chain_stake descending. Tiebreaker: vault_value descending.
func (k Keeper) GetRankedValidators(ctx context.Context) []types.CompositeScore {
	scores := k.GetAllCompositeScores(ctx)

	sort.Slice(scores, func(i, j int) bool {
		cmp := scores[i].ChainStake.Sub(scores[j].ChainStake)
		if !cmp.IsZero() {
			return cmp.IsPositive() // descending
		}
		return scores[i].VaultValue.GT(scores[j].VaultValue) // tiebreaker descending
	})

	return scores
}
