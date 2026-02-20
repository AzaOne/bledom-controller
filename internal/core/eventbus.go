package core

import "sync"

// EventType defines the type of event being published.
type EventType string

const (
	StateChangedEvent    EventType = "StateChanged"
	DeviceConnectedEvent EventType = "DeviceConnected"
	PatternChangedEvent  EventType = "PatternChanged"
	PowerChangedEvent    EventType = "PowerChanged"
	ColorChangedEvent    EventType = "ColorChanged"
)

// Event is the envelope for all system events.
type Event struct {
	Type    EventType
	Payload interface{}
}

// Subscriber is a channel that receives events.
type Subscriber chan Event

// EventBus handles pub/sub messaging for the application.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[EventType][]Subscriber
}

// NewEventBus creates a new EventBus.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[EventType][]Subscriber),
	}
}

// Subscribe returns a channel that receives events of the given types.
func (eb *EventBus) Subscribe(eventTypes ...EventType) Subscriber {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	ch := make(Subscriber, 100) // Buffered channel so publishers don't block
	for _, t := range eventTypes {
		eb.subscribers[t] = append(eb.subscribers[t], ch)
	}

	return ch
}

// Unsubscribe removes a subscriber channel.
func (eb *EventBus) Unsubscribe(ch Subscriber, eventTypes ...EventType) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	for _, t := range eventTypes {
		subs := eb.subscribers[t]
		for i, sub := range subs {
			if sub == ch {
				// Remove the subscriber from the slice
				eb.subscribers[t] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
	}
}

// Publish distributes an event to all active subscribers for its type.
func (eb *EventBus) Publish(event Event) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	if subs, ok := eb.subscribers[event.Type]; ok {
		for _, sub := range subs {
			select {
			case sub <- event:
			default:
				// If the subscriber channel is full, we drop the event to prevent blocking the publishers
			}
		}
	}
}
