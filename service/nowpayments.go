package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
)

const nowPaymentsRequestTimeout = 15 * time.Second

var NOWPaymentsAPIBaseURL = "https://api.nowpayments.io/v1"
var NOWPaymentsHTTPClient = &http.Client{Timeout: nowPaymentsRequestTimeout}

type NOWPaymentsInvoiceRequest struct {
	PriceAmount      string `json:"price_amount"`
	PriceCurrency    string `json:"price_currency"`
	PayCurrency      string `json:"pay_currency,omitempty"`
	OrderID          string `json:"order_id"`
	OrderDescription string `json:"order_description"`
	IPNCallbackURL   string `json:"ipn_callback_url"`
	SuccessURL       string `json:"success_url"`
	CancelURL        string `json:"cancel_url"`
}

type NOWPaymentsInvoice struct {
	ID         string `json:"id"`
	InvoiceURL string `json:"invoice_url"`
}

type NOWPaymentsPayment struct {
	PaymentID     string `json:"payment_id"`
	PaymentStatus string `json:"payment_status"`
	OrderID       string `json:"order_id"`
	PriceAmount   string `json:"price_amount"`
	PriceCurrency string `json:"price_currency"`
}

type NOWPaymentsClient struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
}

func NewNOWPaymentsClient(httpClient *http.Client) *NOWPaymentsClient {
	if httpClient == nil {
		httpClient = NOWPaymentsHTTPClient
	}
	return &NOWPaymentsClient{httpClient: httpClient, baseURL: NOWPaymentsAPIBaseURL, apiKey: setting.NOWPaymentsAPIKey}
}

func (client *NOWPaymentsClient) CreateInvoice(ctx context.Context, request NOWPaymentsInvoiceRequest) (*NOWPaymentsInvoice, error) {
	var invoice NOWPaymentsInvoice
	if err := client.do(ctx, http.MethodPost, "/invoice", request, &invoice); err != nil {
		return nil, err
	}
	return &invoice, nil
}

func (client *NOWPaymentsClient) GetPayment(ctx context.Context, paymentID string) (*NOWPaymentsPayment, error) {
	var payment NOWPaymentsPayment
	if err := client.do(ctx, http.MethodGet, "/payment/"+url.PathEscape(strings.TrimSpace(paymentID)), nil, &payment); err != nil {
		return nil, err
	}
	return &payment, nil
}

func (client *NOWPaymentsClient) do(ctx context.Context, method, path string, body, result any) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, nowPaymentsRequestTimeout)
		defer cancel()
	}
	var reader io.Reader
	if body != nil {
		data, err := common.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(client.baseURL, "/")+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", client.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("nowpayments returned status %d: %s", resp.StatusCode, string(data))
	}
	return common.Unmarshal(data, result)
}

func NOWPaymentsRequestTimeoutContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, nowPaymentsRequestTimeout)
}
