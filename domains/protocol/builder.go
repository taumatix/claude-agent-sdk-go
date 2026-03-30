package protocol

import "encoding/json"

// BuildUserMessage constructs a user message wire frame.
func BuildUserMessage(sessionID, content string, parentToolUseID *string) ([]byte, error) {
	msg := map[string]interface{}{
		"type":       TypeUser,
		"session_id": sessionID,
		"message": map[string]interface{}{
			"role":    "user",
			"content": content,
		},
		"parent_tool_use_id": parentToolUseID,
	}
	return json.Marshal(msg)
}

// BuildControlRequest constructs a control_request envelope.
func BuildControlRequest(requestID string, body interface{}) ([]byte, error) {
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	env := map[string]interface{}{
		"type":       TypeControlRequest,
		"request_id": requestID,
		"request":    json.RawMessage(bodyJSON),
	}
	return json.Marshal(env)
}

// BuildControlResponse constructs a successful control_response envelope.
func BuildControlResponse(requestID string, body interface{}) ([]byte, error) {
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	env := map[string]interface{}{
		"type": TypeControlResponse,
		"response": map[string]interface{}{
			"subtype":    "success",
			"request_id": requestID,
			"response":   json.RawMessage(bodyJSON),
		},
	}
	return json.Marshal(env)
}

// BuildControlErrorResponse constructs an error control_response envelope.
func BuildControlErrorResponse(requestID, errMsg string) ([]byte, error) {
	env := map[string]interface{}{
		"type": TypeControlResponse,
		"response": map[string]interface{}{
			"subtype":    "error",
			"request_id": requestID,
			"error":      errMsg,
		},
	}
	return json.Marshal(env)
}

// BuildInterruptRequest constructs an interrupt control request.
func BuildInterruptRequest(requestID string) ([]byte, error) {
	body := InterruptRequestBody{
		Subtype: SubtypeInterrupt,
	}
	return BuildControlRequest(requestID, body)
}

// BuildSetPermissionModeRequest constructs a set_permission_mode control request.
func BuildSetPermissionModeRequest(requestID, mode string) ([]byte, error) {
	body := SetPermModeRequestBody{
		Subtype: SubtypeSetPermMode,
		Mode:    mode,
	}
	return BuildControlRequest(requestID, body)
}

// BuildSetModelRequest constructs a set_model control request.
func BuildSetModelRequest(requestID, model string) ([]byte, error) {
	body := SetModelRequestBody{
		Subtype: SubtypeSetModel,
		Model:   model,
	}
	return BuildControlRequest(requestID, body)
}
