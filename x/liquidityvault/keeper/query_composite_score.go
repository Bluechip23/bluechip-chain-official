package keeper

import (
	"context"

	"github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"bluechipChain/x/liquidityvault/types"
)

func (k Keeper) CompositeScore(goCtx context.Context, req *types.QueryCompositeScoreRequest) (*types.QueryCompositeScoreResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	score, found := k.GetCompositeScore(goCtx, req.ValidatorAddress)
	if !found {
		return nil, status.Errorf(codes.NotFound, "composite score not found for validator %s", req.ValidatorAddress)
	}

	return &types.QueryCompositeScoreResponse{Score: &score}, nil
}

func (k Keeper) ValidatorRankings(goCtx context.Context, req *types.QueryValidatorRankingsRequest) (*types.QueryValidatorRankingsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	scores := k.GetRankedValidators(goCtx)

	pageRes := &query.PageResponse{
		Total: uint64(len(scores)),
	}

	return &types.QueryValidatorRankingsResponse{
		Scores:     scores,
		Pagination: pageRes,
	}, nil
}
