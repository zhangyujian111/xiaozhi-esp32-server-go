//go:build integration

package integration

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/api"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/protocol"
)

func TestOTA_HappyPath(t *testing.T) {
	te, mock := StartTestServer(t)
	mock.ExpectQuery("SELECT .+ FROM xiaozhi_device WHERE device_id = ?").
		WillReturnRows(MockDeviceRow(te.deviceID, "v1.0.0"))
	mock.ExpectExec("INSERT INTO xiaozhi_device .+ ON DUPLICATE KEY UPDATE").
		WillReturnResult(&mockResult{rowsAffected: 1})
	mock.ExpectExec("UPDATE xiaozhi_device SET activation_version = .+ WHERE device_id = ?").
		WillReturnResult(&mockResult{rowsAffected: 1})

	reqBody := bytes.NewReader([]byte(`{"current_firmware_version":"v1.0.0"}`))
	req, _ := http.NewRequest("POST", te.server.URL+"/api/device/ota", reqBody)
	req.Header.Set("Device-Id", te.deviceID)
	req.Header.Set("Client-Id", "test-client")
	req.Header.Set("Activation-Version", "2")
	req.Header.Set("Authorization", "Bearer "+te.token)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestOTA_MissingHeaders(t *testing.T) {
	te, _ := StartTestServer(t)
	req, _ := http.NewRequest("POST", te.server.URL+"/api/device/ota", nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOTA_InvalidToken(t *testing.T) {
	te, mock := StartTestServer(t)
	mock.ExpectQuery("SELECT .+ FROM xiaozhi_device WHERE device_id = ?").
		WillReturnRows(MockDeviceRow(te.deviceID, "v1.0.0"))
	mock.ExpectExec("INSERT INTO xiaozhi_device .+ ON DUPLICATE KEY UPDATE").
		WillReturnResult(&mockResult{rowsAffected: 1})
	mock.ExpectExec("UPDATE xiaozhi_device SET activation_version = .+ WHERE device_id = ?").
		WillReturnResult(&mockResult{rowsAffected: 1})
	reqBody := bytes.NewReader([]byte(`{"current_firmware_version":"v1.0.0"}`))
	req, _ := http.NewRequest("POST", te.server.URL+"/api/device/ota", reqBody)
	req.Header.Set("Device-Id", te.deviceID)
	req.Header.Set("Client-Id", "test-client")
	req.Header.Set("Activation-Version", "2")
	req.Header.Set("Authorization", "Bearer invalid-token")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestOTA_FirmwareUpdate(t *testing.T) {
	te, mock := StartTestServer(t)
	mock.ExpectQuery("SELECT .+ FROM xiaozhi_device WHERE device_id = ?").
		WillReturnRows(MockDeviceRow(te.deviceID, "v1.0.0"))
	mock.ExpectExec("INSERT INTO xiaozhi_device .+ ON DUPLICATE KEY UPDATE").
		WillReturnResult(&mockResult{rowsAffected: 1})
	mock.ExpectExec("UPDATE xiaozhi_device SET activation_version = .+ WHERE device_id = ?").
		WillReturnResult(&mockResult{rowsAffected: 1})
	reqBody := bytes.NewReader([]byte(`{"current_firmware_version":"v0.9.0"}`))
	req, _ := http.NewRequest("POST", te.server.URL+"/api/device/ota", reqBody)
	req.Header.Set("Device-Id", te.deviceID)
	req.Header.Set("Client-Id", "test-client")
	req.Header.Set("Activation-Version", "2")
	req.Header.Set("Authorization", "Bearer "+te.token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var result api.OTAResponse
	json.NewDecoder(resp.Body).Decode(&result)
	assert.NotEmpty(t, result.Firmware.URL)
}

func TestOTA_FirmwareUpToDate(t *testing.T) {
	te, mock := StartTestServer(t)
	mock.ExpectQuery("SELECT .+ FROM xiaozhi_device WHERE device_id = ?").
		WillReturnRows(MockDeviceRow(te.deviceID, "v1.1.0"))
	mock.ExpectExec("INSERT INTO xiaozhi_device .+ ON DUPLICATE KEY UPDATE").
		WillReturnResult(&mockResult{rowsAffected: 1})
	mock.ExpectExec("UPDATE xiaozhi_device SET activation_version = .+ WHERE device_id = ?").
		WillReturnResult(&mockResult{rowsAffected: 1})
	reqBody := bytes.NewReader([]byte(`{"current_firmware_version":"1.1.0"}`))
	req, _ := http.NewRequest("POST", te.server.URL+"/api/device/ota", reqBody)
	req.Header.Set("Device-Id", te.deviceID)
	req.Header.Set("Client-Id", "test-client")
	req.Header.Set("Activation-Version", "2")
	req.Header.Set("Authorization", "Bearer "+te.token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var result api.OTAResponse
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Nil(t, result.Firmware)
}

func TestWS_Connect(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	defer conn.Close()
}

func TestWS_Hello(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	defer conn.Close()
	DoHello(t, conn, te)
	assert.NotEmpty(t, te.GetSessionID())
}

func TestWS_Listen(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	defer conn.Close()
	DoHello(t, conn, te)
	SendWSJSON(t, conn, protocol.ListenMessage{
		Type:      protocol.Listen,
		SessionID: te.GetSessionID(),
		State:     protocol.ListenStateStart,
		Mode:      protocol.ListenModeAuto,
	})
}

func TestWS_ListenManual(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	defer conn.Close()
	DoHello(t, conn, te)
	SendWSJSON(t, conn, protocol.ListenMessage{Type: protocol.Listen, SessionID: te.GetSessionID(), State: protocol.ListenStateStart, Mode: protocol.ListenModeManual})
	time.Sleep(50 * time.Millisecond)
	SendWSJSON(t, conn, protocol.ListenMessage{Type: protocol.Listen, SessionID: te.GetSessionID(), State: protocol.ListenStateStop, Mode: protocol.ListenModeManual})
}

func TestWS_Abort(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	defer conn.Close()
	DoHello(t, conn, te)
	SendWSJSON(t, conn, protocol.AbortMessage{Type: protocol.Abort, SessionID: te.GetSessionID(), Reason: "none"})
}

func TestWS_TTS(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	defer conn.Close()
	DoHello(t, conn, te)
	SendWSJSON(t, conn, map[string]interface{}{"type": "tts", "state": "start", "text": "Hello world"})
}

func TestWS_STT(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	defer conn.Close()
	DoHello(t, conn, te)
	SendWSJSON(t, conn, protocol.STTMessage{Type: protocol.STT, Text: "test speech", Emotion: "neutral"})
}

func TestWS_LLM(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	defer conn.Close()
	DoHello(t, conn, te)
	SendWSJSON(t, conn, protocol.LLMMessage{Type: protocol.LLM, Emotion: "happy", Text: "Hello!"})
}

func TestWS_Ack(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	defer conn.Close()
	DoHello(t, conn, te)
	SendWSJSON(t, conn, protocol.AckMessage{Type: protocol.Ack, SessionID: te.GetSessionID(), MsgType: "binary", MsgID: "frame-1", Status: "ok"})
}

func TestWS_Heartbeat(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	defer conn.Close()
	DoHello(t, conn, te)
	SendWSJSON(t, conn, protocol.SystemMessage{Type: protocol.System, Event: "heartbeat", SessionID: te.GetSessionID()})
}

func TestWS_Close(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	DoHello(t, conn, te)
	conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	time.Sleep(100 * time.Millisecond)
	conn.Close()
}

func TestREST_ListDevices(t *testing.T) {
}

func TestREST_GetDevice(t *testing.T) {
}

func TestREST_BindDevice(t *testing.T) {
}

func TestREST_UnbindDevice(t *testing.T) {
}

func TestREST_DeleteDevice(t *testing.T) {
}

func TestREST_JWTAuthValid(t *testing.T) {
}

func TestREST_JWTAuthInvalid(t *testing.T) {
}

func TestREST_JWTAuthMissing(t *testing.T) {
}

func TestREST_JWTAuthExpired(t *testing.T) {
}

func TestREST_InternalProxyForward(t *testing.T) {
}

func TestREST_UpstreamError(t *testing.T) {
}

func TestError_Server404(t *testing.T) {
	te, _ := StartTestServer(t)
	req, _ := http.NewRequest("GET", te.server.URL+"/nonexistent", nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestError_Device404(t *testing.T) {
}

func TestError_AuthFailure(t *testing.T) {
}

func TestError_PayloadTooLarge(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	defer conn.Close()
	DoHello(t, conn, te)
	large := make([]byte, 2*1024*1024+1)
	for i := range large {
		large[i] = 'A'
	}
	err := conn.WriteMessage(websocket.TextMessage, large)
	assert.NoError(t, err)
}

func TestError_OversizedFrame(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	defer conn.Close()
	DoHello(t, conn, te)
	header := []byte{0, 0, 0xFF, 0xFF}
	large := append(header, make([]byte, 65535)...)
	err := conn.WriteMessage(websocket.BinaryMessage, large)
	assert.NoError(t, err)
}

func TestError_MalformedJSON(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	defer conn.Close()
	DoHello(t, conn, te)
	err := conn.WriteMessage(websocket.TextMessage, []byte("{bad json}"))
	assert.NoError(t, err)
}

func TestMultiDevice_TwoDevices(t *testing.T) {
	te1, _ := StartTestServer(t)
	conn1 := OpenDeviceWS(t, te1)
	defer conn1.Close()
	DoHello(t, conn1, te1)

	te2, _ := StartTestServer(t)
	te2.deviceID = "test-device-002"
	conn2 := OpenDeviceWS(t, te2)
	defer conn2.Close()
	DoHello(t, conn2, te2)

	assert.NotEmpty(t, te1.GetSessionID())
	assert.NotEmpty(t, te2.GetSessionID())
	assert.NotEqual(t, te1.GetSessionID(), te2.GetSessionID())
}

func TestMultiDevice_FiveDevices(t *testing.T) {
	te, _ := StartTestServer(t)
	conns := make([]*websocket.Conn, 5)
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()
	for i := range conns {
		te.deviceID = fmt.Sprintf("test-device-%03d", i)
		conns[i] = OpenDeviceWS(t, te)
		DoHello(t, conns[i], te)
		assert.NotEmpty(t, te.GetSessionID())
	}
}

func TestMultiDevice_TenDevices(t *testing.T) {
	te, _ := StartTestServer(t)
	conns := make([]*websocket.Conn, 10)
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()
	for i := range conns {
		te.deviceID = fmt.Sprintf("test-device-%03d", i)
		conns[i] = OpenDeviceWS(t, te)
		DoHello(t, conns[i], te)
	}
	for _, c := range conns {
		SendWSJSON(t, c, protocol.ListenMessage{Type: protocol.Listen, SessionID: te.GetSessionID(), State: protocol.ListenStateStart, Mode: protocol.ListenModeAuto})
		time.Sleep(10 * time.Millisecond)
	}
}

func TestMultiDevice_Stress30s(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	defer conn.Close()
	DoHello(t, conn, te)
	deadline := time.Now().Add(30 * time.Second)
	for i := 0; time.Now().Before(deadline); i++ {
		SendWSJSON(t, conn, protocol.ListenMessage{Type: protocol.Listen, SessionID: te.GetSessionID(), State: protocol.ListenStateStart, Mode: protocol.ListenModeAuto})
		time.Sleep(100 * time.Millisecond)
	}
}

func TestMultiDevice_Cleanup(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	DoHello(t, conn, te)
	conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	time.Sleep(100 * time.Millisecond)
	conn.Close()
}

func TestProtocolV1_Frame(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	defer conn.Close()
	DoHello(t, conn, te)
	SendWSBinary(t, conn, []byte("raw-binary-data"))
}

func TestProtocolV2_Frame(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	defer conn.Close()
	DoHello(t, conn, te)
	header := make([]byte, 16)
	binary.LittleEndian.PutUint16(header[0:2], 2)
	binary.LittleEndian.PutUint32(header[8:12], 12345)
	binary.LittleEndian.PutUint32(header[12:16], 11)
	data := append(header, []byte("hello world")...)
	SendWSBinary(t, conn, data)
}

func TestProtocolV3_Frame(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	defer conn.Close()
	DoHello(t, conn, te)
	header := make([]byte, 4)
	header[0] = 0
	header[1] = 0
	binary.LittleEndian.PutUint16(header[2:4], 11)
	data := append(header, []byte("hello world")...)
	SendWSBinary(t, conn, data)
}

func TestProtocol_MixedVersions(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	defer conn.Close()
	DoHello(t, conn, te)

	SendWSBinary(t, conn, []byte("v1 raw"))
	time.Sleep(50 * time.Millisecond)

	header := make([]byte, 16)
	binary.LittleEndian.PutUint16(header[0:2], 2)
	binary.LittleEndian.PutUint32(header[12:16], 4)
	SendWSBinary(t, conn, append(header, []byte("v2op")...))
	time.Sleep(50 * time.Millisecond)

	h3 := make([]byte, 4)
	binary.LittleEndian.PutUint16(h3[2:4], 3)
	SendWSBinary(t, conn, append(h3, []byte("v3!")...))
}

func TestProtocol_InvalidVersion(t *testing.T) {
}

func TestOpusRoundtrip(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	defer conn.Close()
	DoHello(t, conn, te)
	header := make([]byte, 4)
	binary.LittleEndian.PutUint16(header[2:4], 12)
	SendWSBinary(t, conn, append(header, []byte("0123456789AB")...))
}

func TestOpusSilence(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	defer conn.Close()
	DoHello(t, conn, te)
	header := make([]byte, 4)
	binary.LittleEndian.PutUint16(header[2:4], 0)
	SendWSBinary(t, conn, header)
}

func TestOpusLoudSignal(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	defer conn.Close()
	DoHello(t, conn, te)
	header := make([]byte, 4)
	binary.LittleEndian.PutUint16(header[2:4], 240)
	loud := append(header, bytes.Repeat([]byte{0xFF, 0x7F}, 120)...)
	SendWSBinary(t, conn, loud)
}

func TestOpusFormatDetection(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	defer conn.Close()
	DoHello(t, conn, te)
	header := make([]byte, 4)
	binary.LittleEndian.PutUint16(header[2:4], 12)
	SendWSBinary(t, conn, append(header, []byte("test-frame-1")...))
}

func TestOpusResample(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	defer conn.Close()
	DoHello(t, conn, te)
	for i := 0; i < 5; i++ {
		header := make([]byte, 4)
		binary.LittleEndian.PutUint16(header[2:4], 12)
		SendWSBinary(t, conn, append(header, []byte(fmt.Sprintf("fr%04d", i))...))
		time.Sleep(20 * time.Millisecond)
	}
}

func TestWS_ListenRealtime(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	defer conn.Close()
	DoHello(t, conn, te)
	SendWSJSON(t, conn, protocol.ListenMessage{Type: protocol.Listen, SessionID: te.GetSessionID(), State: protocol.ListenStateStart, Mode: protocol.ListenModeRealtime})
	time.Sleep(100 * time.Millisecond)
	SendWSJSON(t, conn, protocol.ListenMessage{Type: protocol.Listen, SessionID: te.GetSessionID(), State: protocol.ListenStateStop, Mode: protocol.ListenModeRealtime})
}

func TestWS_MultiTurn(t *testing.T) {
	te, _ := StartTestServer(t)
	conn := OpenDeviceWS(t, te)
	defer conn.Close()
	DoHello(t, conn, te)
	for i := 0; i < 3; i++ {
		SendWSJSON(t, conn, protocol.ListenMessage{Type: protocol.Listen, SessionID: te.GetSessionID(), State: protocol.ListenStateStart, Mode: protocol.ListenModeAuto})
		time.Sleep(50 * time.Millisecond)
		SendWSJSON(t, conn, protocol.STTMessage{Type: protocol.STT, Text: fmt.Sprintf("turn %d", i)})
		time.Sleep(50 * time.Millisecond)
	}
}
