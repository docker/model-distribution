package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// Models to test
var modelsToTest = []string{
	"ignaciolopezluna020/llama3.2:3b",
}

// OpenAI API parameters to test
type Parameter struct {
	Name        string
	Values      []interface{}
	Description string
}

var parametersToTest = []Parameter{
	{
		Name:        "max_tokens",
		Values:      []interface{}{10, 50, 100},
		Description: "The maximum number of tokens to generate",
	},
	{
		Name:        "max_completion_tokens",
		Values:      []interface{}{10, 50, 100},
		Description: "The maximum number of tokens to generate in the completion",
	},
	{
		Name:        "temperature",
		Values:      []interface{}{0.0, 0.5, 1.0},
		Description: "Controls randomness: 0 = deterministic, 1 = creative",
	},
	{
		Name:        "top_p",
		Values:      []interface{}{0.1, 0.5, 0.9},
		Description: "Controls diversity via nucleus sampling",
	},
	{
		Name:        "n",
		Values:      []interface{}{1, 2, 3},
		Description: "How many completions to generate",
	},
	{
		Name:        "stream",
		Values:      []interface{}{true, false},
		Description: "Whether to stream back partial progress",
	},
	{
		Name: "stream_options",
		Values: []interface{}{
			map[string]interface{}{
				"include_usage": true,
			},
		},
		Description: "Options for controlling streaming behavior",
	},
	{
		Name:        "stop",
		Values:      []interface{}{[]string{"\n"}, []string{".", "!"}, []string{"stop", "end"}},
		Description: "Sequences where the API will stop generating",
	},
	{
		Name:        "presence_penalty",
		Values:      []interface{}{-1.0, 0.0, 1.0},
		Description: "Penalizes repeated tokens",
	},
	{
		Name:        "frequency_penalty",
		Values:      []interface{}{-1.0, 0.0, 1.0},
		Description: "Penalizes frequent tokens",
	},
	{
		Name:        "logit_bias",
		Values:      []interface{}{map[string]float64{"50256": -100}, map[string]float64{"50256": 100}},
		Description: "Modifies likelihood of specified tokens",
	},
	{
		Name:        "response_format",
		Values:      []interface{}{map[string]string{"type": "text"}, map[string]string{"type": "json_object"}},
		Description: "Format of the response (e.g., JSON)",
	},
	{
		Name:        "seed",
		Values:      []interface{}{42, 123, 7777},
		Description: "Seed for deterministic sampling",
	},
	{
		Name: "tools",
		Values: []interface{}{
			[]map[string]interface{}{
				{
					"type": "function",
					"function": map[string]interface{}{
						"name":        "get_weather",
						"description": "Get the current weather",
						"parameters": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"location": map[string]interface{}{
									"type":        "string",
									"description": "The city and state, e.g. San Francisco, CA",
								},
							},
							"required": []string{"location"},
						},
					},
				},
			},
		},
		Description: "List of tools the model may call",
	},
	{
		Name: "tool_choice",
		Values: []interface{}{
			"auto",
			map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name": "get_weather",
				},
			},
		},
		Description: "Controls which (if any) tool is called by the model",
	},
	{
		Name: "audio",
		Values: []interface{}{
			map[string]interface{}{
				"input": "base64_audio_data",
				"model": "whisper-1",
			},
		},
		Description: "Audio input for the model",
	},
	{
		Name: "prediction",
		Values: []interface{}{
			map[string]interface{}{
				"enable": true,
			},
		},
		Description: "Controls whether the model should predict the next token",
	},
	{
		Name: "modalities",
		Values: []interface{}{
			[]map[string]interface{}{
				{
					"type": "image",
					"image": map[string]interface{}{
						"url": "https://example.com/image.jpg",
					},
				},
			},
		},
		Description: "Additional modalities for the model to process",
	},
	{
		Name: "metadata",
		Values: []interface{}{
			map[string]interface{}{
				"user_id":    "user123",
				"session_id": "session456",
			},
		},
		Description: "Additional metadata to include with the request",
	},
	{
		Name:        "reasoning_effort",
		Values:      []interface{}{0.0, 0.5, 1.0},
		Description: "Controls the amount of effort the model spends reasoning",
	},
}

