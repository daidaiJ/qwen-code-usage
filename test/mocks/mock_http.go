// Package mocks Mock HTTP客户端
package mocks

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync"

	"github.com/panda/qwen-usage/internal/database"
)

// MockHTTPClient Mock HTTP客户端
type MockHTTPClient struct {
	mu sync.RWMutex

	// 记录请求
	Requests []struct {
		URL    string
		Method string
		Body   []byte
	}

	// 可配置的响应
	Response      *http.Response
	ResponseErr   error
	ResponseFunc  func(url string) (*http.Response, error)

	// 响应映射（按URL）
	ResponseMap map[string]*http.Response
}

// NewMockHTTPClient 创建MockHTTPClient
func NewMockHTTPClient() *MockHTTPClient {
	return &MockHTTPClient{
		Requests:    []struct{ URL, Method string; Body []byte }{},
		ResponseMap: make(map[string]*http.Response),
	}
}

// Do 执行HTTP请求
func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 读取请求body
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
	}

	m.Requests = append(m.Requests, struct {
		URL    string
		Method string
		Body   []byte
	}{
		URL:    req.URL.String(),
		Method: req.Method,
		Body:   body,
	})

	// 使用自定义函数
	if m.ResponseFunc != nil {
		return m.ResponseFunc(req.URL.String())
	}

	// 使用URL映射
	if resp, ok := m.ResponseMap[req.URL.String()]; ok {
		return resp, m.ResponseErr
	}

	// 使用默认响应
	if m.Response != nil {
		return m.Response, m.ResponseErr
	}

	// 默认返回成功
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader([]byte("{}"))),
	}, m.ResponseErr
}

// SetResponse 设置响应
func (m *MockHTTPClient) SetResponse(statusCode int, body interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var bodyBytes []byte
	if body != nil {
		bodyBytes, _ = json.Marshal(body)
	}

	m.Response = &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
	}
}

// SetResponseForURL 为特定URL设置响应
func (m *MockHTTPClient) SetResponseForURL(url string, statusCode int, body interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var bodyBytes []byte
	if body != nil {
		bodyBytes, _ = json.Marshal(body)
	}

	m.ResponseMap[url] = &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
	}
}

// SetRecordResponse 设置record响应
func (m *MockHTTPClient) SetRecordResponse(resp database.RecordResponse) {
	m.SetResponseForURL("/record", http.StatusOK, resp)
}

// SetSessionResponse 设置session响应
func (m *MockHTTPClient) SetSessionResponse(count int, message string) {
	m.SetResponseForURL("/session/start", http.StatusOK, database.SessionResponse{
		Count:   count,
		Message: message,
	})
	m.SetResponseForURL("/session/end", http.StatusOK, database.SessionResponse{
		Count:   count,
		Message: message,
	})
}

// Reset 重置mock状态
func (m *MockHTTPClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Requests = nil
	m.Response = nil
	m.ResponseErr = nil
	m.ResponseFunc = nil
	m.ResponseMap = make(map[string]*http.Response)
}