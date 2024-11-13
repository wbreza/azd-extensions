// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package azure

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/fatih/color"
)

type DeploymentErrorLine struct {
	// The code of the error line, if applicable
	Code string
	// The message that represents the error
	Message string
	// Inner errors
	Inner []*DeploymentErrorLine
}

func newErrorLine(code string, message string, inner []*DeploymentErrorLine) *DeploymentErrorLine {
	return &DeploymentErrorLine{
		Message: message,
		Code:    code,
		Inner:   inner,
	}
}

type DeploymentError struct {
	Json string

	Details *DeploymentErrorLine
}

// Attempts to create an Azure Deployment error from the HTTP response error
func newDeploymentError(err error) error {
	var responseErr *azcore.ResponseError
	if errors.As(err, &responseErr) {
		var errorText string
		rawBody, err := io.ReadAll(responseErr.RawResponse.Body)
		if err != nil {
			errorText = responseErr.Error()
		} else {
			errorText = string(rawBody)
		}
		return NewDeploymentError(errorText)
	}

	return err
}

func NewDeploymentError(jsonErrorResponse string) *DeploymentError {
	err := &DeploymentError{Json: jsonErrorResponse}
	err.init()
	return err
}

func (e *DeploymentError) init() {
	var errorMap map[string]interface{}
	if err := json.Unmarshal([]byte(e.Json), &errorMap); err == nil {
		e.Details = getErrorsFromMap(errorMap)
	}
}

func (e *DeploymentError) Error() string {
	// Return the original error string if we can't parse the JSON
	if e.Details == nil {
		return e.Json
	}

	lines := generateErrorOutput(e.Details)

	var sb strings.Builder

	for _, line := range lines {
		sb.WriteString(fmt.Sprintln(color.RedString(line)))
	}

	return sb.String()
}

func generateErrorOutput(err *DeploymentErrorLine) []string {
	lines := []string{}

	if strings.TrimSpace(err.Message) != "" {
		lines = append(lines, err.Message)
	}

	for _, innerError := range err.Inner {
		if innerError != nil {
			lines = append(lines, generateErrorOutput(innerError)...)
		}
	}

	return lines
}

func getErrorsFromMap(errorMap map[string]interface{}) *DeploymentErrorLine {
	var output *DeploymentErrorLine
	var code, message string

	// Size of nested output is not known ahead of time.
	nestedOutput := []*DeploymentErrorLine{}

	for key, value := range errorMap {
		switch strings.ToLower(key) {
		case "code":
			code = fmt.Sprint(value)
		case "message":
			rawMessage := fmt.Sprint(value)
			var messageMap map[string]interface{}
			err := json.Unmarshal([]byte(rawMessage), &messageMap)
			if err == nil {
				nestedOutput = append(nestedOutput, getErrorsFromMap(messageMap))
			} else {
				message = rawMessage
			}
		case "error":
			errorMap, ok := value.(map[string]interface{})
			var line *DeploymentErrorLine
			if !ok {
				line = &DeploymentErrorLine{Message: fmt.Sprintf("%s", value)}
			} else {
				line = getErrorsFromMap(errorMap)
			}

			if line != nil {
				nestedOutput = append(nestedOutput, line)
			}
		case "details":
			var lines []*DeploymentErrorLine
			errorArray, ok := value.([]interface{})
			if !ok {
				line := &DeploymentErrorLine{Message: fmt.Sprintf("%s", value)}
				lines = []*DeploymentErrorLine{line}
			} else {
				lines = getErrorsFromArray(errorArray)
			}
			nestedOutput = append(nestedOutput, lines...)
		}
	}

	// Append status, codes, messages first
	var errorMessage string

	// Omit generic deployment failed messages
	if code == "DeploymentFailed" || code == "ResourceDeploymentFailure" {
		return newErrorLine("", errorMessage, nestedOutput)
	}

	if code != "" && message != "" {
		errorMessage = fmt.Sprintf("%s: %s", code, message)
	} else if message != "" {
		errorMessage = fmt.Sprintf("- %s", message)
	}

	output = newErrorLine(code, errorMessage, nestedOutput)

	return output
}

func getErrorsFromArray(errorArray []interface{}) []*DeploymentErrorLine {
	output := make([]*DeploymentErrorLine, len(errorArray))
	for index, value := range errorArray {
		errorMap, ok := value.(map[string]interface{})
		if !ok {
			output[index] = &DeploymentErrorLine{Message: fmt.Sprintf("%s", value)}
		} else {
			output[index] = getErrorsFromMap(errorMap)
		}
	}

	return output
}