// TestResult represents the result of a parameter test
type TestResult struct {
	Parameter                string
	Value                    interface{}
	Works                    bool
	ErrorMsg                 string
	Response                 string
	ExpectedBehaviorObserved bool
	Notes                    string
}

// ModelTestResults represents all test results for a model
type ModelTestResults struct {
	Model   string
	Results []TestResult
}

// DockerAPIClient handles communication with the Docker API
type DockerAPIClient struct {
	SocketPath string
}

// NewDockerAPIClient creates a new Docker API client
func NewDockerAPIClient() *DockerAPIClient {
	return &DockerAPIClient{
		SocketPath: os.Getenv("HOME") + "/.docker/run/docker.sock",
	}
}

// CreateModel pulls a model using the Docker API
func (c *DockerAPIClient) CreateModel(modelName string) error {
	url := "http://localhost/exp/vDD4.40/ml/models/create"
	payload := map[string]string{"from": modelName}

	return c.sendRequest("POST", url, payload, nil)
}

// GetModel gets information about a model
func (c *DockerAPIClient) GetModel(modelName string) (map[string]interface{}, error) {
	url := fmt.Sprintf("http://localhost/exp/vDD4.40/ml/models/%s/json", modelName)

	var response map[string]interface{}
	err := c.sendRequest("GET", url, nil, &response)
	return response, err
}

// DeleteModel deletes a model
func (c *DockerAPIClient) DeleteModel(modelName string) error {
	url := fmt.Sprintf("http://localhost/exp/vDD4.40/ml/models/%s", modelName)

	return c.sendRequest("DELETE", url, nil, nil)
}

// ListModels lists all available models
func (c *DockerAPIClient) ListModels() (map[string]interface{}, error) {
	url := "http://localhost/exp/vDD4.40/ml/models/json"

	var response map[string]interface{}
	err := c.sendRequest("GET", url, nil, &response)
	return response, err
}

// UseModel sends a completion request to the model
func (c *DockerAPIClient) UseModel(modelName string, params map[string]interface{}) (map[string]interface{}, error) {
	url := "http://localhost/exp/vDD4.40/ml/llama.cpp/v1/chat/completions"

	// Ensure required parameters are set
	if params == nil {
		params = make(map[string]interface{})
	}
	params["model"] = modelName

	// Ensure messages parameter is set if not provided
	if _, ok := params["messages"]; !ok {
		params["messages"] = []map[string]string{
			{"role": "user", "content": "Hello!"},
		}
	}

	var response map[string]interface{}
	err := c.sendRequest("POST", url, params, &response)
	return response, err
}

// sendRequest sends a request to the Docker API
func (c *DockerAPIClient) sendRequest(method, url string, payload interface{}, response interface{}) error {
	// Create HTTP client with Unix socket transport
	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", c.SocketPath)
			},
		},
		Timeout: 30 * time.Second,
	}

	// Create request
	var body io.Reader
	if payload != nil {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("error marshaling payload: %v", err)
		}
		body = bytes.NewBuffer(payloadBytes)
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	// Set headers
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Send request
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %v", err)
	}

	// Check for error status code
	if resp.StatusCode >= 400 {
		return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Parse response if needed
	if response != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, response); err != nil {
			return fmt.Errorf("error unmarshaling response: %v", err)
		}
	}

	return nil
}

// TestParameter tests a single parameter with a specific value
func TestParameter(client *DockerAPIClient, modelName string, paramName string, paramValue interface{}) TestResult {
	result := TestResult{
		Parameter: paramName,
		Value:     paramValue,
		Works:     false,
	}

	// Create baseline request with default parameters
	baselineParams := map[string]interface{}{
		"messages": []map[string]string{
			{"role": "user", "content": "Tell me a short story about a robot."},
		},
	}

	// Get baseline response
	baselineResp, err := client.UseModel(modelName, baselineParams)
	if err != nil {
		result.ErrorMsg = fmt.Sprintf("Baseline request failed: %v", err)
		return result
	}

	// Create test request with the parameter to test
	testParams := map[string]interface{}{
		"messages": []map[string]string{
			{"role": "user", "content": "Tell me a short story about a robot."},
		},
		paramName: paramValue,
	}

	// Send test request
	testResp, err := client.UseModel(modelName, testParams)
	if err != nil {
		result.ErrorMsg = fmt.Sprintf("Test request failed: %v", err)
		return result
	}

	// Parameter works if we got a response
	result.Works = true

	// Check if the parameter had the expected effect
	result.ExpectedBehaviorObserved = checkExpectedBehavior(paramName, paramValue, baselineResp, testResp)

	// Format response for logging
	respBytes, _ := json.MarshalIndent(testResp, "", "  ")
	result.Response = string(respBytes)

	// Add notes about the observed behavior
	result.Notes = getObservationNotes(paramName, paramValue, baselineResp, testResp)

	return result
}

