package github

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type ssmParameter struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}

type ssmResponse struct {
	Parameters []ssmParameter `json:"Parameters"`
}

// FetchTokenFromSSM retrieves the GitHub token from AWS SSM Parameter Store
func FetchTokenFromSSM(profile, env, region string) (string, error) {
	if region == "" {
		region = "us-east-1"
	}

	paramName := fmt.Sprintf("/app/%s/githubToken", env)

	args := []string{
		"ssm", "get-parameters",
		"--names", paramName,
		"--with-decryption",
		"--region", region,
	}

	if profile != "" {
		args = append(args, "--profile", profile)
	}

	cmd := exec.Command("aws", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("failed to fetch GitHub token: %s", string(exitErr.Stderr))
		}
		return "", fmt.Errorf("failed to fetch GitHub token: %w", err)
	}

	var resp ssmResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("failed to parse SSM response: %w", err)
	}

	for _, param := range resp.Parameters {
		if param.Name == paramName {
			return strings.TrimSpace(param.Value), nil
		}
	}

	return "", fmt.Errorf("GitHub token not found at %s", paramName)
}

// maxSSMParamsPerRequest is the AWS GetParameters limit (10 names per call)
const maxSSMParamsPerRequest = 10

// FetchMultipleFromSSM retrieves multiple parameters from AWS SSM, batching requests
// (GetParameters allows at most 10 names per call).
func FetchMultipleFromSSM(profile, env, region string, paramSuffixes []string) (map[string]string, error) {
	if region == "" {
		region = "us-east-1"
	}

	prefix := fmt.Sprintf("/app/%s/", env)
	result := make(map[string]string)

	for i := 0; i < len(paramSuffixes); i += maxSSMParamsPerRequest {
		end := i + maxSSMParamsPerRequest
		if end > len(paramSuffixes) {
			end = len(paramSuffixes)
		}
		batch := paramSuffixes[i:end]

		var paramNames []string
		for _, suffix := range batch {
			paramNames = append(paramNames, fmt.Sprintf("/app/%s/%s", env, suffix))
		}

		args := []string{
			"ssm", "get-parameters",
			"--names",
		}
		args = append(args, paramNames...)
		args = append(args, "--with-decryption", "--region", region)

		if profile != "" {
			args = append(args, "--profile", profile)
		}

		cmd := exec.Command("aws", args...)
		out, err := cmd.Output()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return nil, fmt.Errorf("failed to fetch parameters: %s", string(exitErr.Stderr))
			}
			return nil, fmt.Errorf("failed to fetch parameters: %w", err)
		}

		var resp ssmResponse
		if err := json.Unmarshal(out, &resp); err != nil {
			return nil, fmt.Errorf("failed to parse SSM response: %w", err)
		}

		for _, param := range resp.Parameters {
			key := strings.TrimPrefix(param.Name, prefix)
			result[key] = strings.TrimSpace(param.Value)
		}
	}

	return result, nil
}
