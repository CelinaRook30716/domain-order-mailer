package orders

import (
	"context"
	"fmt"
	"html"
)

type Event string

const (
	Checkout    Event = "checkout"
	Fulfillment Event = "fulfillment"
	Receipt     Event = "receipt"
	Update      Event = "update"
)

type OrderEvent struct {
	OrderID       string `json:"order_id"`
	CustomerName  string `json:"customer_name"`
	CustomerEmail string `json:"customer_email"`
	Event         Event  `json:"event"`
	Detail        string `json:"detail"`
}

type Message struct {
	To      string
	Subject string
	HTML    string
}

type Sender interface {
	Send(context.Context, Message, string) (string, error)
}

type Notifier struct {
	sender Sender
}

func NewNotifier(sender Sender) *Notifier {
	return &Notifier{sender: sender}
}

func BuildMessage(event OrderEvent) (Message, error) {
	labels := map[Event]string{
		Checkout:    "Order received",
		Fulfillment: "Order is on its way",
		Receipt:     "Receipt for your order",
		Update:      "Order update",
	}
	label, ok := labels[event.Event]
	if !ok {
		return Message{}, fmt.Errorf("unsupported order event %q", event.Event)
	}
	if event.OrderID == "" || event.CustomerEmail == "" {
		return Message{}, fmt.Errorf("order_id and customer_email are required")
	}
	content := fmt.Sprintf("<h1>%s</h1><p>Hi %s,</p><p>%s</p><p>Order: %s</p>",
		html.EscapeString(label), html.EscapeString(event.CustomerName),
		html.EscapeString(event.Detail), html.EscapeString(event.OrderID))
	return Message{To: event.CustomerEmail, Subject: label + " - " + event.OrderID, HTML: content}, nil
}

func (n *Notifier) Notify(ctx context.Context, event OrderEvent) (string, error) {
	message, err := BuildMessage(event)
	if err != nil {
		return "", err
	}
	return n.sender.Send(ctx, message, "order-"+event.OrderID+"-"+string(event.Event))
}