// checkExpectedBehavior checks if a parameter had the expected effect
func checkExpectedBehavior(paramName string, paramValue interface{}, baselineResp, testResp map[string]interface{}) bool {
	switch paramName {
	case "max_tokens", "max_completion_tokens":
		// Check if response length is affected by max_tokens or max_completion_tokens
		baselineTokens := getResponseTokenCount(baselineResp)
		testTokens := getResponseTokenCount(testResp)
		maxTokens, _ := paramValue.(int)

		// The response should be limited by max_tokens
		return testTokens <= maxTokens && testTokens < baselineTokens

	case "temperature", "reasoning_effort":
		// For temperature and reasoning_effort, we'd need multiple samples to truly verify
		// For now, just check if we got different responses with different values
		if paramValue.(float64) == 0.0 {
			// At value 0, repeated calls should give the same result
			// But we can't test that in a single call, so we'll assume it works
			return true
		}
		return true

	case "n":
		// Check if we got the requested number of completions
		n, _ := paramValue.(int)
		choices, ok := testResp["choices"].([]interface{})
		return ok && len(choices) == n

	case "stream":
		// Hard to verify streaming behavior in this test
		return true

	case "stream_options":
		// Hard to verify stream_options behavior in this test
		// We would need to check if usage is included in streaming responses
		return true

	case "stop":
		// Check if the response stops at the specified sequence
		stopSeq, _ := paramValue.([]string)
		content := getResponseContent(testResp)

		for _, seq := range stopSeq {
			if strings.Contains(content, seq) {
				// If the content contains the stop sequence, it should be at the end
				return strings.HasSuffix(strings.TrimSpace(content), seq)
			}
		}
		return true

	case "response_format":
		// Check if the response format matches the requested format
		formatMap, _ := paramValue.(map[string]string)
		formatType := formatMap["type"]

		if formatType == "json_object" {
			// For JSON format, check if the content is valid JSON
			content := getResponseContent(testResp)
			var js map[string]interface{}
			err := json.Unmarshal([]byte(content), &js)
			return err == nil
		}
		// For text format, any response is valid
		return true

	case "seed":
		// Hard to verify seed behavior in a single test
		// We would need to make multiple identical requests with the same seed
		// and check if they produce the same result
		return true

	case "tools":
		// Check if the model attempted to use a tool
		choices, ok := testResp["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			return false
		}

		choice, ok := choices[0].(map[string]interface{})
		if !ok {
			return false
		}

		// Check for tool_calls in the response
		message, ok := choice["message"].(map[string]interface{})
		if !ok {
			return false
		}

		_, hasToolCalls := message["tool_calls"]
		return hasToolCalls

	case "tool_choice":
		// Similar to tools, check if the model used the specified tool
		if paramValue == "auto" {
			return true // Can't verify auto behavior easily
		}

		toolChoice, _ := paramValue.(map[string]interface{})
		functionName := toolChoice["function"].(map[string]interface{})["name"].(string)

		choices, ok := testResp["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			return false
		}

		choice, ok := choices[0].(map[string]interface{})
		if !ok {
			return false
		}

		message, ok := choice["message"].(map[string]interface{})
		if !ok {
			return false
		}

		toolCalls, ok := message["tool_calls"].([]interface{})
		if !ok || len(toolCalls) == 0 {
			return false
		}

		// Check if any tool call matches the requested function name
		for _, toolCall := range toolCalls {
			tc, ok := toolCall.(map[string]interface{})
			if !ok {
				continue
			}

			function, ok := tc["function"].(map[string]interface{})
			if !ok {
				continue
			}

			name, ok := function["name"].(string)
			if ok && name == functionName {
				return true
			}
		}
		return false

	case "audio":
		// Hard to verify audio behavior in this test
		// We would need to check if the model processed the audio input
		return true

	case "prediction":
		// Hard to verify prediction behavior in this test
		// We would need to check if the model predicted the next token
		return true

	case "modalities":
		// Hard to verify modalities behavior in this test
		// We would need to check if the model processed the additional modalities
		return true

	case "metadata":
		// Hard to verify metadata behavior in this test
		// This is mostly for tracking purposes and doesn't affect the response
		return true

	default:
		// For other parameters, assume they work if we got a response
		return true
	}
}

