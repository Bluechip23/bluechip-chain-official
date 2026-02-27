package keeper

import (
	"context"
	"encoding/binary"
	"math/rand"

	"cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"bluechipChain/x/liquidityvault/types"
)

// AddValuePost adds a value post for a validator
func (k Keeper) AddValuePost(ctx context.Context, valAddr string, value math.Int, blockHeight int64) error {
	post := types.ValuePost{
		ValidatorAddress: valAddr,
		Value:            value,
		BlockHeight:      blockHeight,
	}

	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	bz, err := k.cdc.Marshal(&post)
	if err != nil {
		return err
	}
	store.Set(types.ValuePostKey(valAddr, blockHeight), bz)
	return nil
}

// GetValuePosts returns all value posts for a validator in the current interval
func (k Keeper) GetValuePosts(ctx context.Context, valAddr string) []types.ValuePost {
	var posts []types.ValuePost
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	prefix := types.ValuePostPrefixKey(valAddr)
	iter := storetypes.KVStorePrefixIterator(store, prefix)
	defer iter.Close()

	for ; iter.Valid(); iter.Next() {
		var post types.ValuePost
		k.cdc.MustUnmarshal(iter.Value(), &post)
		posts = append(posts, post)
	}
	return posts
}

// ClearValuePosts removes all value posts for a validator
func (k Keeper) ClearValuePosts(ctx context.Context, valAddr string) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	prefix := types.ValuePostPrefixKey(valAddr)
	iter := storetypes.KVStorePrefixIterator(store, prefix)
	defer iter.Close()

	keys := [][]byte{}
	for ; iter.Valid(); iter.Next() {
		keys = append(keys, iter.Key())
	}

	for _, key := range keys {
		store.Delete(key)
	}
}

// ClearAllValuePosts removes all value posts for all validators
func (k Keeper) ClearAllValuePosts(ctx context.Context) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	iter := storetypes.KVStorePrefixIterator(store, types.ValuePostKeyPrefix)
	defer iter.Close()

	keys := [][]byte{}
	for ; iter.Valid(); iter.Next() {
		keys = append(keys, iter.Key())
	}

	for _, key := range keys {
		store.Delete(key)
	}
}

// ScheduleValuePosts uses pseudo-random block selection to pick value post blocks
// within the given interval. Uses the current block hash as entropy source.
func (k Keeper) ScheduleValuePosts(ctx context.Context, startBlock, endBlock int64) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	params := k.GetParams(ctx)
	numPosts := params.ValuePostsPerComplexInterval

	// Clear any previously scheduled blocks
	k.ClearScheduledValuePosts(ctx)

	// Use block header hash as seed for deterministic pseudo-random
	headerHash := sdkCtx.HeaderHash()
	var seed int64
	if len(headerHash) >= 8 {
		seed = int64(binary.BigEndian.Uint64(headerHash[:8]))
	}
	rng := rand.New(rand.NewSource(seed))

	intervalSize := endBlock - startBlock
	if intervalSize <= 0 {
		return
	}

	// Generate unique block heights
	blocks := make(map[int64]bool)
	maxAttempts := numPosts * 10
	var attempts uint64
	for uint64(len(blocks)) < numPosts && attempts < maxAttempts {
		offset := rng.Int63n(intervalSize)
		block := startBlock + offset
		blocks[block] = true
		attempts++
	}

	// Store each scheduled block
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	for block := range blocks {
		store.Set(types.ScheduledValuePostKey(block), []byte{1})
	}

	k.Logger().Info("scheduled value posts",
		"num_posts", len(blocks),
		"start_block", startBlock,
		"end_block", endBlock,
	)
}

// IsValuePostBlock checks if the given block height is a scheduled value post block
func (k Keeper) IsValuePostBlock(ctx context.Context, blockHeight int64) bool {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	return store.Has(types.ScheduledValuePostKey(blockHeight))
}

// ClearScheduledValuePosts removes all scheduled value post blocks
func (k Keeper) ClearScheduledValuePosts(ctx context.Context) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	iter := storetypes.KVStorePrefixIterator(store, types.ScheduledValuePostKeyPrefix)
	defer iter.Close()

	keys := [][]byte{}
	for ; iter.Valid(); iter.Next() {
		keys = append(keys, iter.Key())
	}

	for _, key := range keys {
		store.Delete(key)
	}
}

// ExecuteValuePost records the current value of all vaults
func (k Keeper) ExecuteValuePost(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	blockHeight := sdkCtx.BlockHeight()

	k.IterateVaults(ctx, func(vault types.Vault) bool {
		// Query total vault value from pool contracts
		value, err := k.QueryTotalVaultValue(ctx, vault)
		if err != nil {
			k.Logger().Error("failed to query vault value for value post",
				"validator", vault.ValidatorAddress,
				"error", err,
			)
			// Use deposit amount as fallback
			value = vault.TotalDeposited.Amount
		}

		if err := k.AddValuePost(ctx, vault.ValidatorAddress, value, blockHeight); err != nil {
			k.Logger().Error("failed to add value post",
				"validator", vault.ValidatorAddress,
				"error", err,
			)
		}
		return false
	})

	k.Logger().Info("executed value post", "block_height", blockHeight)
	return nil
}

// GetLastSimpleCheckHeight returns the last simple check block height
func (k Keeper) GetLastSimpleCheckHeight(ctx context.Context) int64 {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	bz := store.Get(types.LastSimpleCheckHeightKey)
	if bz == nil || len(bz) < 8 {
		return 0
	}
	return int64(binary.BigEndian.Uint64(bz))
}

// SetLastSimpleCheckHeight stores the last simple check block height
func (k Keeper) SetLastSimpleCheckHeight(ctx context.Context, height int64) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	bz := make([]byte, 8)
	binary.BigEndian.PutUint64(bz, uint64(height))
	store.Set(types.LastSimpleCheckHeightKey, bz)
}

// GetLastComplexCheckHeight returns the last complex check block height
func (k Keeper) GetLastComplexCheckHeight(ctx context.Context) int64 {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	bz := store.Get(types.LastComplexCheckHeightKey)
	if bz == nil || len(bz) < 8 {
		return 0
	}
	return int64(binary.BigEndian.Uint64(bz))
}

// SetLastComplexCheckHeight stores the last complex check block height
func (k Keeper) SetLastComplexCheckHeight(ctx context.Context, height int64) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	bz := make([]byte, 8)
	binary.BigEndian.PutUint64(bz, uint64(height))
	store.Set(types.LastComplexCheckHeightKey, bz)
}
