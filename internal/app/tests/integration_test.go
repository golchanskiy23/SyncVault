package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"syncvault/internal/app"
)

// IntegrationTestSuite - интеграционные тесты
type IntegrationTestSuite struct {
	suite.Suite
	server  *httptest.Server
	app     *app.App
	baseURL string
	client  *http.Client
}

// SetupSuite - запуск тестового сервера
func (suite *IntegrationTestSuite) SetupSuite() {
	// Создание приложения
	testApp, err := app.New()
	suite.Require().NoError(err)
	suite.app = testApp

	// Настраиваем роуты для тестов
	testApp.SetupTestRoutes()

	// Создание тестового сервера с роутером приложения
	suite.server = httptest.NewServer(testApp.GetRouter())
	suite.baseURL = suite.server.URL
	suite.client = suite.server.Client()

	fmt.Printf("Test server started on %s", suite.baseURL)
}

// TearDownSuite - остановка тестового сервера
func (suite *IntegrationTestSuite) TearDownSuite() {
	if suite.server != nil {
		suite.server.Close()
		fmt.Printf("Test server stopped")
	}
}

// TestIntegration - запуск integration тестов
func TestIntegration(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}

// ==============================================================================
// FILE HANDLERS INTEGRATION TESTS
// ==============================================================================

func (suite *IntegrationTestSuite) TestFileHandler_CreateFile_Integration() {
	// Arrange
	requestBody := map[string]interface{}{
		"path":    fmt.Sprintf("/integration/test_%d.txt", time.Now().UnixNano()),
		"size":    int64(1024),
		"node_id": "integration-node",
	}
	body, _ := json.Marshal(requestBody)

	// Act
	resp, err := suite.client.Post(suite.baseURL+"/api/v1/files", "application/json", bytes.NewBuffer(body))
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	// Assert
	assert.Equal(suite.T(), http.StatusCreated, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), response)
}

func (suite *IntegrationTestSuite) TestFileHandler_GetFile_Integration() {
	// Arrange
	// Act
	resp, err := suite.client.Get(suite.baseURL + "/api/v1/files/file-123")
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	// Assert
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), response)
}

func (suite *IntegrationTestSuite) TestFileHandler_DeleteFile_Integration() {
	// Arrange
	// Act
	req, _ := http.NewRequest("DELETE", suite.baseURL+"/api/v1/files/file-123", nil)
	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	// Assert
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)
}

// ==============================================================================
// HEALTH HANDLERS INTEGRATION TESTS
// ==============================================================================

func (suite *IntegrationTestSuite) TestHealthHandler_Integration() {
	// Arrange
	// Act
	resp, err := suite.client.Get(suite.baseURL + "/api/v1/health")
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	// Assert
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), response)
}

func (suite *IntegrationTestSuite) TestPingHandler_Integration() {
	// Arrange
	// Act
	resp, err := suite.client.Get(suite.baseURL + "/api/v1/ping")
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	// Assert
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), response)
}

// ==============================================================================
// MIDDLEWARE INTEGRATION TESTS
// ==============================================================================

func (suite *IntegrationTestSuite) TestInternalAuthMiddleware_ValidToken_Integration() {
	// Arrange
	req, _ := http.NewRequest("GET", suite.baseURL+"/internal/test", nil)
	req.Header.Set("X-Internal-Token", "syncvault-internal-token")

	// Act
	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	// Assert
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)
}

func (suite *IntegrationTestSuite) TestInternalAuthMiddleware_InvalidToken_Integration() {
	// Arrange
	req, _ := http.NewRequest("GET", suite.baseURL+"/internal/test", nil)
	req.Header.Set("X-Internal-Token", "invalid-token")

	// Act
	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	// Assert
	assert.Equal(suite.T(), http.StatusUnauthorized, resp.StatusCode)
}

func (suite *IntegrationTestSuite) TestInternalAuthMiddleware_MissingToken_Integration() {
	// Arrange
	req, _ := http.NewRequest("GET", suite.baseURL+"/internal/test", nil)
	// No X-Internal-Token header set

	// Act
	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	// Assert
	assert.Equal(suite.T(), http.StatusUnauthorized, resp.StatusCode)
}

// ==============================================================================
// STORAGE HANDLERS INTEGRATION TESTS
// ==============================================================================

func (suite *IntegrationTestSuite) TestStorageHandler_CreateStorage_Integration() {
	// Arrange
	requestBody := map[string]interface{}{
		"name": "Integration Storage",
		"type": "s3",
		"config": map[string]interface{}{
			"bucket":     "integration-bucket",
			"region":     "us-west-2",
			"access_key": "AKIA...",
			"secret_key": "...",
		},
		"enabled": true,
	}
	body, _ := json.Marshal(requestBody)

	// Act
	resp, err := suite.client.Post(suite.baseURL+"/api/v1/storages", "application/json", bytes.NewBuffer(body))
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	// Assert
	assert.Equal(suite.T(), http.StatusCreated, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), response)
}

func (suite *IntegrationTestSuite) TestStorageHandler_GetStorageUsage_Integration() {
	// Arrange
	// Act
	resp, err := suite.client.Get(suite.baseURL + "/api/v1/storages/storage-123/usage")
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	// Assert
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), response)
}

// ==============================================================================
// SYNC HANDLERS INTEGRATION TESTS
// ==============================================================================

func (suite *IntegrationTestSuite) TestSyncHandler_StartSync_Integration() {
	// Arrange
	requestBody := map[string]interface{}{
		"node_id":   "integration-storage",
		"paths":     []string{"/integration/documents"},
		"recursive": true,
	}
	body, _ := json.Marshal(requestBody)

	// Act
	resp, err := suite.client.Post(suite.baseURL+"/api/v1/sync/start", "application/json", bytes.NewBuffer(body))
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	// Assert
	assert.Equal(suite.T(), http.StatusCreated, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), response)
}

func (suite *IntegrationTestSuite) TestSyncHandler_GetSyncStatus_Integration() {
	// Arrange
	// Act
	resp, err := suite.client.Get(suite.baseURL + "/api/v1/sync/status")
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	// Assert
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), response)
}
