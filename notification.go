package lime

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Notification Information about events associated to a Message in a Session.
// Can be originated by a server or by the Message destination Node.
type Notification struct {
	Envelope

	// Event Related event To the notification
	Event NotificationEvent

	// Reason In the case of a failed event, brings more details about the problem.
	Reason *Reason
}

func (n Notification) MarshalJSON() ([]byte, error) {
	raw, err := n.toRawEnvelope()
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

func (n *Notification) UnmarshalJSON(b []byte) error {
	raw := rawEnvelope{}
	err := json.Unmarshal(b, &raw)
	if err != nil {
		return err
	}

	notification := Notification{}
	err = notification.populate(&raw)
	if err != nil {
		return err
	}

	*n = notification
	return nil
}

func (n *Notification) toRawEnvelope() (*rawEnvelope, error) {
	raw, err := n.Envelope.toRawEnvelope()
	if err != nil {
		return nil, err
	}

	if n.Event != "" {
		raw.Event = &n.Event
	}

	raw.Reason = n.Reason

	return raw, nil
}

func (n *Notification) populate(raw *rawEnvelope) error {
	err := n.Envelope.populate(raw)
	if err != nil {
		return err
	}

	if raw.Event == nil {
		return errors.New("notification event is required")
	}

	n.Event = *raw.Event
	n.Reason = raw.Reason

	return nil
}

// NotificationEvent Events that can happen in the message pipeline.
type NotificationEvent string

const (
	// NotificationEventAccepted The message was received and accepted by the server.
	// This event is similar To 'received' but is emitted by an intermediate node (hop) and not by the message's final destination.
	NotificationEventAccepted = NotificationEvent("accepted")
	// NotificationEventDispatched The message was dispatched To the destination by the server.
	// This event is similar To the 'consumed' but is emitted by an intermediate node (hop) and not by the message's final destination.
	NotificationEventDispatched = NotificationEvent("dispatched")
	// NotificationEventReceived The node has received the message.
	NotificationEventReceived = NotificationEvent("received")
	// NotificationEventConsumed The node has consumed the Content of the message.
	NotificationEventConsumed = NotificationEvent("consumed")
	// NotificationEventFailed A problem occurred during the processing of the message.
	// In this case, the reason property of the notification should be present.
	NotificationEventFailed = NotificationEvent("failed")
)

func (e *NotificationEvent) Validate() error {
	switch *e {
	case NotificationEventAccepted, NotificationEventDispatched, NotificationEventReceived, NotificationEventConsumed, NotificationEventFailed:
		return nil
	}

	return fmt.Errorf("invalid notification event '%v'", e)
}

func (e NotificationEvent) MarshalText() ([]byte, error) {
	err := e.Validate()
	if err != nil {
		return []byte{}, err
	}
	return []byte(e), nil
}

func (e *NotificationEvent) UnmarshalText(text []byte) error {
	event := NotificationEvent(text)
	err := event.Validate()
	if err != nil {
		return err
	}
	*e = event
	return nil
}
