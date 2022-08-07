// Code generated by mockery v2.11.0. DO NOT EDIT.

package mocks

import (
	context "context"

	client "github.com/cosmos/cosmos-sdk/client"

	coretypes "github.com/tendermint/tendermint/rpc/core/types"

	cosmosaccount "github.com/ignite/cli/ignite/pkg/cosmosaccount"

	cosmosclient "github.com/ignite/cli/ignite/pkg/cosmosclient"

	mock "github.com/stretchr/testify/mock"

	testing "testing"

	types "github.com/cosmos/cosmos-sdk/types"
)

// CosmosClient is an autogenerated mock type for the CosmosClient type
type CosmosClient struct {
	mock.Mock
}

// BroadcastTx provides a mock function with given fields: account, msgs
func (_m *CosmosClient) BroadcastTx(account cosmosaccount.Account, msgs ...types.Msg) (cosmosclient.Response, error) {
	_va := make([]interface{}, len(msgs))
	for _i := range msgs {
		_va[_i] = msgs[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, account)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 cosmosclient.Response
	if rf, ok := ret.Get(0).(func(cosmosaccount.Account, ...types.Msg) cosmosclient.Response); ok {
		r0 = rf(account, msgs...)
	} else {
		r0 = ret.Get(0).(cosmosclient.Response)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(cosmosaccount.Account, ...types.Msg) error); ok {
		r1 = rf(account, msgs...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ConsensusInfo provides a mock function with given fields: ctx, height
func (_m *CosmosClient) ConsensusInfo(ctx context.Context, height int64) (cosmosclient.ConsensusInfo, error) {
	ret := _m.Called(ctx, height)

	var r0 cosmosclient.ConsensusInfo
	if rf, ok := ret.Get(0).(func(context.Context, int64) cosmosclient.ConsensusInfo); ok {
		r0 = rf(ctx, height)
	} else {
		r0 = ret.Get(0).(cosmosclient.ConsensusInfo)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, int64) error); ok {
		r1 = rf(ctx, height)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Context provides a mock function with given fields:
func (_m *CosmosClient) Context() client.Context {
	ret := _m.Called()

	var r0 client.Context
	if rf, ok := ret.Get(0).(func() client.Context); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(client.Context)
	}

	return r0
}

// Status provides a mock function with given fields: ctx
func (_m *CosmosClient) Status(ctx context.Context) (*coretypes.ResultStatus, error) {
	ret := _m.Called(ctx)

	var r0 *coretypes.ResultStatus
	if rf, ok := ret.Get(0).(func(context.Context) *coretypes.ResultStatus); ok {
		r0 = rf(ctx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*coretypes.ResultStatus)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// NewCosmosClient creates a new instance of CosmosClient. It also registers a cleanup function to assert the mocks expectations.
func NewCosmosClient(t testing.TB) *CosmosClient {
	mock := &CosmosClient{}

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