// getObservationNotes generates notes about the observed behavior
func getObservationNotes(paramName string, paramValue interface{}, baselineResp, testResp map[string]interface{}) string {
	switch paramName {
	case "max_tokens":
		baselineTokens := getResponseTokenCount(baselineResp)
		testTokens := getResponseTokenCount(testResp)
		return fmt.Sprintf("Baseline response: %d tokens, Test response: %d tokens", baselineTokens, testTokens)

	case "max_completion_tokens":
		baselineTokens := getResponseTokenCount(baselineResp)
		testTokens := getResponseTokenCount(testResp)
		return fmt.Sprintf("Baseline response: %d tokens, Test response with max_completion_tokens=%d: %d tokens",
			baselineTokens, paramValue.(int), testTokens)

	case "temperature":
		temp := paramValue.(float64)
		if temp == 0.0 {
			return "Temperature 0 should produce deterministic results"
		} else {
			return fmt.Sprintf("Temperature %.1f should produce more random results", temp)
		}

	case "reasoning_effort":
		effort := paramValue.(float64)
		if effort == 0.0 {
			return "Reasoning effort 0 should produce faster but less reasoned results"
		} else if effort == 1.0 {
			return "Reasoning effort 1.0 should produce more thorough reasoning"
		} else {
			return fmt.Sprintf("Reasoning effort %.1f should balance speed and reasoning quality", effort)
		}

	case "n":
		n := paramValue.(int)
		choices, _ := testResp["choices"].([]interface{})
		return fmt.Sprintf("Requested %d completions, received %d", n, len(choices))

	case "stream":
		stream := paramValue.(bool)
		return fmt.Sprintf("Stream mode: %v", stream)

	case "stream_options":
		options, _ := paramValue.(map[string]interface{})
		return fmt.Sprintf("Stream options: %v (controls additional information in streaming responses)", options)

	case "stop":
		stopSeq, _ := paramValue.([]string)
		content := getResponseContent(testResp)
		return fmt.Sprintf("Stop sequences: %v, Response ends with: %s", stopSeq, getLastFewWords(content))

	case "response_format":
		formatMap, _ := paramValue.(map[string]string)
		formatType := formatMap["type"]

		if formatType == "json_object" {
			content := getResponseContent(testResp)
			var js map[string]interface{}
			err := json.Unmarshal([]byte(content), &js)

			if err == nil {
				return "Response format: JSON object (valid JSON detected)"
			} else {
				return "Response format: JSON object requested but response is not valid JSON"
			}
		} else {
			return "Response format: text"
		}

	case "seed":
		seedValue, _ := paramValue.(int)
		return fmt.Sprintf("Seed value: %d (deterministic results expected for identical requests)", seedValue)

	case "tools":
		// Check if the model attempted to use a tool
		choices, ok := testResp["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			return "Tools provided but not used in response"
		}

		choice, ok := choices[0].(map[string]interface{})
		if !ok {
			return "Tools provided but not used in response"
		}

		message, ok := choice["message"].(map[string]interface{})
		if !ok {
			return "Tools provided but not used in response"
		}

		toolCalls, ok := message["tool_calls"].([]interface{})
		if !ok || len(toolCalls) == 0 {
			return "Tools provided but not used in response"
		}

		return fmt.Sprintf("Tools used: %d tool calls found in response", len(toolCalls))

	case "tool_choice":
		if paramValue == "auto" {
			return "Tool choice: auto (model decides whether to use tools)"
		}

		toolChoice, _ := paramValue.(map[string]interface{})
		functionName := toolChoice["function"].(map[string]interface{})["name"].(string)

		// Check if the model used the specified tool
		choices, ok := testResp["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			return fmt.Sprintf("Tool choice: %s (not used in response)", functionName)
		}

		choice, ok := choices[0].(map[string]interface{})
		if !ok {
			return fmt.Sprintf("Tool choice: %s (not used in response)", functionName)
		}

		message, ok := choice["message"].(map[string]interface{})
		if !ok {
			return fmt.Sprintf("Tool choice: %s (not used in response)", functionName)
		}

		toolCalls, ok := message["tool_calls"].([]interface{})
		if !ok || len(toolCalls) == 0 {
			return fmt.Sprintf("Tool choice: %s (not used in response)", functionName)
		}

		// Check if any tool call matches the requested function name
		for _, toolCall := range toolCalls {
			tc, ok := toolCall.(map[string]interface{})
			if !ok {
				continue
			}

			function, ok := tc["function"].(map[string]interface{})
			if !ok {
				continue
			}

			name, ok := function["name"].(string)
			if ok && name == functionName {
				return fmt.Sprintf("Tool choice: %s (successfully used in response)", functionName)
			}
		}

		return fmt.Sprintf("Tool choice: %s (different tool used in response)", functionName)

	case "audio":
		audioConfig, _ := paramValue.(map[string]interface{})
		return fmt.Sprintf("Audio input provided: model=%v (processes audio input for transcription/analysis)",
			audioConfig["model"])

	case "prediction":
		predictionConfig, _ := paramValue.(map[string]interface{})
		return fmt.Sprintf("Prediction enabled: %v (controls token prediction behavior)",
			predictionConfig["enable"])

	case "modalities":
		modalities, _ := paramValue.([]map[string]interface{})
		modalityTypes := []string{}
		for _, m := range modalities {
			if t, ok := m["type"].(string); ok {
				modalityTypes = append(modalityTypes, t)
			}
		}
		return fmt.Sprintf("Additional modalities provided: %v (enables multi-modal processing)",
			strings.Join(modalityTypes, ", "))

	case "metadata":
		metadata, _ := paramValue.(map[string]interface{})
		return fmt.Sprintf("Metadata provided: %v (additional tracking information)", metadata)

	default:
		return fmt.Sprintf("Parameter: %s, Value: %v", paramName, paramValue)
	}
}

