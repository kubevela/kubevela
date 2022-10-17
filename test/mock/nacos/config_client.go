/*
Copyright 2022 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nacos

import (
	"reflect"

	"github.com/golang/mock/gomock"
	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

// MockIConfigClient is a mock of IConfigClient interface
type MockIConfigClient struct {
	ctrl     *gomock.Controller
	recorder *MockIConfigClientMockRecorder
}

// MockIConfigClientMockRecorder is the mock recorder for MockIConfigClient
type MockIConfigClientMockRecorder struct {
	mock *MockIConfigClient
}

// NewMockIConfigClient creates a new mock instance
func NewMockIConfigClient(ctrl *gomock.Controller) *MockIConfigClient {
	mock := &MockIConfigClient{ctrl: ctrl}
	mock.recorder = &MockIConfigClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockIConfigClient) EXPECT() *MockIConfigClientMockRecorder {
	return m.recorder
}

// GetConfig mock getting the config
func (m *MockIConfigClient) GetConfig(param vo.ConfigParam) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetConfig", param)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetConfig indicates an expected call of GetConfig
func (mr *MockIConfigClientMockRecorder) GetConfig(param interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetConfig", reflect.TypeOf((*MockIConfigClient)(nil).GetConfig), param)
}

// PublishConfig use to publish config to nacos server
// dataId  require
// group   require
// content require
// tenant ==>nacos.namespace optional
func (m *MockIConfigClient) PublishConfig(param vo.ConfigParam) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PublishConfig", param)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// PublishConfig indicates an expected call of PublishConfig
func (mr *MockIConfigClientMockRecorder) PublishConfig(param interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PublishConfig", reflect.TypeOf((*MockIConfigClient)(nil).PublishConfig), param)
}

// DeleteConfig use to delete config
// dataId  require
// group   require
// tenant ==>nacos.namespace optional
func (m *MockIConfigClient) DeleteConfig(param vo.ConfigParam) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteConfig", param)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DeleteConfig indicates an expected call of DeleteConfig
func (mr *MockIConfigClientMockRecorder) DeleteConfig(param interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteConfig", reflect.TypeOf((*MockIConfigClient)(nil).DeleteConfig), param)
}

// ListenConfig use to listen config change,it will callback OnChange() when config change
// dataId  require
// group   require
// onchange require
// tenant ==>nacos.namespace optional
func (m *MockIConfigClient) ListenConfig(params vo.ConfigParam) (err error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListenConfig", params)
	ret0, _ := ret[0].(error)
	return ret0
}

// ListenConfig indicates an expected call of ListenConfig
func (mr *MockIConfigClientMockRecorder) ListenConfig(params interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListenConfig", reflect.TypeOf((*MockIConfigClient)(nil).ListenConfig), params)
}

// CancelListenConfig use to cancel listen config change
// dataId  require
// group   require
// tenant ==>nacos.namespace optional
func (m *MockIConfigClient) CancelListenConfig(params vo.ConfigParam) (err error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CancelListenConfig", params)
	ret0, _ := ret[0].(error)
	return ret0
}

// CancelListenConfig indicates an expected call of CancelListenConfig
func (mr *MockIConfigClientMockRecorder) CancelListenConfig(params interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CancelListenConfig", reflect.TypeOf((*MockIConfigClient)(nil).CancelListenConfig), params)
}

// SearchConfig use to search nacos config
// search  require search=accurate--精确搜索  search=blur--模糊搜索
// group   option
// dataId  option
// tenant ==>nacos.namespace optional
// pageNo  option,default is 1
// pageSize option,default is 10
func (m *MockIConfigClient) SearchConfig(param vo.SearchConfigParm) (*model.ConfigPage, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SearchConfig", param)
	ret0, _ := ret[0].(*model.ConfigPage)
	ret1, _ := ret[0].(error)
	return ret0, ret1
}

// SearchConfig indicates an expected call of SearchConfig
func (mr *MockIConfigClientMockRecorder) SearchConfig(params interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SearchConfig", reflect.TypeOf((*MockIConfigClient)(nil).SearchConfig), params)
}

// CloseClient Close the GRPC client
func (m *MockIConfigClient) CloseClient() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "SearchConfig")
}

// CloseClient indicates an expected call of CloseClient
func (mr *MockIConfigClientMockRecorder) CloseClient(params interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CloseClient", reflect.TypeOf((*MockIConfigClient)(nil).CloseClient), params)
}
