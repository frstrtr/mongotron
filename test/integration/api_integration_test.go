package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/frstrtr/mongotron/internal/api/handlers"
	"github.com/frstrtr/mongotron/internal/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	baseURL = "http://localhost:8080"
	apiPath = "/api/v1"
)

// TestAPIIntegration_FullFlow tests the complete API flow
// Note: This requires the API server to be running
func TestAPIIntegration_FullFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Test 1: Health Check
	t.Run("HealthCheck", func(t *testing.T) {
		resp, err := http.Get(baseURL + apiPath + "/health")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var health handlers.HealthResponse
		err = json.NewDecoder(resp.Body).Decode(&health)
		require.NoError(t, err)

		assert.Equal(t, "ok", health.Status)
		assert.NotEmpty(t, health.Version)
	})

	// Test 2: Create Subscription
	var subscriptionID string
	t.Run("CreateSubscription", func(t *testing.T) {
		reqBody := handlers.CreateSubscriptionRequest{
			Address: "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
			Filters: models.SubscriptionFilters{
				ContractTypes: []string{"TransferContract"},
				MinAmount:     0,
				MaxAmount:     0,
				OnlySuccess:   true,
			},
			StartBlock: 0,
		}

		body, _ := json.Marshal(reqBody)
		resp, err := http.Post(
			baseURL+apiPath+"/subscriptions",
			"application/json",
			bytes.NewReader(body),
		)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var subscription handlers.SubscriptionResponse
		err = json.NewDecoder(resp.Body).Decode(&subscription)
		require.NoError(t, err)

		assert.NotEmpty(t, subscription.SubscriptionID)
		assert.Equal(t, "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf", subscription.Address)
		assert.Equal(t, "active", subscription.Status)

		subscriptionID = subscription.SubscriptionID
	})

	// Test 3: Get Subscription
	t.Run("GetSubscription", func(t *testing.T) {
		require.NotEmpty(t, subscriptionID)

		resp, err := http.Get(baseURL + apiPath + "/subscriptions/" + subscriptionID)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var subscription handlers.SubscriptionResponse
		err = json.NewDecoder(resp.Body).Decode(&subscription)
		require.NoError(t, err)

		assert.Equal(t, subscriptionID, subscription.SubscriptionID)
	})

	// Test 4: List Subscriptions
	t.Run("ListSubscriptions", func(t *testing.T) {
		resp, err := http.Get(baseURL + apiPath + "/subscriptions?limit=10&skip=0")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result handlers.ListSubscriptionsResponse
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Greater(t, result.Total, int64(0))
		assert.NotEmpty(t, result.Subscriptions)
	})

	// Test 5: Wait for potential events (optional, may not generate events in test)
	time.Sleep(5 * time.Second)

	// Test 6: List Events
	t.Run("ListEvents", func(t *testing.T) {
		resp, err := http.Get(baseURL + apiPath + "/events?limit=10")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result handlers.ListEventsResponse
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		// Events may or may not exist depending on blockchain activity
		assert.GreaterOrEqual(t, result.Total, int64(0))
	})

	// Test 7: Delete Subscription
	t.Run("DeleteSubscription", func(t *testing.T) {
		require.NotEmpty(t, subscriptionID)

		req, err := http.NewRequest(
			"DELETE",
			baseURL+apiPath+"/subscriptions/"+subscriptionID,
			nil,
		)
		require.NoError(t, err)

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Equal(t, true, result["success"])
	})

	// Test 8: Verify Subscription Deleted
	t.Run("VerifySubscriptionDeleted", func(t *testing.T) {
		require.NotEmpty(t, subscriptionID)

		resp, err := http.Get(baseURL + apiPath + "/subscriptions/" + subscriptionID)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

// TestAPIIntegration_ErrorHandling tests error scenarios
func TestAPIIntegration_ErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("CreateSubscription_MissingAddress", func(t *testing.T) {
		reqBody := handlers.CreateSubscriptionRequest{
			Address: "",
		}

		body, _ := json.Marshal(reqBody)
		resp, err := http.Post(
			baseURL+apiPath+"/subscriptions",
			"application/json",
			bytes.NewReader(body),
		)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("GetSubscription_NotFound", func(t *testing.T) {
		resp, err := http.Get(baseURL + apiPath + "/subscriptions/sub_notfound")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("DeleteSubscription_NotFound", func(t *testing.T) {
		req, err := http.NewRequest(
			"DELETE",
			baseURL+apiPath+"/subscriptions/sub_notfound",
			nil,
		)
		require.NoError(t, err)

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})
}

// TestAPIIntegration_RateLimiting tests rate limiting
func TestAPIIntegration_RateLimiting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("RateLimit", func(t *testing.T) {
		// Make many requests rapidly
		successCount := 0
		rateLimitCount := 0

		for i := 0; i < 150; i++ {
			resp, err := http.Get(baseURL + apiPath + "/health")
			require.NoError(t, err)
			resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				successCount++
			} else if resp.StatusCode == http.StatusTooManyRequests {
				rateLimitCount++
			}
		}

		t.Logf("Success: %d, Rate Limited: %d", successCount, rateLimitCount)

		// Should hit rate limit (100 req/min)
		assert.Greater(t, rateLimitCount, 0, "Expected some requests to be rate limited")
	})
}

// TestAPIIntegration_Pagination tests pagination
func TestAPIIntegration_Pagination(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create multiple subscriptions
	var subscriptionIDs []string
	for i := 0; i < 5; i++ {
		reqBody := handlers.CreateSubscriptionRequest{
			Address:    fmt.Sprintf("TAddress%d", i),
			StartBlock: 0,
		}

		body, _ := json.Marshal(reqBody)
		resp, err := http.Post(
			baseURL+apiPath+"/subscriptions",
			"application/json",
			bytes.NewReader(body),
		)
		require.NoError(t, err)

		var subscription handlers.SubscriptionResponse
		json.NewDecoder(resp.Body).Decode(&subscription)
		resp.Body.Close()

		subscriptionIDs = append(subscriptionIDs, subscription.SubscriptionID)
		time.Sleep(100 * time.Millisecond) // Avoid rate limit
	}

	// Cleanup
	defer func() {
		client := &http.Client{}
		for _, id := range subscriptionIDs {
			req, _ := http.NewRequest("DELETE", baseURL+apiPath+"/subscriptions/"+id, nil)
			resp, _ := client.Do(req)
			if resp != nil {
				resp.Body.Close()
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	t.Run("ListWithPagination", func(t *testing.T) {
		// Get first page
		resp, err := http.Get(baseURL + apiPath + "/subscriptions?limit=2&skip=0")
		require.NoError(t, err)
		defer resp.Body.Close()

		var page1 handlers.ListSubscriptionsResponse
		json.NewDecoder(resp.Body).Decode(&page1)

		assert.Equal(t, int64(2), page1.Limit)
		assert.Equal(t, int64(0), page1.Skip)
		assert.LessOrEqual(t, len(page1.Subscriptions), 2)

		// Get second page
		resp, err = http.Get(baseURL + apiPath + "/subscriptions?limit=2&skip=2")
		require.NoError(t, err)
		defer resp.Body.Close()

		var page2 handlers.ListSubscriptionsResponse
		json.NewDecoder(resp.Body).Decode(&page2)

		assert.Equal(t, int64(2), page2.Limit)
		assert.Equal(t, int64(2), page2.Skip)

		// Total should be the same
		assert.Equal(t, page1.Total, page2.Total)
	})
}

// TestAPIIntegration_Concurrent tests concurrent requests
func TestAPIIntegration_Concurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("ConcurrentHealthChecks", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Launch 10 concurrent health check requests
		results := make(chan error, 10)
		for i := 0; i < 10; i++ {
			go func() {
				resp, err := http.Get(baseURL + apiPath + "/health")
				if err != nil {
					results <- err
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					results <- fmt.Errorf("expected 200, got %d", resp.StatusCode)
					return
				}
				results <- nil
			}()
		}

		// Wait for all requests
		for i := 0; i < 10; i++ {
			select {
			case err := <-results:
				assert.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("Timeout waiting for concurrent requests")
			}
		}
	})
}
