package orders

import "testing"

func TestBuildMessageChoosesOrderNotification(t *testing.T) {
	tests := []struct {
		name    string
		event   OrderEvent
		subject string
		wantErr bool
	}{
		{"checkout confirmation", OrderEvent{OrderID: "ord-42", CustomerEmail: "buyer@example.com", Event: Checkout}, "Order received - ord-42", false},
		{"fulfillment notice", OrderEvent{OrderID: "ord-42", CustomerEmail: "buyer@example.com", Event: Fulfillment}, "Order is on its way - ord-42", false},
		{"receipt", OrderEvent{OrderID: "ord-42", CustomerEmail: "buyer@example.com", Event: Receipt}, "Receipt for your order - ord-42", false},
		{"customer update", OrderEvent{OrderID: "ord-42", CustomerEmail: "buyer@example.com", Event: Update}, "Order update - ord-42", false},
		{"unknown event", OrderEvent{OrderID: "ord-42", CustomerEmail: "buyer@example.com", Event: "refund"}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message, err := BuildMessage(tt.event)
			if (err != nil) != tt.wantErr {
				t.Fatalf("BuildMessage() error = %v, wantErr %v", err, tt.wantErr)
			}
			if message.Subject != tt.subject {
				t.Fatalf("subject = %q, want %q", message.Subject, tt.subject)
			}
		})
	}
}
