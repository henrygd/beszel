package ws

import (
	"context"
	"errors"

	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/smart"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/entities/systemd"
	"github.com/lxzan/gws"
	"golang.org/x/crypto/ssh"
)

// ResponseHandler defines interface for handling agent responses
type ResponseHandler interface {
	Handle(agentResponse common.AgentResponse) error
	HandleLegacy(rawData []byte) error
}

// BaseHandler provides a default implementation that can be embedded to make HandleLegacy optional
type BaseHandler struct{}

func (h *BaseHandler) HandleLegacy(rawData []byte) error {
	return errors.New("legacy format not supported")
}

////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////

// systemDataHandler implements ResponseHandler for system data requests
type systemDataHandler struct {
	data *system.CombinedData
}

func (h *systemDataHandler) HandleLegacy(rawData []byte) error {
	return cbor.Unmarshal(rawData, h.data)
}

func (h *systemDataHandler) Handle(agentResponse common.AgentResponse) error {
	if agentResponse.SystemData != nil {
		*h.data = *agentResponse.SystemData
	}
	return nil
}

// RequestSystemData requests system metrics from the agent and unmarshals the response.
func (ws *WsConn) RequestSystemData(ctx context.Context, data *system.CombinedData, options common.DataRequestOptions) error {
	if !ws.IsConnected() {
		return gws.ErrConnClosed
	}

	req, err := ws.requestManager.SendRequest(ctx, common.GetData, options)
	if err != nil {
		return err
	}

	handler := &systemDataHandler{data: data}
	return ws.handleAgentRequest(req, handler)
}

////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////

// stringResponseHandler is a generic handler for string responses from agents
type stringResponseHandler struct {
	BaseHandler
	value    string
	errorMsg string
}

func (h *stringResponseHandler) Handle(agentResponse common.AgentResponse) error {
	if agentResponse.String == nil {
		return errors.New(h.errorMsg)
	}
	h.value = *agentResponse.String
	return nil
}

////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////

// requestContainerStringViaWS is a generic function to request container-related strings via WebSocket
func (ws *WsConn) requestContainerStringViaWS(ctx context.Context, action common.WebSocketAction, requestData any, errorMsg string) (string, error) {
	if !ws.IsConnected() {
		return "", gws.ErrConnClosed
	}

	req, err := ws.requestManager.SendRequest(ctx, action, requestData)
	if err != nil {
		return "", err
	}

	handler := &stringResponseHandler{errorMsg: errorMsg}
	if err := ws.handleAgentRequest(req, handler); err != nil {
		return "", err
	}

	return handler.value, nil
}

// RequestContainerLogs requests logs for a specific container via WebSocket.
func (ws *WsConn) RequestContainerLogs(ctx context.Context, containerID string) (string, error) {
	return ws.requestContainerStringViaWS(ctx, common.GetContainerLogs, common.ContainerLogsRequest{ContainerID: containerID}, "no logs in response")
}

// RequestContainerInfo requests information about a specific container via WebSocket.
func (ws *WsConn) RequestContainerInfo(ctx context.Context, containerID string) (string, error) {
	return ws.requestContainerStringViaWS(ctx, common.GetContainerInfo, common.ContainerInfoRequest{ContainerID: containerID}, "no info in response")
}

// RequestPodLogs requests logs for a specific pod via WebSocket.
func (ws *WsConn) RequestPodLogs(ctx context.Context, namespace, podName string) (string, error) {
	return ws.requestContainerStringViaWS(ctx, common.GetPodLogs, common.PodLogsRequest{Namespace: namespace, PodName: podName}, "no logs in response")
}

// RequestPodInfo requests information about a specific pod via WebSocket.
func (ws *WsConn) RequestPodInfo(ctx context.Context, namespace, podName string) (string, error) {
	return ws.requestContainerStringViaWS(ctx, common.GetPodInfo, common.PodInfoRequest{Namespace: namespace, PodName: podName}, "no info in response")
}

////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////

// RequestSystemdInfo requests detailed information about a systemd service via WebSocket.
func (ws *WsConn) RequestSystemdInfo(ctx context.Context, serviceName string) (systemd.ServiceDetails, error) {
	if !ws.IsConnected() {
		return nil, gws.ErrConnClosed
	}

	req, err := ws.requestManager.SendRequest(ctx, common.GetSystemdInfo, common.SystemdInfoRequest{ServiceName: serviceName})
	if err != nil {
		return nil, err
	}

	var result systemd.ServiceDetails
	handler := &systemdInfoHandler{result: &result}
	if err := ws.handleAgentRequest(req, handler); err != nil {
		return nil, err
	}

	return result, nil
}

// systemdInfoHandler parses ServiceDetails from AgentResponse
type systemdInfoHandler struct {
	BaseHandler
	result *systemd.ServiceDetails
}

func (h *systemdInfoHandler) Handle(agentResponse common.AgentResponse) error {
	if agentResponse.ServiceInfo == nil {
		return errors.New("no systemd info in response")
	}
	*h.result = agentResponse.ServiceInfo
	return nil
}

////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////

// RequestSmartData requests SMART data via WebSocket.
func (ws *WsConn) RequestSmartData(ctx context.Context) (map[string]smart.SmartData, error) {
	if !ws.IsConnected() {
		return nil, gws.ErrConnClosed
	}
	req, err := ws.requestManager.SendRequest(ctx, common.GetSmartData, nil)
	if err != nil {
		return nil, err
	}
	var result map[string]smart.SmartData
	handler := ResponseHandler(&smartDataHandler{result: &result})
	if err := ws.handleAgentRequest(req, handler); err != nil {
		return nil, err
	}
	return result, nil
}

// smartDataHandler parses SMART data map from AgentResponse
type smartDataHandler struct {
	BaseHandler
	result *map[string]smart.SmartData
}

func (h *smartDataHandler) Handle(agentResponse common.AgentResponse) error {
	if agentResponse.SmartData == nil {
		return errors.New("no SMART data in response")
	}
	*h.result = agentResponse.SmartData
	return nil
}

////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////

// fingerprintHandler implements ResponseHandler for fingerprint requests
type fingerprintHandler struct {
	result *common.FingerprintResponse
}

func (h *fingerprintHandler) HandleLegacy(rawData []byte) error {
	return cbor.Unmarshal(rawData, h.result)
}

func (h *fingerprintHandler) Handle(agentResponse common.AgentResponse) error {
	if agentResponse.Fingerprint != nil {
		*h.result = *agentResponse.Fingerprint
		return nil
	}
	return errors.New("no fingerprint data in response")
}

// GetFingerprint authenticates with the agent using SSH signature and returns the agent's fingerprint.
func (ws *WsConn) GetFingerprint(ctx context.Context, token string, signer ssh.Signer, needSysInfo bool) (common.FingerprintResponse, error) {
	if !ws.IsConnected() {
		return common.FingerprintResponse{}, gws.ErrConnClosed
	}

	challenge := []byte(token)
	signature, err := signer.Sign(nil, challenge)
	if err != nil {
		return common.FingerprintResponse{}, err
	}

	req, err := ws.requestManager.SendRequest(ctx, common.CheckFingerprint, common.FingerprintRequest{
		Signature:   signature.Blob,
		NeedSysInfo: needSysInfo,
	})
	if err != nil {
		return common.FingerprintResponse{}, err
	}

	var result common.FingerprintResponse
	handler := &fingerprintHandler{result: &result}
	err = ws.handleAgentRequest(req, handler)
	return result, err
}