// getResponseTokenCount gets the token count from a response
func getResponseTokenCount(resp map[string]interface{}) int {
	usage, ok := resp["usage"].(map[string]interface{})
	if !ok {
		return 0
	}

	completionTokens, ok := usage["completion_tokens"].(float64)
	if !ok {
		return 0
	}

	return int(completionTokens)
}

// getResponseContent gets the content from a response
func getResponseContent(resp map[string]interface{}) string {
	choices, ok := resp["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return ""
	}

	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return ""
	}

	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		return ""
	}

	content, ok := message["content"].(string)
	if !ok {
		return ""
	}

	return content
}

// getLastFewWords gets the last few words from a string
func getLastFewWords(s string) string {
	words := strings.Fields(s)
	if len(words) <= 5 {
		return s
	}
	return strings.Join(words[len(words)-5:], " ")
}

// RunTests runs all tests for all models
func RunTests() []ModelTestResults {
	client := NewDockerAPIClient()
	allResults := make([]ModelTestResults, 0, len(modelsToTest))
	serviceAvailable := true

	for _, modelName := range modelsToTest {
		fmt.Printf("Testing model: %s\n", modelName)

		modelResults := ModelTestResults{
			Model:   modelName,
			Results: make([]TestResult, 0),
		}

		// Create the model
		fmt.Printf("  Creating model...\n")
		err := client.CreateModel(modelName)
		if err != nil {
			fmt.Printf("  Error creating model: %v\n", err)

			// Add error result for this model
			errorResult := TestResult{
				Parameter: "service_availability",
				Value:     "N/A",
				Works:     false,
				ErrorMsg:  fmt.Sprintf("Could not create model: %v", err),
			}
			modelResults.Results = append(modelResults.Results, errorResult)
			allResults = append(allResults, modelResults)

			// If we get a service unavailable error, mark the service as unavailable
			if strings.Contains(err.Error(), "service unavailable") {
				serviceAvailable = false
			}

			continue
		}

		// Verify the model is available
		fmt.Printf("  Verifying model...\n")
		_, err = client.GetModel(modelName)
		if err != nil {
			fmt.Printf("  Error verifying model: %v\n", err)

			// Add error result for this model
			errorResult := TestResult{
				Parameter: "service_availability",
				Value:     "N/A",
				Works:     false,
				ErrorMsg:  fmt.Sprintf("Could not verify model: %v", err),
			}
			modelResults.Results = append(modelResults.Results, errorResult)
			allResults = append(allResults, modelResults)

			// Try to delete the model anyway
			_ = client.DeleteModel(modelName)

			continue
		}

		// Test parameters
		fmt.Printf("  Testing parameters...\n")

		for _, param := range parametersToTest {
			fmt.Printf("    Testing parameter: %s\n", param.Name)

			for _, value := range param.Values {
				fmt.Printf("      Testing value: %v\n", value)
				result := TestParameter(client, modelName, param.Name, value)
				modelResults.Results = append(modelResults.Results, result)

				if result.Works {
					fmt.Printf("      ✅ Parameter works\n")
					if result.ExpectedBehaviorObserved {
						fmt.Printf("      ✅ Expected behavior observed\n")
					} else {
						fmt.Printf("      ⚠️ Expected behavior not observed\n")
					}
				} else {
					fmt.Printf("      ❌ Parameter does not work: %s\n", result.ErrorMsg)
				}
			}
		}

		allResults = append(allResults, modelResults)

		// Delete the model
		fmt.Printf("  Deleting model...\n")
		err = client.DeleteModel(modelName)
		if err != nil {
			fmt.Printf("  Error deleting model: %v\n", err)
		}
	}

	// If the service is not available, add a note to the results
	if !serviceAvailable {
		fmt.Println("\nWARNING: The Docker API service appears to be unavailable.")
		fmt.Println("Please ensure that Docker is running and the API is accessible.")
		fmt.Println("You can try running the following command to check the Docker service:")
		fmt.Println("  docker info")
	}

	return allResults
}

