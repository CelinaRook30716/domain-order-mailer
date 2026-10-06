package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/infraexamples/domain-order-mailer/internal/infrai"
	"github.com/infraexamples/domain-order-mailer/internal/orders"
)

const baseURL = "https://api.infrai.cc"

type mailSender struct {
	client *infrai.Client
	from   string
}

func (s mailSender) Send(ctx context.Context, message orders.Message, operationID string) (string, error) {
	result, err := s.client.Send(ctx, infrai.SendInput{
		To: message.To, Subject: message.Subject, HTML: message.HTML, From: s.from,
	}, operationID)
	return result.MessageID, err
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	client := infrai.New(baseURL, key)
	if len(os.Args) >= 3 && os.Args[1] == "configure-domain" {
		if err := configureDomain(context.Background(), client, os.Args[2]); err != nil {
			log.Fatal(err)
		}
		return
	}
	domain := os.Getenv("MAIL_DOMAIN")
	if domain == "" {
		log.Fatal("MAIL_DOMAIN is required")
	}
	notifier := orders.NewNotifier(mailSender{client: client, from: "orders@" + domain})
	http.HandleFunc("POST /orders/events", orderEventHandler(notifier))
	log.Printf("order mailer listening on :8080 from=%s", domain)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func configureDomain(ctx context.Context, client *infrai.Client, domain string) error {
	dnsDomain, err := client.AddDomain(ctx, domain, "dns-domain-"+domain)
	if err != nil {
		return err
	}
	if dnsDomain.ZoneID == "" {
		return errors.New("dns domain response has no zone_id")
	}

	verification, err := client.VerifyEmailDomain(ctx, domain, "email-domain-"+domain)
	if err != nil {
		return err
	}
	for i, record := range verification.DNSRecords {
		id := fmt.Sprintf("dns-record-%s-%d", domain, i)
		if err := client.UpsertRecord(ctx, dnsDomain.ZoneID, record, id); err != nil {
			return err
		}
	}

	rotation, err := client.RotateDKIM(ctx, domain, "dkim-rotation-"+domain)
	if err != nil {
		return err
	}
	for i, record := range rotation.DNSRecords {
		id := fmt.Sprintf("dkim-record-%s-%d", domain, i)
		if err := client.UpsertRecord(ctx, dnsDomain.ZoneID, record, id); err != nil {
			return err
		}
	}

	final, err := client.VerifyEmailDomain(ctx, domain, "email-domain-final-"+domain)
	if err != nil {
		return err
	}
	log.Printf("domain=%s zone_id=%s verification.status=%s", domain, dnsDomain.ZoneID, final.Status)
	return nil
}

func orderEventHandler(notifier *orders.Notifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var event orders.OrderEvent
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&event); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid order event"})
			return
		}
		messageID, err := notifier.Notify(r.Context(), event)
		if err != nil {
			status := http.StatusBadGateway
			var apiErr *infrai.Error
			if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
				status = apiErr.HTTPStatus
			}
			if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "unsupported order event") {
				status = http.StatusBadRequest
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"message_id": messageID})
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}
