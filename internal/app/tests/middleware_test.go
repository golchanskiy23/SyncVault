package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"syncvault/internal/app"
)

// MiddlewareTestSuite - тесты для middleware
type MiddlewareTestSuite struct {
	suite.Suite
	app *app.App
}

// SetupSuite - настройка тестовой среды
func (suite *MiddlewareTestSuite) SetupSuite() {
	testApp, err := app.New()
	suite.Require().NoError(err)
	suite.app = testApp
}

// TearDownSuite - очистка после тестов
func (suite *MiddlewareTestSuite) TearDownSuite() {
	if suite.app != nil {
		suite.app = nil
	}
}

// TestMiddleware - запуск middleware тестов
func TestMiddleware(t *testing.T) {
	suite.Run(t, new(MiddlewareTestSuite))
}

// ==============================================================================
// INTERNAL AUTH MIDDLEWARE TESTS
// ==============================================================================

func (suite *MiddlewareTestSuite) TestInternalAuthMiddleware_ValidToken() {
	// Arrange
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})
	
	middleware := suite.app.InternalAuthMiddleware(nextHandler)
	
	req := httptest.NewRequest("GET", "/internal/test", nil)
	req.Header.Set("X-Internal-Token", "syncvault-internal-token")
	
	// Act
	rr := httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)
	
	// Assert
	assert.Equal(suite.T(), http.StatusOK, rr.Code)
	assert.Equal(suite.T(), "success", rr.Body.String())
}

func (suite *MiddlewareTestSuite) TestInternalAuthMiddleware_InvalidToken() {
	// Arrange
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		suite.T().Error("Next handler should not be called with invalid token")
	})
	
	middleware := suite.app.InternalAuthMiddleware(nextHandler)
	
	req := httptest.NewRequest("GET", "/internal/test", nil)
	req.Header.Set("X-Internal-Token", "invalid-token")
	
	// Act
	rr := httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)
	
	// Assert
	assert.Equal(suite.T(), http.StatusUnauthorized, rr.Code)
	assert.Contains(suite.T(), rr.Body.String(), "Unauthorized")
}

func (suite *MiddlewareTestSuite) TestInternalAuthMiddleware_MissingToken() {
	// Arrange
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		suite.T().Error("Next handler should not be called with missing token")
	})
	
	middleware := suite.app.InternalAuthMiddleware(nextHandler)
	
	req := httptest.NewRequest("GET", "/internal/test", nil)
	// No X-Internal-Token header set
	
	// Act
	rr := httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)
	
	// Assert
	assert.Equal(suite.T(), http.StatusUnauthorized, rr.Code)
	assert.Contains(suite.T(), rr.Body.String(), "Unauthorized")
}

// ==============================================================================
// MIDDLEWARE CHAIN TESTS
// ==============================================================================

func (suite *MiddlewareTestSuite) TestMiddlewareChain_Order() {
	// Arrange
	callOrder := []string{}
	
	firstMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callOrder = append(callOrder, "first")
			next.ServeHTTP(w, r)
		})
	}
	
	secondMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callOrder = append(callOrder, "second")
			next.ServeHTTP(w, r)
		})
	}
	
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callOrder = append(callOrder, "final")
		w.WriteHeader(http.StatusOK)
	})
	
	// Act
	chain := firstMiddleware(secondMiddleware(finalHandler))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	chain.ServeHTTP(rr, req)
	
	// Assert
	assert.Equal(suite.T(), []string{"first", "second", "final"}, callOrder)
	assert.Equal(suite.T(), http.StatusOK, rr.Code)
}

func (suite *MiddlewareTestSuite) TestMiddlewareChain_Headers() {
	// Arrange
	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Custom-Header", "test-value")
			next.ServeHTTP(w, r)
		})
	}
	
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("response"))
	})
	
	// Act
	chain := middleware(handler)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	chain.ServeHTTP(rr, req)
	
	// Assert
	assert.Equal(suite.T(), http.StatusOK, rr.Code)
	assert.Equal(suite.T(), "test-value", rr.Header().Get("X-Custom-Header"))
	assert.Equal(suite.T(), "response", rr.Body.String())
}

func (suite *MiddlewareTestSuite) TestMiddlewareChain_RequestModification() {
	// Arrange
	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Header.Set("X-Added-By-Middleware", "added")
			next.ServeHTTP(w, r)
		})
	}
	
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		addedHeader := r.Header.Get("X-Added-By-Middleware")
		require.Equal(suite.T(), "added", addedHeader)
		w.WriteHeader(http.StatusOK)
	})
	
	// Act
	chain := middleware(handler)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	chain.ServeHTTP(rr, req)
	
	// Assert
	assert.Equal(suite.T(), http.StatusOK, rr.Code)
}