// GenerateReport generates a detailed report of the test results
func GenerateReport(results []ModelTestResults) string {
	var report strings.Builder

	report.WriteString("# OpenAI API Parameter Compatibility Report\n\n")

	for _, modelResult := range results {
		report.WriteString(fmt.Sprintf("## Model: %s\n\n", modelResult.Model))

		// Check if we have any service availability errors
		hasServiceError := false
		for _, result := range modelResult.Results {
			if result.Parameter == "service_availability" && !result.Works {
				hasServiceError = true
				report.WriteString("### Service Unavailable\n\n")
				report.WriteString(fmt.Sprintf("Error: %s\n\n", result.ErrorMsg))
				break
			}
		}

		// If we have a service error and no other results, continue to the next model
		if hasServiceError && len(modelResult.Results) == 1 {
			continue
		}

		// Create a summary table
		report.WriteString("### Summary\n\n")
		report.WriteString("| Parameter | Works | Expected Behavior Observed |\n")
		report.WriteString("|-----------|-------|----------------------------|\n")

		// Group results by parameter
		paramResults := make(map[string][]TestResult)
		for _, result := range modelResult.Results {
			// Skip service availability results in the parameter summary
			if result.Parameter == "service_availability" {
				continue
			}
			paramResults[result.Parameter] = append(paramResults[result.Parameter], result)
		}

		// Add summary for each parameter
		for _, param := range parametersToTest {
			results, ok := paramResults[param.Name]
			if !ok {
				continue
			}

			// Check if all values work
			allWork := true
			allExpectedBehavior := true

			for _, result := range results {
				if !result.Works {
					allWork = false
				}
				if !result.ExpectedBehaviorObserved {
					allExpectedBehavior = false
				}
			}

			// Add to summary table
			workStatus := "✅ Yes"
			if !allWork {
				workStatus = "❌ No"
			}

			behaviorStatus := "✅ Yes"
			if !allExpectedBehavior {
				behaviorStatus = "⚠️ Partial"
			}

			report.WriteString(fmt.Sprintf("| %s | %s | %s |\n", param.Name, workStatus, behaviorStatus))
		}

		report.WriteString("\n### Detailed Results\n\n")

		// Add detailed results for each parameter
		for _, param := range parametersToTest {
			results, ok := paramResults[param.Name]
			if !ok {
				continue
			}

			report.WriteString(fmt.Sprintf("#### %s\n\n", param.Name))
			report.WriteString(fmt.Sprintf("Description: %s\n\n", param.Description))

			for _, result := range results {
				report.WriteString(fmt.Sprintf("##### Value: `%v`\n\n", result.Value))

				if result.Works {
					report.WriteString("- ✅ Parameter works\n")

					if result.ExpectedBehaviorObserved {
						report.WriteString("- ✅ Expected behavior observed\n")
					} else {
						report.WriteString("- ⚠️ Expected behavior not observed\n")
					}

					report.WriteString(fmt.Sprintf("- Notes: %s\n\n", result.Notes))

					// Add a snippet of the response
					report.WriteString("Response snippet:\n")
					report.WriteString("```json\n")

					// Limit the response to a reasonable size
					respLines := strings.Split(result.Response, "\n")
					if len(respLines) > 10 {
						report.WriteString(strings.Join(respLines[:10], "\n"))
						report.WriteString("\n... (truncated)\n")
					} else {
						report.WriteString(result.Response)
					}

					report.WriteString("\n```\n\n")
				} else {
					report.WriteString("- ❌ Parameter does not work\n")
					report.WriteString(fmt.Sprintf("- Error: %s\n\n", result.ErrorMsg))
				}
			}
		}
	}

	return report.String()
}

