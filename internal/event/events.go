package event

func NewDeviceConnectedEvent(deviceID, sessionID string) Event {
	return Event{
		Type:      "device_connected",
		DeviceID:  deviceID,
		SessionID: sessionID,
	}
}

func NewDeviceDisconnectedEvent(deviceID, sessionID string) Event {
	return Event{
		Type:      "device_disconnected",
		DeviceID:  deviceID,
		SessionID: sessionID,
	}
}