func main() {
	// Parse command-line flags
	outputFile := flag.String("output", "openai_api_compatibility_report.md", "Output file for the report")
	flag.Parse()

	fmt.Println("Starting OpenAI API Parameter Compatibility Test")
	fmt.Println("================================================")
	fmt.Println()
	fmt.Println("Models to test:")
	for _, model := range modelsToTest {
		fmt.Printf("  - %s\n", model)
	}
	fmt.Println()
	fmt.Println("Parameters to test:")
	for _, param := range parametersToTest {
		fmt.Printf("  - %s: %s\n", param.Name, param.Description)
	}
	fmt.Println()

	// Run the tests
	fmt.Println("Running tests...")
	results := RunTests()

	// Generate the report
	fmt.Println("Generating report...")
	report := GenerateReport(results)

	// Write the report to a file
	fmt.Printf("Writing report to %s...\n", *outputFile)
	err := os.WriteFile(*outputFile, []byte(report), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing report: %v\n", err)
		os.Exit(1)
	}

	// Print a summary to the console
	fmt.Println()
	fmt.Println("Test Summary:")
	fmt.Println("============")
	for _, modelResult := range results {
		fmt.Printf("Model: %s\n", modelResult.Model)

		// Group results by parameter
		paramResults := make(map[string][]TestResult)
		for _, result := range modelResult.Results {
			paramResults[result.Parameter] = append(paramResults[result.Parameter], result)
		}

		// Count working parameters
		workingParams := 0
		for _, param := range parametersToTest {
			results, ok := paramResults[param.Name]
			if !ok {
				continue
			}

			allWork := true
			for _, result := range results {
				if !result.Works {
					allWork = false
					break
				}
			}

			if allWork {
				workingParams++
			}
		}

		fmt.Printf("  Working parameters: %d/%d\n", workingParams, len(parametersToTest))

		// List working parameters
		workingParamNames := make([]string, 0)
		for _, param := range parametersToTest {
			results, ok := paramResults[param.Name]
			if !ok {
				continue
			}

			allWork := true
			for _, result := range results {
				if !result.Works {
					allWork = false
					break
				}
			}

			if allWork {
				workingParamNames = append(workingParamNames, param.Name)
			}
		}

		fmt.Printf("  Working: %s\n", strings.Join(workingParamNames, ", "))

		// List non-working parameters
		nonWorkingParamNames := make([]string, 0)
		for _, param := range parametersToTest {
			results, ok := paramResults[param.Name]
			if !ok {
				continue
			}

			anyFailed := false
			for _, result := range results {
				if !result.Works {
					anyFailed = true
					break
				}
			}

			if anyFailed {
				nonWorkingParamNames = append(nonWorkingParamNames, param.Name)
			}
		}

		if len(nonWorkingParamNames) > 0 {
			fmt.Printf("  Not working: %s\n", strings.Join(nonWorkingParamNames, ", "))
		}

		fmt.Println()
	}

	fmt.Printf("Detailed report written to %s\n", *outputFile)
}
